package components

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// ToolPresenter defines tool-specific display strategies for TUI rendering.
// Each tool type provides its own noun, label, and param formatting.
type ToolPresenter interface {
	// DisplayName returns the user-facing tool name (e.g. "Read", "Bash").
	DisplayName() string
	// GroupNoun returns the count + noun for group headers (e.g. "3 files", "2 commands").
	GroupNoun(total, finished int, allDone bool) string
	// RunningLabel returns the action verb shown while running (e.g. "reading...").
	RunningLabel() string
	// FormatParams extracts and formats tool input params for display.
	FormatParams(input string) string
}

// GetPresenter returns the ToolPresenter for the given tool name.
func GetPresenter(toolName string) ToolPresenter {
	switch toolName {
	case "View", "Read":
		return readPresenter{}
	case "Edit":
		return editPresenter{}
	case "Write":
		return writePresenter{}
	case "Bash":
		return bashPresenter{}
	case "Glob":
		return globPresenter{}
	case "Grep":
		return grepPresenter{}
	case "WebSearch":
		return webSearchPresenter{}
	case "WebFetch":
		return webFetchPresenter{}
	case "Task":
		return taskPresenter{}
	case "ScholarSearch", "ArxivSearch", "CrossrefSearch", "PubmedSearch", "OpenAlexSearch":
		return scholarPresenter{name: toolName}
	case "KBQuery", "KBSearch":
		return kbQueryPresenter{name: toolName}
	case "KBAdd":
		return kbAddPresenter{}
	case "KBList", "KBTree":
		return kbListPresenter{name: toolName}
	case "ToolSearch":
		return toolSearchPresenter{}
	case "DocExport":
		return docExportPresenter{}
	case "ImageGen":
		return imageGenPresenter{}
	case "DiagramGen":
		return diagramGenPresenter{}
	case "AskUser":
		return askUserPresenter{}
	case "RecordFeedback":
		return recordFeedbackPresenter{}
	case "SkillQuery":
		return skillQueryPresenter{}
	case "SkillManage":
		return skillManagePresenter{}
	case "PaperValidate":
		return paperValidatePresenter{}
	case "ResearchControl", "ResearchPipeline":
		return researchControlPresenter{name: toolName}
	case "ResearchTask":
		return researchTaskPresenter{}
	case "ResearchMessage":
		return researchMessagePresenter{}
	default:
		return genericPresenter{name: toolName}
	}
}

// FormatToolIntent formats tool input into a short user-facing intent string.
// Returns empty string when no concise representation is available.
func FormatToolIntent(toolName string, input string) string {
	return GetPresenter(toolName).FormatParams(input)
}

// FormatToolBatchLabel returns the Claude Code-like batch header label.
func FormatToolBatchLabel(toolName string, total, finished int, allDone bool) string {
	p := GetPresenter(toolName)
	return p.DisplayName() + "  " + p.GroupNoun(total, finished, allDone)
}

// FormatToolDetail returns the best short detail line for live progress or completion.
func FormatToolDetail(toolName, input, resultSummary string, state string) string {
	if state == "completed" || state == "errored" || state == "canceled" {
		if resultSummary != "" {
			return resultSummary
		}
		if state == "canceled" {
			return "canceled"
		}
	}
	if intent := FormatToolIntent(toolName, input); intent != "" {
		return intent
	}
	switch state {
	case "queued":
		return "queued"
	case "running":
		return GetPresenter(toolName).RunningLabel()
	case "errored":
		return "failed"
	case "completed":
		return "completed"
	case "canceled":
		return "canceled"
	default:
		return ""
	}
}

// --- helper ---

func parseInputJSON(input string) map[string]any {
	if input == "" {
		return nil
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return nil
	}
	return params
}

func truncPresenter(s string, max int) string {
	if max <= 0 {
		max = 60
	}
	s = strings.ReplaceAll(s, "\n", " ")
	// Use rune-aware truncation to avoid cutting multi-byte characters (CJK, emoji)
	runes := []rune(s)
	if len(runes) > max {
		return string(runes[:max-1]) + "…"
	}
	return s
}

// --- Read (View) ---

