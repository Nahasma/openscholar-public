package skillbank

import (
	"strings"
	"testing"
)

func TestParseSkillContent_RichMetadataAndAliases(t *testing.T) {
	raw := `---
name: "skill name"
description: "desc"
category: "workflow"
tags: ["a", "b"]
version: 2
author: "user"
when-to-use: "use me"
allowed_tools: ["bash", "view"]
agent: "coder"
effort: "medium"
model: "gpt-5"
user_invocable: true
disable_model_invocation: true
paths: ["internal/skillbank/**"]
platforms: ["darwin"]
source: "bundle"
---
instruction body
`
	skill, err := ParseSkillContent("workflow/test", []byte(raw))
	if err != nil {
		t.Fatalf("ParseSkillContent error: %v", err)
	}
	if skill.WhenToUse != "use me" {
		t.Fatalf("when_to_use mismatch: %q", skill.WhenToUse)
	}
	if len(skill.AllowedTools) != 2 || skill.AllowedTools[0] != "bash" {
		t.Fatalf("allowed-tools mismatch: %#v", skill.AllowedTools)
	}
	if !skill.UserInvocable || !skill.DisableModelInvocation {
		t.Fatalf("bool metadata mismatch: user-invocable=%v disable-model-invocation=%v", skill.UserInvocable, skill.DisableModelInvocation)
	}
	if skill.Exposure != SkillExposureExplicit {
		t.Fatalf("exposure mismatch: %q", skill.Exposure)
	}
	if skill.Source != "bundle" {
		t.Fatalf("source mismatch: %q", skill.Source)
	}
	if skill.Instruction != "instruction body" {
		t.Fatalf("instruction mismatch: %q", skill.Instruction)
	}
}

func TestSkill_ToMarkdown_IncludesRichMetadataCanonicalKeys(t *testing.T) {
	skill := Skill{
		Name:                   "name",
		Description:            "desc",
		Category:               "memory",
		Version:                1,
		Author:                 "system",
		WhenToUse:              "when",
		AllowedTools:           []string{"SkillQuery"},
		Exposure:               SkillExposureExplicit,
		UserInvocable:          true,
		DisableModelInvocation: true,
		Instruction:            "body",
	}
	md := skill.ToMarkdown()
	for _, key := range []string{"when_to_use:", "allowed-tools:", "exposure:", "user-invocable:", "disable-model-invocation:"} {
		if !strings.Contains(md, key) {
			t.Fatalf("missing key %q in markdown:\n%s", key, md)
		}
	}
}
