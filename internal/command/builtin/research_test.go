package builtin

import "testing"

func TestParseStartArgs_ParsesBooleanFlags(t *testing.T) {
	topic, flags := parseStartArgs(`"topic" --template aris_empirical --auto-write --human-checkpoint true --effort max`)
	if topic != "topic" {
		t.Fatalf("topic = %q", topic)
	}
	if flags["template"] != "aris_empirical" {
		t.Fatalf("template = %q", flags["template"])
	}
	if flags["auto-write"] != "true" {
		t.Fatalf("auto-write = %q", flags["auto-write"])
	}
	if flags["human-checkpoint"] != "true" {
		t.Fatalf("human-checkpoint = %q", flags["human-checkpoint"])
	}
	if flags["effort"] != "max" {
		t.Fatalf("effort = %q", flags["effort"])
	}
}

func TestResearchRunPolicyFromFlags_DerivesSubmission(t *testing.T) {
	p := researchRunPolicyFromFlags(map[string]string{
		"effort":              "max",
		"reviewer-difficulty": "hard",
		"auto-write":          "true",
		"human-checkpoint":    "true",
		"batch":               "queue",
		"trace":               "meta",
		"max-review-rounds":   "6",
	})

	if p.Assurance != "submission" {
		t.Fatalf("assurance = %q", p.Assurance)
	}
	if p.ReviewerDifficulty != "hard" || !p.AutoWrite || !p.HumanCheckpoint || p.AutoProceed || p.BatchPolicy != "queue" || p.TraceMode != "meta" || p.MaxReviewRounds != 6 {
		t.Fatalf("unexpected policy: %#v", p)
	}
}