type readPresenter struct{}

func (readPresenter) DisplayName() string { return "Read" }
func (readPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d files", total)
	}
	return fmt.Sprintf("%d/%d files", finished, total)
}
func (readPresenter) RunningLabel() string { return "reading..." }
func (readPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if fp, ok := p["file_path"].(string); ok {
			return truncatePathMiddle(fp, 60)
		}
	}
	return ""
}

// --- Edit ---

type editPresenter struct{}

func (editPresenter) DisplayName() string { return "Edit" }
func (editPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d files", total)
	}
	return fmt.Sprintf("%d/%d files", finished, total)
}
func (editPresenter) RunningLabel() string { return "editing..." }
func (editPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if fp, ok := p["file_path"].(string); ok {
			return truncatePathMiddle(fp, 60)
		}
	}
	return ""
}

// --- Write ---

type writePresenter struct{}

func (writePresenter) DisplayName() string { return "Write" }
func (writePresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d files", total)
	}
	return fmt.Sprintf("%d/%d files", finished, total)
}
func (writePresenter) RunningLabel() string { return "writing..." }
func (writePresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if fp, ok := p["file_path"].(string); ok {
			return truncatePathMiddle(fp, 60)
		}
	}
	return ""
}

// --- Bash ---

type bashPresenter struct{}

func (bashPresenter) DisplayName() string { return "Bash" }
func (bashPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d commands", total)
	}
	return fmt.Sprintf("%d/%d commands", finished, total)
}
func (bashPresenter) RunningLabel() string { return "running..." }
func (bashPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if cmd, ok := p["command"].(string); ok {
			return truncPresenter(cmd, 60)
		}
	}
	return ""
}

// --- Glob ---

type globPresenter struct{}

func (globPresenter) DisplayName() string { return "Glob" }
func (globPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d patterns", total)
	}
	return fmt.Sprintf("%d/%d patterns", finished, total)
}
func (globPresenter) RunningLabel() string { return "finding files..." }
func (globPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if pattern, ok := p["pattern"].(string); ok {
			return pattern
		}
	}
	return ""
}

// --- Grep ---

type grepPresenter struct{}

func (grepPresenter) DisplayName() string { return "Grep" }
func (grepPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d patterns", total)
	}
	return fmt.Sprintf("%d/%d patterns", finished, total)
}
func (grepPresenter) RunningLabel() string { return "searching..." }
func (grepPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if pattern, ok := p["pattern"].(string); ok {
			if path, ok := p["path"].(string); ok {
				return pattern + " in " + path
			}
			return pattern
		}
	}
	return ""
}

// --- WebSearch ---

type webSearchPresenter struct{}

func (webSearchPresenter) DisplayName() string { return "WebSearch" }
func (webSearchPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d searches", total)
	}
	return fmt.Sprintf("%d/%d searches", finished, total)
}
func (webSearchPresenter) RunningLabel() string { return "searching web..." }
func (webSearchPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if q, ok := p["query"].(string); ok {
			return truncPresenter(q, 60)
		}
	}
	return ""
}

// --- WebFetch ---

type webFetchPresenter struct{}

func (webFetchPresenter) DisplayName() string { return "WebFetch" }
func (webFetchPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d fetches", total)
	}
	return fmt.Sprintf("%d/%d fetches", finished, total)
}
func (webFetchPresenter) RunningLabel() string { return "fetching..." }
func (webFetchPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if u, ok := p["url"].(string); ok {
			return truncPresenter(u, 60)
		}
	}
	return ""
}

// --- Task ---

type taskPresenter struct{}

func (taskPresenter) DisplayName() string { return "Task" }
func (taskPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d tasks", total)
	}
	return fmt.Sprintf("%d/%d tasks", finished, total)
}
func (taskPresenter) RunningLabel() string { return "running task..." }
func (taskPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if desc, ok := p["description"].(string); ok {
			return truncPresenter(desc, 60)
		}
		if prompt, ok := p["prompt"].(string); ok {
			return truncPresenter(prompt, 60)
		}
	}
	return ""
}

// --- Scholar searches ---

type scholarPresenter struct{ name string }

