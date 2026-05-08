package components

import "testing"

func TestBodyWidths_RespectTerminalSafeWidthOnNarrowTerminals(t *testing.T) {
	for _, w := range []int{8, 10, 20} {
		safe := TerminalSafeWidth(w)
		if bw := BodyWidth(w); bw > safe {
			t.Fatalf("BodyWidth(%d)=%d exceeds safe width %d", w, bw, safe)
		}
		if cw := ContinuationBodyWidth(w); cw > safe {
			t.Fatalf("ContinuationBodyWidth(%d)=%d exceeds safe width %d", w, cw, safe)
		}
		if rw := ResponseBodyWidth(w); rw > safe {
			t.Fatalf("ResponseBodyWidth(%d)=%d exceeds safe width %d", w, rw, safe)
		}
	}
}
