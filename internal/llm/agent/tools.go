package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"encoding/json"

	"github.com/openscholar/openscholar/internal/debug"
	"github.com/openscholar/openscholar/internal/hooks"
	"github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/message"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/pubsub"
)

const defaultMaxResultBytes = 20 * 1024 // 20 KB

// activeTools returns the current tool set for LLM calls.
// If using DeferredRegistry, returns only core + activated tools.
// If using static tool list, returns all tools.
func (a *agent) activeTools() []tools.BaseTool {
	if a.registry != nil {
		return a.registry.ActiveTools()
	}
	return a.tools
}

// allTools returns all known tools, including deferred tools when a registry is present.
func (a *agent) allTools() []tools.BaseTool {
	if a.registry != nil {
		return a.registry.AllTools()
	}
	return a.tools
}

// findTool looks up a tool by name.
// If using DeferredRegistry, auto-activates deferred tools on lookup.
func (a *agent) findTool(name string) tools.BaseTool {
	if a.registry != nil {
		t, _ := a.registry.FindTool(name)
		return t
	}
	for _, t := range a.tools {
		if t.Info().Name == name {
			return t
		}
	}
	return nil
}

func normalizeToolNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, name)
	}
	return out
}

func resolveAllowedToolSet(catalog []tools.BaseTool, allowed []string) (map[string]struct{}, []string) {
	if len(allowed) == 0 {
		return nil, nil
	}
	index := make(map[string]string, len(catalog))
	for _, tool := range catalog {
		name := strings.TrimSpace(tool.Info().Name)
		if name == "" {
			continue
		}
		index[strings.ToLower(name)] = name
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	var unknown []string
	for _, requested := range normalizeToolNames(allowed) {
		actual, ok := index[strings.ToLower(requested)]
		if !ok {
			unknown = append(unknown, requested)
			continue
		}
		allowedSet[actual] = struct{}{}
	}
	return allowedSet, unknown
}

// intentToolGroups maps user intent keywords to tool names that should be pre-activated.
var intentToolGroups = map[string][]string{
	"kb":      {"KBAdd", "KBQuery", "KBSearch", "KBTree", "KBList"},
	"reading": {"Task", "ScholarSearch"},
	"web":     {"WebSearch", "WebFetch"},
	"docgen":  {"DocExport", "DiagramGen", "ImageGen"},
}

// intentKeywords maps keywords to tool group names.
var intentKeywords = map[string]string{
	"知识库":   "kb",
	"我的知识库": "kb", "本地论文": "kb", "已添加": "kb", "已上传": "kb", "库里": "kb", "这篇 PDF": "kb", "这篇论文": "kb",
	"local kb": "kb", "my papers": "kb", "uploaded": "kb", "this pdf": "kb", "this paper": "kb",
	"pdf": "reading", "PDF": "reading", "论文": "reading", "文献": "reading", "阅读": "reading", "读": "reading",
	"精读": "reading", "多论文": "reading", "多篇论文": "reading", "长论文": "reading", "paper": "reading", "papers": "reading",
	"read": "reading", "reading": "reading", "literature": "reading",
	"报告": "kb",
	// Web tools: URLs and search intent
	"http://": "web", "https://": "web",
	"搜索": "web", "搜一下": "web", "查一下": "web", "查查": "web",
	"网页": "web", "网站": "web", "抓取": "web", "fetch": "web",
	"search the web": "web", "web search": "web",
	"最新": "web", "最近": "web", "latest": "web", "recent": "web",
	// Document generation tools
	"导出": "docgen", "export": "docgen", "转换": "docgen", "convert": "docgen",
	"生成pdf": "docgen", "生成PDF": "docgen", "生成文档": "docgen",
	"docx": "docgen", "DOCX": "docgen",
}

// preActivateToolsByIntent scans user content for intent keywords
// and pre-activates matching tool groups to skip ToolSearch iterations.
func (a *agent) preActivateToolsByIntent(content string) {
	activated := make(map[string]bool)
	for keyword, group := range intentKeywords {
		if strings.Contains(content, keyword) && !activated[group] {
			if group == "kb" && !tools.TextHasLocalKBIntent(content) {
				continue
			}
			activated[group] = true
			if toolNames, ok := intentToolGroups[group]; ok {
				a.registry.ActivateByNames(toolNames)
			}
		}
	}
}

func isTransientStreamError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "unexpected end of JSON input") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "EOF")
}

