package components

import (
	"strings"
	"testing"
)

func TestRenderRuntimeErrorDetailCapsWrappedHeight(t *testing.T) {
	rendered := RenderRuntimeErrorDetail("summary", strings.Repeat("minimax-auth-detail ", 80), 40)
	if h := MeasureRenderedHeight(rendered); h > 10 {
		t.Fatalf("rendered height=%d, want <= 10 rows including border:\n%s", h, rendered)
	}
	if !strings.Contains(rendered, "truncated") {
		t.Fatalf("expected truncation marker in rendered detail:\n%s", rendered)
	}
}
