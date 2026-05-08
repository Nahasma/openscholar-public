package command

import "testing"

func TestResultNormalizedActions_AdaptsLegacyAndTyped(t *testing.T) {
	res := Result{
		Action:  "screen:fullscreen",
		Actions: []CommandAction{{Kind: CommandActionScreen, Target: "fullscreen"}, {Kind: CommandActionCD}},
	}
	actions := res.NormalizedActions()
	if len(actions) != 2 {
		t.Fatalf("actions len = %d, want 2", len(actions))
	}
	if actions[0].Kind != CommandActionScreen || actions[1].Kind != CommandActionCD {
		t.Fatalf("unexpected actions: %#v", actions)
	}
}
