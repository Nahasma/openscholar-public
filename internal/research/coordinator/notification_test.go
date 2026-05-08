package coordinator

import (
	"strings"
	"testing"
)

func TestFormatPhaseNotification_Completed(t *testing.T) {
	phase := PhaseRef{ID: "p1", Name: "Literature Review", Order: 1}
	next := &PhaseRef{ID: "p2", Name: "Methodology Design", Order: 2}

	got := FormatPhaseNotification("task-abc", NotifyPhaseCompleted, phase, next, "Phase 1 完成，已生成文献综述。")

	// Verify root element attributes.
	if !strings.Contains(got, `task-id="task-abc"`) {
		t.Errorf("missing task-id attribute, got:\n%s", got)
	}
	if !strings.Contains(got, `type="phase-completed"`) {
		t.Errorf("missing type attribute, got:\n%s", got)
	}

	// Verify phase element.
	if !strings.Contains(got, `name="Literature Review"`) {
		t.Errorf("missing phase name, got:\n%s", got)
	}
	if !strings.Contains(got, `order="1"`) {
		t.Errorf("missing phase order, got:\n%s", got)
	}
	if !strings.Contains(got, `status="completed"`) {
		t.Errorf("missing phase status, got:\n%s", got)
	}

	// Verify next-phase element.
	if !strings.Contains(got, `<next-phase`) {
		t.Errorf("missing next-phase element, got:\n%s", got)
	}
	if !strings.Contains(got, `name="Methodology Design"`) {
		t.Errorf("missing next-phase name, got:\n%s", got)
	}
	if !strings.Contains(got, `order="2"`) {
		t.Errorf("missing next-phase order, got:\n%s", got)
	}

	// Verify summary.
	if !strings.Contains(got, "Phase 1 完成，已生成文献综述。") {
		t.Errorf("missing summary text, got:\n%s", got)
	}
}

func TestFormatPhaseNotification_Failed(t *testing.T) {
	phase := PhaseRef{ID: "p3", Name: "Experiment", Order: 3}

	got := FormatPhaseNotification("task-xyz", NotifyPhaseFailed, phase, nil, "Experiment phase failed due to timeout.")

	if !strings.Contains(got, `type="phase-failed"`) {
		t.Errorf("missing type attribute, got:\n%s", got)
	}
	if !strings.Contains(got, `status="failed"`) {
		t.Errorf("missing failed status, got:\n%s", got)
	}

	// next-phase must be absent.
	if strings.Contains(got, "<next-phase") {
		t.Errorf("unexpected next-phase element in failed notification, got:\n%s", got)
	}

	if !strings.Contains(got, "Experiment phase failed due to timeout.") {
		t.Errorf("missing summary text, got:\n%s", got)
	}
}

func TestFormatPhaseNotification_Checkpoint(t *testing.T) {
	phase := PhaseRef{ID: "p2", Name: "Methodology Design", Order: 2}

	got := FormatPhaseNotification("task-chk", NotifyCheckpoint, phase, nil, "Checkpoint: awaiting user approval.")

	if !strings.Contains(got, `type="checkpoint"`) {
		t.Errorf("missing checkpoint type, got:\n%s", got)
	}
	if !strings.Contains(got, `status="checkpoint"`) {
		t.Errorf("missing checkpoint status, got:\n%s", got)
	}
	if !strings.Contains(got, "Checkpoint: awaiting user approval.") {
		t.Errorf("missing summary text, got:\n%s", got)
	}
}

func TestFormatPhaseNotification_XMLEscape(t *testing.T) {
	// Phase name and summary contain XML special characters.
	phase := PhaseRef{ID: "p1", Name: `Analysis & "Review" <v2>`, Order: 1}
	summary := `Result: a < b && c > d, status="ok"`

	got := FormatPhaseNotification("task-esc", NotifyPhaseStarted, phase, nil, summary)

	// The raw special characters must NOT appear unescaped inside attributes or content.
	// encoding/xml encodes & → &amp;  < → &lt;  > → &gt;  " → &#34; or &quot;
	if strings.Contains(got, `name="Analysis & "`) {
		t.Errorf("unescaped & or \" in attribute, got:\n%s", got)
	}
	if strings.Contains(got, `<v2>`) {
		t.Errorf("unescaped angle brackets in attribute, got:\n%s", got)
	}

	// The document must still be valid XML: verify by checking escaped forms exist.
	if !strings.Contains(got, "&amp;") {
		t.Errorf("expected &amp; in output for '&', got:\n%s", got)
	}
	if !strings.Contains(got, "&lt;") {
		t.Errorf("expected &lt; in output for '<', got:\n%s", got)
	}
}
