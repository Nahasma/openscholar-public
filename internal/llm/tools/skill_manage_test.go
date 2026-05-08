package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/skillbank"
)

func TestSkillManageTool_UpdateCanClearRichMetadata(t *testing.T) {
	conn := setupSkillQueryTestDB(t)
	userDir := t.TempDir()
	writeSkillFileForQueryTest(t, filepath.Join(userDir, "workflow", "manage_target.md"), `---
name: "manage target"
description: "metadata clear target"
category: "workflow"
tags: ["manage"]
when_to_use: "before clear"
allowed-tools: ["View", "Edit"]
agent: "coder"
effort: "high"
model: "gpt-5.4"
user-invocable: true
disable-model-invocation: true
paths: ["**/*.md"]
platforms: ["darwin"]
source: "bundle"
---
instruction body should remain
`)

	svc := skillbank.NewService(conn, userDir, nil)
	tool := NewSkillManageTool(svc)

	call := ToolCall{
		Name: "SkillManage",
		Input: `{
			"action":"update",
			"id":"workflow/manage_target",
			"when_to_use":"",
			"allowed_tools":[],
			"agent":"",
			"effort":"",
			"model":"",
			"user_invocable":false,
			"disable_model_invocation":false,
			"paths":[],
			"platforms":[],
			"source":""
		}`,
	}
	resp, err := tool.Run(context.Background(), call)
	if err != nil {
		t.Fatalf("run update: %v", err)
	}
	if resp.IsError {
		t.Fatalf("update failed: %s", resp.Content)
	}

	view, err := svc.View(context.Background(), "workflow/manage_target")
	if err != nil {
		t.Fatalf("view updated skill: %v", err)
	}
	if view.Name != "manage target" {
		t.Fatalf("name changed unexpectedly: %q", view.Name)
	}
	if view.Instruction != "instruction body should remain" {
		t.Fatalf("instruction changed unexpectedly: %q", view.Instruction)
	}
	if view.WhenToUse != "" || view.Agent != "" || view.Effort != "" || view.Model != "" || view.Source != "" {
		t.Fatalf("string metadata not cleared: %#v", view)
	}
	if len(view.AllowedTools) != 0 || len(view.Paths) != 0 || len(view.Platforms) != 0 {
		t.Fatalf("slice metadata not cleared: %#v", view)
	}
	if view.UserInvocable || view.DisableModelInvocation {
		t.Fatalf("bool metadata not cleared: %#v", view)
	}
}

func TestSkillManageTool_InstallListAndInfo(t *testing.T) {
	conn := setupSkillQueryTestDB(t)
	userDir := t.TempDir()
	src := filepath.Join(t.TempDir(), "workflow", "managed_skill.md")
	writeSkillFileForQueryTest(t, src, `---
name: "managed skill"
description: "managed governance target"
category: "workflow"
source: "bundle"
---
managed instruction body
`)

	svc := skillbank.NewService(conn, userDir, nil)
	tool := NewSkillManageTool(svc)

	installResp, err := tool.Run(context.Background(), ToolCall{
		Name:  "SkillManage",
		Input: `{"action":"install","local_path":"` + src + `"}`,
	})
	if err != nil {
		t.Fatalf("run install: %v", err)
	}
	if installResp.IsError {
		t.Fatalf("install failed: %s", installResp.Content)
	}
	if !strings.Contains(installResp.Content, "Installed skill 'workflow/managed_skill'.") {
		t.Fatalf("unexpected install response: %s", installResp.Content)
	}

	listResp, err := tool.Run(context.Background(), ToolCall{
		Name:  "SkillManage",
		Input: `{"action":"list","category":"workflow"}`,
	})
	if err != nil {
		t.Fatalf("run list: %v", err)
	}
	if listResp.IsError {
		t.Fatalf("list failed: %s", listResp.Content)
	}
	if !strings.Contains(listResp.Content, "workflow/managed_skill") || !strings.Contains(listResp.Content, "user-managed/local-file") {
		t.Fatalf("unexpected list response: %s", listResp.Content)
	}

	infoResp, err := tool.Run(context.Background(), ToolCall{
		Name:  "SkillManage",
		Input: `{"action":"info","id":"workflow/managed_skill"}`,
	})
	if err != nil {
		t.Fatalf("run info: %v", err)
	}
	if infoResp.IsError {
		t.Fatalf("info failed: %s", infoResp.Content)
	}
	for _, want := range []string{
		"## managed skill [workflow/managed_skill]",
		"Active source: user-managed / local-file",
		"Declared source: bundle",
	} {
		if !strings.Contains(infoResp.Content, want) {
			t.Fatalf("info response missing %q:\n%s", want, infoResp.Content)
		}
	}

	syncResp, err := tool.Run(context.Background(), ToolCall{
		Name:  "SkillManage",
		Input: `{"action":"sync","id":"workflow/managed_skill"}`,
	})
	if err != nil {
		t.Fatalf("run sync: %v", err)
	}
	if syncResp.IsError {
		t.Fatalf("sync failed: %s", syncResp.Content)
	}
	if !strings.Contains(syncResp.Content, "Synced skill 'workflow/managed_skill'.") {
		t.Fatalf("unexpected sync response: %s", syncResp.Content)
	}

	uninstallResp, err := tool.Run(context.Background(), ToolCall{
		Name:  "SkillManage",
		Input: `{"action":"uninstall","id":"workflow/managed_skill"}`,
	})
	if err != nil {
		t.Fatalf("run uninstall: %v", err)
	}
	if uninstallResp.IsError {
		t.Fatalf("uninstall failed: %s", uninstallResp.Content)
	}
	if !strings.Contains(uninstallResp.Content, "Uninstalled skill 'workflow/managed_skill'.") {
		t.Fatalf("unexpected uninstall response: %s", uninstallResp.Content)
	}
}