func (s scholarPresenter) DisplayName() string { return s.name }
func (s scholarPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d searches", total)
	}
	return fmt.Sprintf("%d/%d searches", finished, total)
}
func (scholarPresenter) RunningLabel() string { return "searching papers..." }
func (scholarPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if q, ok := p["query"].(string); ok {
			return truncPresenter(q, 50)
		}
	}
	return ""
}

// --- KB Query/Search ---

type kbQueryPresenter struct{ name string }

func (k kbQueryPresenter) DisplayName() string { return k.name }
func (k kbQueryPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d queries", total)
	}
	return fmt.Sprintf("%d/%d queries", finished, total)
}
func (kbQueryPresenter) RunningLabel() string { return "querying KB..." }
func (kbQueryPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if q, ok := p["question"].(string); ok {
			return truncPresenter(q, 50)
		}
	}
	return ""
}

// --- KB Add ---

type kbAddPresenter struct{}

func (kbAddPresenter) DisplayName() string { return "KBAdd" }
func (kbAddPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d files", total)
	}
	return fmt.Sprintf("%d/%d files", finished, total)
}
func (kbAddPresenter) RunningLabel() string { return "adding to KB..." }
func (kbAddPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if fp, ok := p["file_path"].(string); ok {
			return filepath.Base(fp)
		}
	}
	return ""
}

// --- KB List/Tree ---

type kbListPresenter struct{ name string }

func (k kbListPresenter) DisplayName() string { return k.name }
func (k kbListPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d items", total)
	}
	return fmt.Sprintf("%d/%d items", finished, total)
}
func (k kbListPresenter) RunningLabel() string {
	if k.name == "KBTree" {
		return "building tree..."
	}
	return "listing KB..."
}
func (kbListPresenter) FormatParams(_ string) string { return "" }

// --- ToolSearch ---

type toolSearchPresenter struct{}

func (toolSearchPresenter) DisplayName() string { return "ToolSearch" }
func (toolSearchPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d searches", total)
	}
	return fmt.Sprintf("%d/%d searches", finished, total)
}
func (toolSearchPresenter) RunningLabel() string { return "searching tools..." }
func (toolSearchPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if q, ok := p["query"].(string); ok {
			return truncPresenter(q, 60)
		}
	}
	return ""
}

// --- DocExport ---

type docExportPresenter struct{}

func (docExportPresenter) DisplayName() string { return "DocExport" }
func (docExportPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d exports", total)
	}
	return fmt.Sprintf("%d/%d exports", finished, total)
}
func (docExportPresenter) RunningLabel() string { return "exporting..." }
func (docExportPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if fp, ok := p["input_path"].(string); ok {
			return filepath.Base(fp)
		}
	}
	return ""
}

// --- ImageGen ---

type imageGenPresenter struct{}

func (imageGenPresenter) DisplayName() string { return "ImageGen" }
func (imageGenPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d images", total)
	}
	return fmt.Sprintf("%d/%d images", finished, total)
}
func (imageGenPresenter) RunningLabel() string { return "generating image..." }
func (imageGenPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if prompt, ok := p["prompt"].(string); ok {
			return truncPresenter(prompt, 50)
		}
	}
	return ""
}

// --- DiagramGen ---

type diagramGenPresenter struct{}

func (diagramGenPresenter) DisplayName() string { return "DiagramGen" }
func (diagramGenPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d diagrams", total)
	}
	return fmt.Sprintf("%d/%d diagrams", finished, total)
}
func (diagramGenPresenter) RunningLabel() string { return "generating diagram..." }
func (diagramGenPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if code, ok := p["code"].(string); ok {
			return truncPresenter(code, 50)
		}
	}
	return ""
}

// --- AskUser ---

type askUserPresenter struct{}

func (askUserPresenter) DisplayName() string { return "AskUser" }
func (askUserPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d questions", total)
	}
	return fmt.Sprintf("%d/%d questions", finished, total)
}
func (askUserPresenter) RunningLabel() string { return "asking..." }
func (askUserPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if q, ok := p["question"].(string); ok {
			return truncPresenter(q, 50)
		}
	}
	return ""
}