// executeToolCalls runs all tool calls in assistantMsg and returns the persisted
// tool-result message (or nil when there are no tool calls).
func (a *agent) executeToolCalls(ctx context.Context, assistantMsg message.Message) (message.Message, *message.Message, error) {
	// Initialize all tool calls to queued state with order indices before execution.
	assistantMsg.InitToolCallsQueued()
	a.messages.Update(context.Background(), assistantMsg)

	// Re-fetch tool calls after InitToolCallsQueued has set Order fields.
	toolCalls := assistantMsg.ToolCalls()

	// Emit tool_queued events for every tool call in the batch.
	for _, tc := range toolCalls {
		a.Publish(pubsub.CreatedEvent, AgentEvent{
			Type:      AgentEventTypeToolQueued,
			SessionID: assistantMsg.SessionID,
			Message:   assistantMsg,
			ToolLifecycle: &ToolLifecycleEvent{
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
				BatchID:    tc.BatchID,
				Input:      tc.Input,
				State:      message.ToolCallQueued,
				Order:      tc.Order,
				Total:      len(toolCalls),
			},
		})
	}

	toolResults := make([]message.ToolResult, len(toolCalls))
	batchCooldownScopes := map[string]struct{}{}
	batchDuplicateSignatures := map[string]struct{}{}
	execBudget, hasExecBudget := ctx.Value(toolExecutionBudgetKey).(toolExecutionBudget)
	runtimeAllowedSet := map[string]struct{}(nil)
	if runtime, ok := RequestRuntimeFromContext(ctx); ok && len(runtime.AllowedTools) > 0 {
		var unknown []string
		runtimeAllowedSet, unknown = resolveAllowedToolSet(a.allTools(), runtime.AllowedTools)
		if len(unknown) > 0 {
			return assistantMsg, nil, fmt.Errorf("runtime override references unknown tools: %s", strings.Join(unknown, ", "))
		}
	}
	if shouldExecuteScholarBatchInParallel(toolCalls) {
		a.executeParallelScholarToolCalls(ctx, &assistantMsg, toolCalls, toolResults, &execBudget, hasExecBudget, runtimeAllowedSet)
		goto out
	}
toolLoop:
	for i, toolCall := range toolCalls {
		if a.permissionService != nil && assistantMsg.SessionID != "" {
			ctx = permission.WithMode(ctx, a.permissionService.SessionMode(assistantMsg.SessionID))
		}
		select {
		case <-ctx.Done():
			for j := i; j < len(toolCalls); j++ {
				assistantMsg.CancelToolCall(toolCalls[j].ID)
				toolResults[j] = message.ToolResult{
					ToolCallID: toolCalls[j].ID,
					Content:    "Tool execution canceled",
					IsError:    true,
				}
			}
			// Persist before publishing events
			a.messages.Update(context.Background(), assistantMsg)
			for j := i; j < len(toolCalls); j++ {
				a.Publish(pubsub.CreatedEvent, AgentEvent{
					Type:      AgentEventTypeToolCanceled,
					SessionID: assistantMsg.SessionID,
					Message:   assistantMsg,
					ToolLifecycle: &ToolLifecycleEvent{
						ToolCallID: toolCalls[j].ID,
						ToolName:   toolCalls[j].Name,
						BatchID:    toolCalls[j].BatchID,
						Input:      toolCalls[j].Input,
						State:      message.ToolCallCanceled,
						Order:      toolCalls[j].Order,
						Total:      len(toolCalls),
					},
				})
			}
			goto out
		default:
			if hasExecBudget && shouldSkipToolForBudget(toolCall, &execBudget) {
				a.recordSyntheticToolError(ctx, &assistantMsg, toolCall, &toolResults[i],
					"Skipped tool call because the generation tool budget is exhausted.",
					syntheticToolMetadata(toolCall.Name, "tool_budget_exceeded", canonicalToolCallSignature(toolCall)))
				continue
			}
			dupSig := canonicalToolCallSignature(toolCall)
			if _, ok := batchDuplicateSignatures[dupSig]; ok {
				a.recordSyntheticToolError(ctx, &assistantMsg, toolCall, &toolResults[i],
					"Skipped duplicate tool call; use the previous result.",
					syntheticToolMetadata(toolCall.Name, "duplicate_tool_call", dupSig))
				continue
			}
			batchDuplicateSignatures[dupSig] = struct{}{}
			if hasExecBudget {
				consumeToolBudget(toolCall, &execBudget)
			}

			if scholarShouldSkipForBatchCooldown(toolCall, batchCooldownScopes) {
				assistantMsg.CancelToolCall(toolCall.ID)
				a.messages.Update(ctx, assistantMsg)
				a.Publish(pubsub.CreatedEvent, AgentEvent{
					Type:      AgentEventTypeToolCanceled,
					SessionID: assistantMsg.SessionID,
					Message:   assistantMsg,
					ToolLifecycle: &ToolLifecycleEvent{
						ToolCallID: toolCall.ID,
						ToolName:   toolCall.Name,
						BatchID:    toolCall.BatchID,
						Input:      toolCall.Input,
						State:      message.ToolCallCanceled,
						Order:      toolCall.Order,
						Total:      len(toolCalls),
					},
				})
				toolResults[i] = message.ToolResult{
					ToolCallID: toolCall.ID,
					Content:    scholarCooldownSkipContent(toolCall.Input),
					IsError:    true,
					Metadata:   scholarCooldownSkipMetadata(toolCall.Input),
				}
				a.runPostToolHooks(ctx, toolCall, toolResults[i], nil)
				continue
			}
			if len(runtimeAllowedSet) > 0 {
				if _, ok := runtimeAllowedSet[toolCall.Name]; !ok {
					assistantMsg.ErrorToolCall(toolCall.ID)
					a.messages.Update(ctx, assistantMsg)
					a.Publish(pubsub.CreatedEvent, AgentEvent{
						Type:      AgentEventTypeToolFailed,
						SessionID: assistantMsg.SessionID,
						Message:   assistantMsg,
						ToolLifecycle: &ToolLifecycleEvent{
							ToolCallID: toolCall.ID,
							ToolName:   toolCall.Name,
							BatchID:    toolCall.BatchID,
							Input:      toolCall.Input,
							State:      message.ToolCallErrored,
							Order:      toolCall.Order,
							Total:      len(toolCalls),
						},
					})
					toolResults[i] = message.ToolResult{
						ToolCallID: toolCall.ID,
						Content:    fmt.Sprintf("Tool not allowed by runtime override: %s", toolCall.Name),
						IsError:    true,
					}
					continue
				}
			}
			tool := a.findTool(toolCall.Name)
			if tool == nil {
				assistantMsg.ErrorToolCall(toolCall.ID)
				a.messages.Update(ctx, assistantMsg)
				a.Publish(pubsub.CreatedEvent, AgentEvent{
					Type:      AgentEventTypeToolFailed,
					SessionID: assistantMsg.SessionID,
					Message:   assistantMsg,
					ToolLifecycle: &ToolLifecycleEvent{
						ToolCallID: toolCall.ID,
						ToolName:   toolCall.Name,
						BatchID:    toolCall.BatchID,
						Input:      toolCall.Input,
						State:      message.ToolCallErrored,
						Order:      toolCall.Order,
						Total:      len(toolCalls),
					},
				})
				toolResults[i] = message.ToolResult{
					ToolCallID: toolCall.ID,
					Content:    fmt.Sprintf("Tool not found: %s", toolCall.Name),
					IsError:    true,
				}
				continue
			}

			// Research mode: enforce tool whitelist at execution layer.
			// FilterResearchTools removes non-whitelisted tools from LLM definitions,
			// but weak models may hallucinate calls to tools they cannot see.
			if err := tools.ValidateResearchToolCall(ctx, toolCall.Name); err != nil {
				assistantMsg.ErrorToolCall(toolCall.ID)
				a.messages.Update(ctx, assistantMsg)
				a.Publish(pubsub.CreatedEvent, AgentEvent{
					Type:      AgentEventTypeToolFailed,
					SessionID: assistantMsg.SessionID,
					Message:   assistantMsg,
					ToolLifecycle: &ToolLifecycleEvent{
						ToolCallID: toolCall.ID,
						ToolName:   toolCall.Name,
						BatchID:    toolCall.BatchID,
						Input:      toolCall.Input,
						State:      message.ToolCallErrored,
						Order:      toolCall.Order,
						Total:      len(toolCalls),
					},
				})
				toolResults[i] = message.ToolResult{
					ToolCallID: toolCall.ID,
					Content:    err.Error(),
					IsError:    true,
				}
				continue
			}
			if permission.CurrentMode(ctx) == permission.ModePlan && !tools.PlanModeToolAllowed(toolCall.Name) {
				assistantMsg.ErrorToolCall(toolCall.ID)
				a.messages.Update(ctx, assistantMsg)
				a.Publish(pubsub.CreatedEvent, AgentEvent{
					Type:      AgentEventTypeToolFailed,
					SessionID: assistantMsg.SessionID,
					Message:   assistantMsg,
					ToolLifecycle: &ToolLifecycleEvent{
						ToolCallID: toolCall.ID,
						ToolName:   toolCall.Name,
						BatchID:    toolCall.BatchID,
						Input:      toolCall.Input,
						State:      message.ToolCallErrored,
						Order:      toolCall.Order,
						Total:      len(toolCalls),
					},
				})
				toolResults[i] = message.ToolResult{
					ToolCallID: toolCall.ID,
					Content:    fmt.Sprintf("Tool '%s' is not available in plan mode. Use read-only exploration tools, update only the plan file, then call ExitPlanMode for approval.", toolCall.Name),
					IsError:    true,
				}
				continue
			}

			// [Hook injection] PreToolUse — before tool.Run()
			if a.hookService != nil {
				hookInput := buildHookInput(ctx, toolCall.Name, toolCall.Input)
				if err := a.hookService.RunBlocking(ctx, hooks.PreToolUse, hookInput); err != nil {
					assistantMsg.ErrorToolCall(toolCall.ID)
					a.messages.Update(ctx, assistantMsg)
					a.Publish(pubsub.CreatedEvent, AgentEvent{
						Type:      AgentEventTypeToolFailed,
						SessionID: assistantMsg.SessionID,
						Message:   assistantMsg,
						ToolLifecycle: &ToolLifecycleEvent{
							ToolCallID: toolCall.ID,
							ToolName:   toolCall.Name,
							BatchID:    toolCall.BatchID,
							Input:      toolCall.Input,
							State:      message.ToolCallErrored,
							Order:      toolCall.Order,
							Total:      len(toolCalls),
						},
					})
					toolResults[i] = message.ToolResult{
						ToolCallID: toolCall.ID,
						Content:    "[Hook blocked] " + err.Error(),
						IsError:    true,
					}
					continue
				}
			}

			// Mark tool as running before execution.
			assistantMsg.StartToolCall(toolCall.ID)
			a.messages.Update(ctx, assistantMsg)
			a.Publish(pubsub.CreatedEvent, AgentEvent{
				Type:      AgentEventTypeToolStarted,
				SessionID: assistantMsg.SessionID,
				Message:   assistantMsg,
				ToolLifecycle: &ToolLifecycleEvent{
					ToolCallID: toolCall.ID,
					ToolName:   toolCall.Name,
					BatchID:    toolCall.BatchID,
					Input:      toolCall.Input,
					State:      message.ToolCallRunning,
					Order:      toolCall.Order,
					Total:      len(toolCalls),
				},
			})

			// Debug: log tool start
			dl := debugLoggerFromCtx(ctx)
			toolStart := time.Now()
			if dl != nil {
				dl.LogToolStart(debug.ToolEventData{
					ToolName:   toolCall.Name,
					ToolCallID: toolCall.ID,
					Input:      toolCall.Input,
				})
			}

			toolResult, toolErr := tool.Run(ctx, tools.ToolCall{
				ID:    toolCall.ID,
				Name:  toolCall.Name,
				Input: toolCall.Input,
			})

			// Debug: log tool end
			if dl != nil {
				ted := debug.ToolEventData{
					ToolName:   toolCall.Name,
					ToolCallID: toolCall.ID,
					DurationMs: time.Since(toolStart).Milliseconds(),
				}
				if toolErr != nil {
					ted.IsError = true
					ted.Error = toolErr.Error()
				} else {
					ted.Output = toolResult.Content
					ted.IsError = toolResult.IsError
					ted.MetadataSummary = toolMetadataSummary(toolResult.Metadata, len(toolResult.Content))
				}
				dl.LogToolEnd(ted)
			}

			// Apply tool result size budget (truncate oversized outputs).
			if toolErr == nil && !toolResult.IsError {
				maxBytes := tool.Info().MaxResultBytes
				if maxBytes == 0 {
					maxBytes = defaultMaxResultBytes
				}
				if maxBytes > 0 && len(toolResult.Content) > maxBytes {
					omitted := len(toolResult.Content) - maxBytes
					toolResult.Content = toolResult.Content[:maxBytes] + fmt.Sprintf("\n\n[truncated: %d bytes omitted]", omitted)
				}
			}

			if toolErr != nil {
				if errors.Is(toolErr, permission.ErrorPermissionDenied) {
					var msg string
					if pe, ok := permission.AsPermissionError(toolErr); ok {
						msg = pe.Result.UserMessage()
					} else {
						msg = "Permission denied.\n\nOptions:\n" +
							"1. Use AskUser to explain why this is needed and request permission\n" +
							"2. Try an alternative approach\n3. Skip this step"
					}
					assistantMsg.ErrorToolCall(toolCall.ID)
					a.messages.Update(ctx, assistantMsg)
					a.Publish(pubsub.CreatedEvent, AgentEvent{
						Type:      AgentEventTypeToolFailed,
						SessionID: assistantMsg.SessionID,
						Message:   assistantMsg,
						ToolLifecycle: &ToolLifecycleEvent{
							ToolCallID:    toolCall.ID,
							ToolName:      toolCall.Name,
							BatchID:       toolCall.BatchID,
							Input:         toolCall.Input,
							State:         message.ToolCallErrored,
							Order:         toolCall.Order,
							Total:         len(toolCalls),
							DurationMs:    time.Since(toolStart).Milliseconds(),
							ResultSummary: summarizeToolResult(msg, true),
						},
					})
					toolResults[i] = message.ToolResult{
						ToolCallID: toolCall.ID,
						Content:    msg,
						IsError:    true,
					}
					// Cancel remaining tools in this batch
					for j := i + 1; j < len(toolCalls); j++ {
						assistantMsg.CancelToolCall(toolCalls[j].ID)
						toolResults[j] = message.ToolResult{
							ToolCallID: toolCalls[j].ID,
							Content:    "Canceled (prior tool in batch was denied permission)",
							IsError:    true,
						}
					}
					// Persist before publishing events
					a.messages.Update(ctx, assistantMsg)
					for j := i + 1; j < len(toolCalls); j++ {
						a.Publish(pubsub.CreatedEvent, AgentEvent{
							Type:      AgentEventTypeToolCanceled,
							SessionID: assistantMsg.SessionID,
							Message:   assistantMsg,
							ToolLifecycle: &ToolLifecycleEvent{
								ToolCallID: toolCalls[j].ID,
								ToolName:   toolCalls[j].Name,
								BatchID:    toolCalls[j].BatchID,
								Input:      toolCalls[j].Input,
								State:      message.ToolCallCanceled,
								Order:      toolCalls[j].Order,
								Total:      len(toolCalls),
							},
						})
					}
					break toolLoop // 退出批次，但不终止 agent 循环
				}
			}

			// Determine success vs error/canceled for state transition.
			isCanceled := toolErr != nil && errors.Is(toolErr, context.Canceled)
			isToolError := (toolErr != nil && !isCanceled) || toolResult.IsError
			if isCanceled {
				assistantMsg.CancelToolCall(toolCall.ID)
				a.messages.Update(ctx, assistantMsg)
				a.Publish(pubsub.CreatedEvent, AgentEvent{
					Type:      AgentEventTypeToolCanceled,
					SessionID: assistantMsg.SessionID,
					Message:   assistantMsg,
					ToolLifecycle: &ToolLifecycleEvent{
						ToolCallID: toolCall.ID,
						ToolName:   toolCall.Name,
						BatchID:    toolCall.BatchID,
						Input:      toolCall.Input,
						State:      message.ToolCallCanceled,
						Order:      toolCall.Order,
						Total:      len(toolCalls),
						DurationMs: time.Since(toolStart).Milliseconds(),
					},
				})
			} else if isToolError {
				assistantMsg.ErrorToolCall(toolCall.ID)
				a.messages.Update(ctx, assistantMsg)
				a.Publish(pubsub.CreatedEvent, AgentEvent{
					Type:      AgentEventTypeToolFailed,
					SessionID: assistantMsg.SessionID,
					Message:   assistantMsg,
					ToolLifecycle: &ToolLifecycleEvent{
						ToolCallID:    toolCall.ID,
						ToolName:      toolCall.Name,
						BatchID:       toolCall.BatchID,
						Input:         toolCall.Input,
						State:         message.ToolCallErrored,
						Order:         toolCall.Order,
						Total:         len(toolCalls),
						DurationMs:    time.Since(toolStart).Milliseconds(),
						ResultSummary: summarizeToolResult(toolResult.Content, true),
					},
				})
			} else {
				assistantMsg.FinishToolCall(toolCall.ID)
				a.messages.Update(ctx, assistantMsg)
				a.Publish(pubsub.CreatedEvent, AgentEvent{
					Type:      AgentEventTypeToolFinished,
					SessionID: assistantMsg.SessionID,
					Message:   assistantMsg,
					ToolLifecycle: &ToolLifecycleEvent{
						ToolCallID:    toolCall.ID,
						ToolName:      toolCall.Name,
						BatchID:       toolCall.BatchID,
						Input:         toolCall.Input,
						State:         message.ToolCallCompleted,
						Order:         toolCall.Order,
						Total:         len(toolCalls),
						DurationMs:    time.Since(toolStart).Milliseconds(),
						ResultSummary: summarizeToolResult(toolResult.Content, false),
					},
				})
			}

			toolResults[i] = message.ToolResult{
				ToolCallID: toolCall.ID,
				Name:       toolCall.Name,
				Content:    toolResult.Content,
				Metadata:   toolResult.Metadata,
				IsError:    toolResult.IsError,
			}
			if cooldownScope := scholarCooldownScopeFromToolResult(toolCall, toolResults[i]); cooldownScope != "" {
				batchCooldownScopes[cooldownScope] = struct{}{}
			}

			a.runPostToolHooks(ctx, toolCall, toolResults[i], toolErr)
		}
	}

out:
	if len(toolResults) == 0 {
		return assistantMsg, nil, nil
	}

	for i := range toolResults {
		toolName := toolResults[i].Name
		if toolName == "" {
			for _, tc := range toolCalls {
				if tc.ID == toolResults[i].ToolCallID {
					toolName = tc.Name
					break
				}
			}
		}
		compact := tools.CompactToolResponse(toolName, tools.ToolResponse{
			Content:  toolResults[i].Content,
			Metadata: toolResults[i].Metadata,
			IsError:  toolResults[i].IsError,
		})
		toolResults[i].Content = compact.Content
	}

	parts := make([]message.ContentPart, 0, len(toolResults))
	for _, tr := range toolResults {
		parts = append(parts, tr)
	}

	msg, err := a.messages.Create(context.Background(), assistantMsg.SessionID, message.CreateMessageParams{
		Role:  message.Tool,
		Parts: parts,
	})
	if err != nil {
		return assistantMsg, nil, fmt.Errorf("failed to create tool message: %w", err)
	}

	return assistantMsg, &msg, nil
}

