package components

import "testing"

func TestSmoothTokenStep_SmallGap(t *testing.T) {
	// gap < 20: increment by max(1, gap/7)
	// gap = 14: 14/7 = 2, so displayed+2
	result := SmoothTokenStep(100, 114)
	if result != 102 {
		t.Errorf("gap=14: expected 102, got %d", result)
	}

	// gap = 1: max(1, 1/7=0) = 1
	result = SmoothTokenStep(100, 101)
	if result != 101 {
		t.Errorf("gap=1: expected 101, got %d", result)
	}

	// gap = 7: 7/7=1, increment=1
	result = SmoothTokenStep(100, 107)
	if result != 101 {
		t.Errorf("gap=7: expected 101, got %d", result)
	}
}

func TestSmoothTokenStep_MediumGap(t *testing.T) {
	// gap in [20, 70): increment by 3
	result := SmoothTokenStep(100, 150)
	if result != 103 {
		t.Errorf("gap=50: expected 103, got %d", result)
	}

	result = SmoothTokenStep(0, 20)
	if result != 3 {
		t.Errorf("gap=20: expected 3, got %d", result)
	}

	result = SmoothTokenStep(0, 69)
	if result != 3 {
		t.Errorf("gap=69: expected 3, got %d", result)
	}
}

func TestSmoothTokenStep_LargeGap(t *testing.T) {
	// gap >= 200: increment by 50
	result := SmoothTokenStep(0, 500)
	if result != 50 {
		t.Errorf("gap=500: expected 50, got %d", result)
	}

	result = SmoothTokenStep(100, 300)
	if result != 150 {
		t.Errorf("gap=200: expected 150, got %d", result)
	}

	// gap in [70, 200): increment by max(8, gap*15/100)
	// gap=100: 100*15/100=15, max(8,15)=15
	result = SmoothTokenStep(0, 100)
	if result != 15 {
		t.Errorf("gap=100: expected 15, got %d", result)
	}

	// gap=70: 70*15/100=10, max(8,10)=10
	result = SmoothTokenStep(0, 70)
	if result != 10 {
		t.Errorf("gap=70: expected 10, got %d", result)
	}

	// gap=50 is medium but let's test gap=199: 199*15/100=29
	result = SmoothTokenStep(0, 199)
	if result != 29 {
		t.Errorf("gap=199: expected 29, got %d", result)
	}
}

func TestSmoothTokenStep_NoGap(t *testing.T) {
	// displayed == target: no change
	result := SmoothTokenStep(100, 100)
	if result != 100 {
		t.Errorf("no gap: expected 100, got %d", result)
	}

	result = SmoothTokenStep(0, 0)
	if result != 0 {
		t.Errorf("no gap zero: expected 0, got %d", result)
	}
}

func TestSmoothTokenStep_NeverExceedsTarget(t *testing.T) {
	cases := []struct {
		displayed int
		target    int
	}{
		{0, 1},
		{99, 100},
		{0, 500},
		{450, 460},
		{100, 115},
	}

	for _, c := range cases {
		result := SmoothTokenStep(c.displayed, c.target)
		if result > c.target {
			t.Errorf("displayed=%d target=%d: result %d exceeds target", c.displayed, c.target, result)
		}
	}

	// Also test safety: displayed > target (gap < 0)
	result := SmoothTokenStep(200, 100)
	if result != 100 {
		t.Errorf("negative gap safety: expected 100, got %d", result)
	}
}

func TestFormatTokenLabel_Wide(t *testing.T) {
	// width >= 48: with arrow prefix and "tokens"
	result := FormatTokenLabel(128, 80)
	expected := "↓ 128 tokens"
	if result != expected {
		t.Errorf("wide: expected %q, got %q", expected, result)
	}

	result = FormatTokenLabel(128, 72)
	if result != expected {
		t.Errorf("width=72: expected %q, got %q", expected, result)
	}
}

func TestFormatTokenLabel_Medium(t *testing.T) {
	// width 48-71: still shows arrow prefix and "tokens" (unified format)
	result := FormatTokenLabel(128, 60)
	expected := "↓ 128 tokens"
	if result != expected {
		t.Errorf("medium: expected %q, got %q", expected, result)
	}

	result = FormatTokenLabel(128, 48)
	if result != expected {
		t.Errorf("width=48: expected %q, got %q", expected, result)
	}

	result = FormatTokenLabel(128, 71)
	if result != expected {
		t.Errorf("width=71: expected %q, got %q", expected, result)
	}
}

func TestFormatTokenLabel_Narrow(t *testing.T) {
	// width < 48: empty string
	result := FormatTokenLabel(128, 47)
	if result != "" {
		t.Errorf("narrow width=47: expected empty, got %q", result)
	}

	result = FormatTokenLabel(128, 0)
	if result != "" {
		t.Errorf("narrow width=0: expected empty, got %q", result)
	}
}

func TestFormatTokenLabel_Large(t *testing.T) {
	// count >= 1000: CC compact notation (1.2k, 2.5k, etc.)
	result := FormatTokenLabel(1200, 80)
	expected := "↓ 1.2k tokens"
	if result != expected {
		t.Errorf("1200 wide: expected %q, got %q", expected, result)
	}

	result = FormatTokenLabel(1200, 60)
	expected = "↓ 1.2k tokens"
	if result != expected {
		t.Errorf("1200 medium: expected %q, got %q", expected, result)
	}

	result = FormatTokenLabel(2500, 80)
	expected = "↓ 2.5k tokens"
	if result != expected {
		t.Errorf("2500 wide: expected %q, got %q", expected, result)
	}

	result = FormatTokenLabel(1000, 80)
	expected = "↓ 1k tokens"
	if result != expected {
		t.Errorf("1000 wide: expected %q, got %q", expected, result)
	}
}