// --- RecordFeedback ---

type recordFeedbackPresenter struct{}

func (recordFeedbackPresenter) DisplayName() string { return "RecordFeedback" }
func (recordFeedbackPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d items", total)
	}
	return fmt.Sprintf("%d/%d items", finished, total)
}
func (recordFeedbackPresenter) RunningLabel() string { return "recording..." }
func (recordFeedbackPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if f, ok := p["feedback"].(string); ok {
			return truncPresenter(f, 40)
		}
	}
	return ""
}

// --- SkillQuery ---

type skillQueryPresenter struct{}

func (skillQueryPresenter) DisplayName() string { return "SkillQuery" }
func (skillQueryPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d queries", total)
	}
	return fmt.Sprintf("%d/%d queries", finished, total)
}
func (skillQueryPresenter) RunningLabel() string { return "querying skills..." }
func (skillQueryPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if q, ok := p["query"].(string); ok {
			return truncPresenter(q, 50)
		}
	}
	return ""
}

// --- SkillManage ---

type skillManagePresenter struct{}

func (skillManagePresenter) DisplayName() string { return "SkillManage" }
func (skillManagePresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d items", total)
	}
	return fmt.Sprintf("%d/%d items", finished, total)
}
func (skillManagePresenter) RunningLabel() string { return "managing skills..." }
func (skillManagePresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if action, ok := p["action"].(string); ok {
			return action
		}
	}
	return ""
}

// --- PaperValidate ---

type paperValidatePresenter struct{}

func (paperValidatePresenter) DisplayName() string { return "PaperValidate" }
func (paperValidatePresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d validations", total)
	}
	return fmt.Sprintf("%d/%d validations", finished, total)
}
func (paperValidatePresenter) RunningLabel() string { return "validating..." }
func (paperValidatePresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if wd, ok := p["work_dir"].(string); ok {
			return filepath.Base(wd)
		}
	}
	return ""
}

// --- ResearchControl/Pipeline ---

type researchControlPresenter struct{ name string }

func (r researchControlPresenter) DisplayName() string { return r.name }
func (r researchControlPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d items", total)
	}
	return fmt.Sprintf("%d/%d items", finished, total)
}
func (researchControlPresenter) RunningLabel() string { return "controlling research..." }
func (researchControlPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if action, ok := p["action"].(string); ok {
			return action
		}
	}
	return ""
}

// --- ResearchTask ---

type researchTaskPresenter struct{}

func (researchTaskPresenter) DisplayName() string { return "ResearchTask" }
func (researchTaskPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d tasks", total)
	}
	return fmt.Sprintf("%d/%d tasks", finished, total)
}
func (researchTaskPresenter) RunningLabel() string { return "managing tasks..." }
func (researchTaskPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if action, ok := p["action"].(string); ok {
			return action
		}
	}
	return ""
}

// --- ResearchMessage ---

type researchMessagePresenter struct{}

func (researchMessagePresenter) DisplayName() string { return "ResearchMessage" }
func (researchMessagePresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d messages", total)
	}
	return fmt.Sprintf("%d/%d messages", finished, total)
}
func (researchMessagePresenter) RunningLabel() string { return "messaging..." }
func (researchMessagePresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		if action, ok := p["action"].(string); ok {
			return action
		}
	}
	return ""
}

// --- Generic fallback ---

type genericPresenter struct{ name string }

func (g genericPresenter) DisplayName() string { return g.name }
func (g genericPresenter) GroupNoun(total, finished int, allDone bool) string {
	if allDone {
		return fmt.Sprintf("%d items", total)
	}
	return fmt.Sprintf("%d/%d items", finished, total)
}
func (genericPresenter) RunningLabel() string { return "processing..." }
func (genericPresenter) FormatParams(input string) string {
	if p := parseInputJSON(input); p != nil {
		// Smart fallback: try common keys
		for _, key := range []string{"query", "description", "file_path", "name", "command", "prompt", "input", "url", "pattern", "question"} {
			if v, ok := p[key].(string); ok && v != "" {
				return truncPresenter(v, 60)
			}
		}
	}
	return ""
}