func TestSkillManageTool_InstallBlockedInResearchMode(t *testing.T) {
	conn := setupSkillQueryTestDB(t)
	userDir := t.TempDir()
	src := filepath.Join(t.TempDir(), "workflow", "blocked_skill.md")
	writeSkillFileForQueryTest(t, src, `---
name: "blocked skill"
description: "blocked governance target"
category: "workflow"
---
blocked instruction body
`)

	svc := skillbank.NewService(conn, userDir, nil)
	tool := NewSkillManageTool(svc)
	ctx := context.WithValue(context.Background(), ResearchModeContextKey, true)

	resp, err := tool.Run(ctx, ToolCall{
		Name:  "SkillManage",
		Input: `{"action":"install","local_path":"` + src + `"}`,
	})
	if err != nil {
		t.Fatalf("run install: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected research mode install to be blocked: %#v", resp)
	}
	if !strings.Contains(resp.Content, "Skill modification is not allowed in research mode") {
		t.Fatalf("unexpected block message: %s", resp.Content)
	}
}

func TestSkillManageTool_CreateRejectsExactDuplicateID(t *testing.T) {
	conn := setupSkillQueryTestDB(t)
	userDir := t.TempDir()
	writeSkillFileForQueryTest(t, filepath.Join(userDir, "workflow", "my_skill.md"), `---
name: "my skill"
description: "existing"
category: "workflow"
---
existing body
`)

	svc := skillbank.NewService(conn, userDir, nil)
	tool := NewSkillManageTool(svc)

	resp, err := tool.Run(context.Background(), ToolCall{
		Name: "SkillManage",
		Input: `{
			"action":"create",
			"name":"My Skill",
			"category":"workflow",
			"instruction":"new body"
		}`,
	})
	if err != nil {
		t.Fatalf("run create: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected duplicate create to fail, got: %#v", resp)
	}
	if !strings.Contains(resp.Content, "already exists") || !strings.Contains(resp.Content, "action=update") {
		t.Fatalf("unexpected duplicate refusal: %s", resp.Content)
	}
}

func TestSkillManageTool_CreateRejectsInvalidCategoryBeforeDuplicateLookup(t *testing.T) {
	conn := setupSkillQueryTestDB(t)
	userDir := t.TempDir()
	outsidePath := filepath.Join(filepath.Dir(userDir), "outside", "leaked.md")
	writeSkillFileForQueryTest(t, outsidePath, `---
name: "leaked"
description: "outside skill root"
category: "workflow"
---
outside body
`)

	svc := skillbank.NewService(conn, userDir, nil)
	tool := NewSkillManageTool(svc)

	resp, err := tool.Run(context.Background(), ToolCall{
		Name: "SkillManage",
		Input: `{
			"action":"create",
			"name":"leaked",
			"category":"../outside",
			"instruction":"new body"
		}`,
	})
	if err != nil {
		t.Fatalf("run create: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected invalid category create to fail, got: %#v", resp)
	}
	if !strings.Contains(resp.Content, "invalid category") {
		t.Fatalf("expected invalid category refusal, got: %s", resp.Content)
	}
	if strings.Contains(resp.Content, "already exists") {
		t.Fatalf("invalid category should be rejected before duplicate lookup, got: %s", resp.Content)
	}
}