type parallelToolExecution struct {
	index     int
	toolCall  message.ToolCall
	tool      tools.BaseTool
	startedAt time.Time
	response  tools.ToolResponse
	err       error
	debugLog  *debug.SessionLogger
	shouldRun bool
}

func shouldExecuteScholarBatchInParallel(toolCalls []message.ToolCall) bool {
	if len(toolCalls) < 2 {
		return false
	}
	for _, tc := range toolCalls {
		if !scholarToolCallParallelSafe(tc) {
			return false
		}
	}
	return true
}

func scholarToolCallParallelSafe(tc message.ToolCall) bool {
	if tc.Name != "ScholarSearch" {
		return false
	}
	var params struct {
		Action string `json:"action"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal([]byte(tc.Input), &params); err != nil {
		return false
	}
	action := strings.TrimSpace(params.Action)
	if action == "" {
		action = "search"
	}
	if action != "search" {
		return false
	}
	switch strings.TrimSpace(params.Source) {
	case "openalex", "crossref", "pubmed", "core", "eric", "patentsview", "europepmc", "unpaywall":
		return true
	default:
		return false
	}
}

func (a *agent) executeParallelScholarToolCalls(ctx context.Context, assistantMsg *message.Message, toolCalls []message.ToolCall, toolResults []message.ToolResult, execBudget *toolExecutionBudget, hasExecBudget bool, runtimeAllowedSet map[string]struct{}) {
	batchDuplicateSignatures := map[string]struct{}{}
	executions := make([]parallelToolExecution, 0, len(toolCalls))
	for i, toolCall := range toolCalls {
		if a.permissionService != nil && assistantMsg.SessionID != "" {
			ctx = permission.WithMode(ctx, a.permissionService.SessionMode(assistantMsg.SessionID))
		}
		select {
		case <-ctx.Done():
			assistantMsg.CancelToolCall(toolCall.ID)
			toolResults[i] = message.ToolResult{
				ToolCallID: toolCall.ID,
				Content:    "Tool execution canceled",
				IsError:    true,
			}
			a.Publish(pubsub.CreatedEvent, AgentEvent{
				Type:      AgentEventTypeToolCanceled,
				SessionID: assistantMsg.SessionID,
				Message:   *assistantMsg,
				ToolLifecycle: &ToolLifecycleEvent{
					ToolCallID: toolCall.ID,
					ToolName:   toolCall.Name,
					BatchID:    toolCall.BatchID,
					Input:      toolCall.Input,
					State:      message.ToolCallCanceled,
					Order:      toolCall.Order,
					Total:      len(toolCalls),
				},
			})
			continue
		default:
		}
		if hasExecBudget && shouldSkipToolForBudget(toolCall, execBudget) {
			a.recordSyntheticToolError(ctx, assistantMsg, toolCall, &toolResults[i],
				"Skipped tool call because the generation tool budget is exhausted.",
				syntheticToolMetadata(toolCall.Name, "tool_budget_exceeded", canonicalToolCallSignature(toolCall)))
			continue
		}
		dupSig := canonicalToolCallSignature(toolCall)
		if _, ok := batchDuplicateSignatures[dupSig]; ok {
			a.recordSyntheticToolError(ctx, assistantMsg, toolCall, &toolResults[i],
				"Skipped duplicate tool call; use the previous result.",
				syntheticToolMetadata(toolCall.Name, "duplicate_tool_call", dupSig))
			continue
		}
		batchDuplicateSignatures[dupSig] = struct{}{}
		if hasExecBudget {
			consumeToolBudget(toolCall, execBudget)
		}
		if len(runtimeAllowedSet) > 0 {
			if _, ok := runtimeAllowedSet[toolCall.Name]; !ok {
				a.recordSyntheticToolError(ctx, assistantMsg, toolCall, &toolResults[i],
					fmt.Sprintf("Tool not allowed by runtime override: %s", toolCall.Name),
					syntheticToolMetadata(toolCall.Name, "runtime_tool_not_allowed", dupSig))
				continue
			}
		}
		tool := a.findTool(toolCall.Name)
		if tool == nil {
			a.recordSyntheticToolError(ctx, assistantMsg, toolCall, &toolResults[i],
				fmt.Sprintf("Tool not found: %s", toolCall.Name),
				syntheticToolMetadata(toolCall.Name, "tool_not_found", dupSig))
			continue
		}
		if err := tools.ValidateResearchToolCall(ctx, toolCall.Name); err != nil {
			a.recordSyntheticToolError(ctx, assistantMsg, toolCall, &toolResults[i],
				err.Error(), syntheticToolMetadata(toolCall.Name, "research_tool_not_allowed", dupSig))
			continue
		}
		if permission.CurrentMode(ctx) == permission.ModePlan && !tools.PlanModeToolAllowed(toolCall.Name) {
			a.recordSyntheticToolError(ctx, assistantMsg, toolCall, &toolResults[i],
				fmt.Sprintf("Tool '%s' is not available in plan mode. Use read-only exploration tools, update only the plan file, then call ExitPlanMode for approval.", toolCall.Name),
				syntheticToolMetadata(toolCall.Name, "plan_mode_tool_not_allowed", dupSig))
			continue
		}
		if a.hookService != nil {
			hookInput := buildHookInput(ctx, toolCall.Name, toolCall.Input)
			if err := a.hookService.RunBlocking(ctx, hooks.PreToolUse, hookInput); err != nil {
				a.recordSyntheticToolError(ctx, assistantMsg, toolCall, &toolResults[i],
					"[Hook blocked] "+err.Error(),
					syntheticToolMetadata(toolCall.Name, "hook_blocked", dupSig))
				continue
			}
		}

		assistantMsg.StartToolCall(toolCall.ID)
		a.Publish(pubsub.CreatedEvent, AgentEvent{
			Type:      AgentEventTypeToolStarted,
			SessionID: assistantMsg.SessionID,
			Message:   *assistantMsg,
			ToolLifecycle: &ToolLifecycleEvent{
				ToolCallID: toolCall.ID,
				ToolName:   toolCall.Name,
				BatchID:    toolCall.BatchID,
				Input:      toolCall.Input,
				State:      message.ToolCallRunning,
				Order:      toolCall.Order,
				Total:      len(toolCalls),
			},
		})
		dl := debugLoggerFromCtx(ctx)
		startedAt := time.Now()
		if dl != nil {
			dl.LogToolStart(debug.ToolEventData{
				ToolName:   toolCall.Name,
				ToolCallID: toolCall.ID,
				Input:      toolCall.Input,
			})
		}
		executions = append(executions, parallelToolExecution{
			index:     i,
			toolCall:  toolCall,
			tool:      tool,
			startedAt: startedAt,
			debugLog:  dl,
			shouldRun: true,
		})
	}
	a.messages.Update(ctx, *assistantMsg)

	var wg sync.WaitGroup
	sem := make(chan struct{}, defaultParallelScholarToolConcurrency())
	for i := range executions {
		if !executions[i].shouldRun {
			continue
		}
		wg.Add(1)
		go func(exec *parallelToolExecution) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				exec.err = ctx.Err()
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			exec.response, exec.err = exec.tool.Run(ctx, tools.ToolCall{
				ID:    exec.toolCall.ID,
				Name:  exec.toolCall.Name,
				Input: exec.toolCall.Input,
			})
		}(&executions[i])
	}
	wg.Wait()

	for i := range executions {
		exec := executions[i]
		toolCall := exec.toolCall
		toolResult := exec.response
		toolErr := exec.err
		if exec.debugLog != nil {
			ted := debug.ToolEventData{
				ToolName:   toolCall.Name,
				ToolCallID: toolCall.ID,
				DurationMs: time.Since(exec.startedAt).Milliseconds(),
			}
			if toolErr != nil {
				ted.IsError = true
				ted.Error = toolErr.Error()
			} else {
				ted.Output = toolResult.Content
				ted.IsError = toolResult.IsError
				ted.MetadataSummary = toolMetadataSummary(toolResult.Metadata, len(toolResult.Content))
			}
			exec.debugLog.LogToolEnd(ted)
		}
		if toolErr == nil && !toolResult.IsError {
			maxBytes := exec.tool.Info().MaxResultBytes
			if maxBytes == 0 {
				maxBytes = defaultMaxResultBytes
			}
			if maxBytes > 0 && len(toolResult.Content) > maxBytes {
				omitted := len(toolResult.Content) - maxBytes
				toolResult.Content = toolResult.Content[:maxBytes] + fmt.Sprintf("\n\n[truncated: %d bytes omitted]", omitted)
			}
		}

		isCanceled := toolErr != nil && errors.Is(toolErr, context.Canceled)
		isToolError := (toolErr != nil && !isCanceled) || toolResult.IsError
		switch {
		case isCanceled:
			assistantMsg.CancelToolCall(toolCall.ID)
			a.Publish(pubsub.CreatedEvent, AgentEvent{
				Type:      AgentEventTypeToolCanceled,
				SessionID: assistantMsg.SessionID,
				Message:   *assistantMsg,
				ToolLifecycle: &ToolLifecycleEvent{
					ToolCallID: toolCall.ID,
					ToolName:   toolCall.Name,
					BatchID:    toolCall.BatchID,
					Input:      toolCall.Input,
					State:      message.ToolCallCanceled,
					Order:      toolCall.Order,
					Total:      len(toolCalls),
					DurationMs: time.Since(exec.startedAt).Milliseconds(),
				},
			})
		case isToolError:
			assistantMsg.ErrorToolCall(toolCall.ID)
			a.Publish(pubsub.CreatedEvent, AgentEvent{
				Type:      AgentEventTypeToolFailed,
				SessionID: assistantMsg.SessionID,
				Message:   *assistantMsg,
				ToolLifecycle: &ToolLifecycleEvent{
					ToolCallID:    toolCall.ID,
					ToolName:      toolCall.Name,
					BatchID:       toolCall.BatchID,
					Input:         toolCall.Input,
					State:         message.ToolCallErrored,
					Order:         toolCall.Order,
					Total:         len(toolCalls),
					DurationMs:    time.Since(exec.startedAt).Milliseconds(),
					ResultSummary: summarizeToolResult(toolResult.Content, true),
				},
			})
		default:
			assistantMsg.FinishToolCall(toolCall.ID)
			a.Publish(pubsub.CreatedEvent, AgentEvent{
				Type:      AgentEventTypeToolFinished,
				SessionID: assistantMsg.SessionID,
				Message:   *assistantMsg,
				ToolLifecycle: &ToolLifecycleEvent{
					ToolCallID:    toolCall.ID,
					ToolName:      toolCall.Name,
					BatchID:       toolCall.BatchID,
					Input:         toolCall.Input,
					State:         message.ToolCallCompleted,
					Order:         toolCall.Order,
					Total:         len(toolCalls),
					DurationMs:    time.Since(exec.startedAt).Milliseconds(),
					ResultSummary: summarizeToolResult(toolResult.Content, false),
				},
			})
		}
		toolResults[exec.index] = message.ToolResult{
			ToolCallID: toolCall.ID,
			Name:       toolCall.Name,
			Content:    toolResult.Content,
			Metadata:   toolResult.Metadata,
			IsError:    toolResult.IsError,
		}
		a.runPostToolHooks(ctx, toolCall, toolResults[exec.index], toolErr)
	}
	a.messages.Update(ctx, *assistantMsg)
}

func defaultParallelScholarToolConcurrency() int {
	return 4
}

func shouldSkipToolForBudget(toolCall message.ToolCall, budget *toolExecutionBudget) bool {
	if budget == nil {
		return false
	}
	if budget.RemainingToolCalls != -1 && budget.RemainingToolCalls <= 0 {
		return true
	}
	if isSearchFamilyTool(toolCall.Name) && budget.RemainingSearchCalls != -1 && budget.RemainingSearchCalls <= 0 {
		return true
	}
	return false
}

func consumeToolBudget(toolCall message.ToolCall, budget *toolExecutionBudget) {
	if budget == nil {
		return
	}
	if budget.RemainingToolCalls > 0 {
		budget.RemainingToolCalls--
	}
	if isSearchFamilyTool(toolCall.Name) && budget.RemainingSearchCalls > 0 {
		budget.RemainingSearchCalls--
	}
}

func canonicalToolCallSignature(toolCall message.ToolCall) string {
	return strings.ToLower(strings.TrimSpace(toolCall.Name)) + ":" + toolInputHash(toolCall.Input)
}

func syntheticToolMetadata(toolName, errorKind, signature string) string {
	md, _ := json.Marshal(map[string]any{
		"tool":          toolName,
		"error_kind":    errorKind,
		"progress_kind": "none",
		"recoverable":   false,
		"signature":     signature,
	})
	return string(md)
}

func toolMetadataSummary(raw string, resultBytes int) map[string]any {
	summary := map[string]any{"result_bytes": resultBytes}
	if strings.TrimSpace(raw) == "" {
		return summary
	}
	var md map[string]any
	if err := json.Unmarshal([]byte(raw), &md); err != nil {
		return summary
	}
	for _, key := range []string{"error_kind", "progress_kind", "provider", "source", "target_key", "query_key", "content_class", "low_value_reason"} {
		if v, ok := md[key]; ok {
			summary[key] = v
		}
	}
	if evidence := collectStringArray(md, "evidence_keys"); len(evidence) > 0 {
		summary["evidence_count"] = len(evidence)
	}
	if attempts, ok := md["attempts"]; ok {
		if arr, ok := attempts.([]any); ok {
			summary["attempt_count"] = len(arr)
		}
	}
	return summary
}

func (a *agent) recordSyntheticToolError(ctx context.Context, assistantMsg *message.Message, toolCall message.ToolCall, result *message.ToolResult, content, metadata string) {
	assistantMsg.ErrorToolCall(toolCall.ID)
	a.messages.Update(ctx, *assistantMsg)
	a.Publish(pubsub.CreatedEvent, AgentEvent{
		Type:      AgentEventTypeToolFailed,
		SessionID: assistantMsg.SessionID,
		Message:   *assistantMsg,
		ToolLifecycle: &ToolLifecycleEvent{
			ToolCallID:    toolCall.ID,
			ToolName:      toolCall.Name,
			BatchID:       toolCall.BatchID,
			Input:         toolCall.Input,
			State:         message.ToolCallErrored,
			Order:         toolCall.Order,
			Total:         len(assistantMsg.ToolCalls()),
			ResultSummary: summarizeToolResult(content, true),
		},
	})
	*result = message.ToolResult{
		ToolCallID: toolCall.ID,
		Name:       toolCall.Name,
		Content:    content,
		Metadata:   metadata,
		IsError:    true,
	}
	if dl := debugLoggerFromCtx(ctx); dl != nil {
		dl.LogToolEnd(debug.ToolEventData{
			ToolName:        toolCall.Name,
			ToolCallID:      toolCall.ID,
			IsError:         true,
			Output:          content,
			MetadataSummary: toolMetadataSummary(metadata, len(content)),
		})
	}
	a.runPostToolHooks(ctx, toolCall, *result, nil)
}

func (a *agent) runPostToolHooks(ctx context.Context, toolCall message.ToolCall, toolResult message.ToolResult, toolErr error) {
	if a == nil || a.hookService == nil {
		return
	}
	postEvent := hooks.PostToolUse
	if toolErr != nil || toolResult.IsError {
		postEvent = hooks.PostToolUseFailure
	}
	postInput := buildHookInput(ctx, toolCall.Name, toolCall.Input)
	postInput.ToolResult = toolResult.Content
	postInput.IsError = toolResult.IsError
	applyHookResultMetadata(&postInput, toolResult.Metadata)
	a.hookService.RunAsync(ctx, postEvent, postInput)
	if postInput.ErrorKind == "rate_limited" || postInput.ErrorKind == "provider_cooldown" {
		a.hookService.RunAsync(ctx, hooks.ProviderCooldown, postInput)
	}
	if !postInput.IsError && len(postInput.ArtifactPaths) > 0 {
		a.hookService.RunAsync(ctx, hooks.ArtifactCreated, postInput)
	}
}

func scholarShouldSkipForBatchCooldown(toolCall message.ToolCall, cooldownScopes map[string]struct{}) bool {
	if toolCall.Name != "ScholarSearch" {
		return false
	}
	source, action := parseScholarInput(toolCall.Input)
	if action == "" {
		action = "search"
	}
	if scholarActionUsesSemanticSource(action) {
		source = "semantic_scholar"
	}
	if source == "" {
		source = "semantic_scholar"
	}
	scope := strings.ToLower(source + ":" + source)
	if _, ok := cooldownScopes[scope]; !ok {
		return false
	}
	switch action {
	case "details", "citations", "references", "download":
		return true
	case "search", "":
		return true
	default:
		return false
	}
}

func scholarCooldownSkipContent(rawInput string) string {
	source, action := parseScholarInput(rawInput)
	if scholarActionUsesSemanticSource(action) {
		source = "semantic_scholar"
	}
	if source == "" || source == "semantic_scholar" {
		return "Skipped: Semantic Scholar provider cooldown already detected in this tool batch. Use source=openalex/arxiv/crossref or retry later."
	}
	return fmt.Sprintf("Skipped: %s provider cooldown already detected in this tool batch. Use another source or retry later.", source)
}

func scholarActionUsesSemanticSource(action string) bool {
	switch action {
	case "details", "citations", "references", "download":
		return true
	default:
		return false
	}
}

func parseScholarInput(raw string) (source string, action string) {
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return "", ""
	}
	if sv, ok := v["source"].(string); ok {
		source = strings.ToLower(strings.TrimSpace(sv))
	}
	if av, ok := v["action"].(string); ok {
		action = strings.ToLower(strings.TrimSpace(av))
	}
	return source, action
}

func scholarCooldownSkipMetadata(rawInput string) string {
	source, action := parseScholarInput(rawInput)
	if source == "" {
		source = "semantic_scholar"
	}
	if action == "" {
		action = "search"
	}
	if scholarActionUsesSemanticSource(action) {
		source = "semantic_scholar"
	}
	suggestedSources := scholarSuggestedSourcesExcluding(source)
	md, _ := json.Marshal(map[string]any{
		"error_kind":        "provider_cooldown",
		"tool":              "ScholarSearch",
		"provider":          source,
		"source":            source,
		"recoverable":       true,
		"action":            action,
		"query_key":         "cooldown:" + source,
		"suggested_sources": suggestedSources,
	})
	return string(md)
}

func scholarSuggestedSourcesExcluding(source string) []string {
	all := []string{"semantic_scholar", "openalex", "arxiv", "crossref"}
	out := make([]string, 0, len(all)-1)
	for _, candidate := range all {
		if candidate == source {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func scholarCooldownScopeFromToolResult(toolCall message.ToolCall, result message.ToolResult) string {
	if toolCall.Name != "ScholarSearch" || !result.IsError || result.Metadata == "" {
		return ""
	}
	var md map[string]any
	if err := json.Unmarshal([]byte(result.Metadata), &md); err != nil {
		return ""
	}
	kind, _ := md["error_kind"].(string)
	if kind != "rate_limited" && kind != "provider_cooldown" {
		return ""
	}
	provider, _ := md["provider"].(string)
	source, _ := md["source"].(string)
	if provider == "" || source == "" {
		return ""
	}
	return strings.ToLower(provider + ":" + source)
}

// buildHookInput constructs a hooks.Input from the current context and tool call info.
func buildHookInput(ctx context.Context, toolName, rawInput string) hooks.Input {
	var toolInput map[string]any
	if rawInput != "" {
		_ = json.Unmarshal([]byte(rawInput), &toolInput) // best-effort parse
	}
	return hooks.Input{
		SessionID: getSessionID(ctx),
		ToolName:  toolName,
		ToolInput: toolInput,
		Timestamp: time.Now(),
	}
}

func applyHookResultMetadata(input *hooks.Input, raw string) {
	if input == nil || strings.TrimSpace(raw) == "" {
		return
	}
	var md map[string]any
	if err := json.Unmarshal([]byte(raw), &md); err != nil {
		return
	}
	input.Provider, _ = md["provider"].(string)
	input.Source, _ = md["source"].(string)
	input.ErrorKind, _ = md["error_kind"].(string)
	input.ProgressKind, _ = md["progress_kind"].(string)
	if goal, _ := md["goal_type"].(string); goal != "" {
		input.GoalType = goal
	}
	input.ArtifactPaths = collectHookArtifactPaths(md)
}

func collectHookArtifactPaths(md map[string]any) []string {
	out := make([]string, 0, 4)
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		for _, existing := range out {
			if existing == v {
				return
			}
		}
		out = append(out, v)
	}
	for _, key := range []string{"path", "svg_path", "png_path", "d2_path"} {
		if v, _ := md[key].(string); v != "" {
			add(v)
		}
	}
	if artifact, ok := md["artifact"].(map[string]any); ok {
		if v, _ := artifact["path"].(string); v != "" {
			add(v)
		}
	}
	if artifacts, ok := md["artifacts"].([]any); ok {
		for _, item := range artifacts {
			if artifact, ok := item.(map[string]any); ok {
				if v, _ := artifact["path"].(string); v != "" {
					add(v)
				}
			}
		}
	}
	if paths, ok := md["artifact_paths"].([]any); ok {
		for _, item := range paths {
			if v, ok := item.(string); ok {
				add(v)
			}
		}
	}
	return out
}

func summarizeToolResult(content string, isErr bool) string {
	s := strings.TrimSpace(strings.ReplaceAll(content, "\n", " "))
	if s == "" {
		if isErr {
			return "error"
		}
		return "done"
	}
	const max = 80
	runes := []rune(s)
	if len(runes) > max {
		s = string(runes[:max-1]) + "…"
	}
	return s
}
