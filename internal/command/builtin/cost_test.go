package builtin

import "testing"

func TestFormatCostUnknown(t *testing.T) {
	if got := formatCost(0, false); got != "unknown" {
		t.Fatalf("formatCost unknown = %q", got)
	}
	if got := formatCost(0.12, false); got != "unknown (known subtotal $0.1200)" {
		t.Fatalf("formatCost subtotal = %q", got)
	}
}
