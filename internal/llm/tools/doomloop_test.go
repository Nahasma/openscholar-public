package tools

import (
	"testing"
)

func TestDoomLoopDefaultRecordCall(t *testing.T) {
	d := NewDoomLoopDetector()

	// 少于 window 个调用，不应触发
	for i := 0; i < 4; i++ {
		if d.RecordCall("tool:a") {
			t.Fatalf("should not detect loop before window is filled (call %d)", i+1)
		}
	}

	// 第 5 次：calls = [a,a,a,a,a]，last=a，前4个中有4个a，repeats=4 >= 3
	if !d.RecordCall("tool:a") {
		t.Fatal("expected loop detection at 5th repeated call")
	}
}

func TestDoomLoopDefaultRecordCallNoLoop(t *testing.T) {
	d := NewDoomLoopDetector()

	sigs := []string{"a", "b", "c", "d", "e"}
	for _, s := range sigs {
		if d.RecordCall(s) {
			t.Fatalf("distinct calls should not trigger loop detection, sig=%s", s)
		}
	}
}

func TestDoomLoopCustomConfig(t *testing.T) {
	// window=3, threshold=2：最近3次中同一签名出现2次即触发
	d := NewDoomLoopDetectorWithConfig(3, 2)

	// 第1次，不足 window
	if d.RecordCall("x") {
		t.Fatal("should not detect loop on first call")
	}
	// 第2次，不足 window
	if d.RecordCall("x") {
		t.Fatal("should not detect loop on second call (window=3)")
	}
	// 第3次：calls=[x,x,x], last=x, 前2个中有2个x, repeats=2 >= 2 → true
	if !d.RecordCall("x") {
		t.Fatal("expected loop detection with custom config at 3rd repeated call")
	}
}

func TestDoomLoopObserveReadonly(t *testing.T) {
	d := NewDoomLoopDetector()

	// 预填充 2 个调用
	d.RecordCall("a")
	d.RecordCall("b")

	// Observe 追加 [a, a, a]：combined=[a,b,a,a,a]，最后5个中 a 出现4次 >= 3
	sigs := []string{"a", "a", "a"}
	if !d.Observe(sigs) {
		t.Fatal("Observe should detect loop pattern")
	}
}

func TestDoomLoopObserveDoesNotModifyState(t *testing.T) {
	d := NewDoomLoopDetector()
	d.RecordCall("x")
	d.RecordCall("y")

	// 调用 Observe 前后，内部 calls 长度不变
	d.mu.Lock()
	lenBefore := len(d.calls)
	d.mu.Unlock()

	d.Observe([]string{"a", "b", "c"})

	d.mu.Lock()
	lenAfter := len(d.calls)
	d.mu.Unlock()

	if lenBefore != lenAfter {
		t.Fatalf("Observe must not modify internal state: before=%d after=%d", lenBefore, lenAfter)
	}
}

func TestDoomLoopObserveNoLoop(t *testing.T) {
	d := NewDoomLoopDetector()
	// combined = [a,b,c,d,e]，每个签名出现1次，不触发
	if d.Observe([]string{"a", "b", "c", "d", "e"}) {
		t.Fatal("Observe should not detect loop for distinct signatures")
	}
}

func TestDoomLoopReset(t *testing.T) {
	d := NewDoomLoopDetector()

	// 触发 loop
	for i := 0; i < 5; i++ {
		d.RecordCall("sig")
	}

	d.Reset()

	// Reset 后内部 calls 应为空
	d.mu.Lock()
	n := len(d.calls)
	d.mu.Unlock()

	if n != 0 {
		t.Fatalf("Reset should clear calls, got len=%d", n)
	}

	// Reset 后重新记录不应立即触发
	if d.RecordCall("sig") {
		t.Fatal("should not detect loop immediately after Reset")
	}
}
