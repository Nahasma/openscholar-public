package permission

import (
	"testing"
)

func TestDenialTracker_Record(t *testing.T) {
	tracker := NewDenialTracker()
	tracker.RecordDenial("Write", "write file", "/src/main.go", "", SourceUser, ReasonUserDenied)

	if tracker.ConsecutiveDenials() != 1 {
		t.Fatalf("expected 1 denial, got %d", tracker.ConsecutiveDenials())
	}
}

func TestDenialTracker_Reset(t *testing.T) {
	tracker := NewDenialTracker()
	tracker.RecordDenial("Write", "write file", "/src/main.go", "", SourceUser, ReasonUserDenied)
	tracker.RecordDenial("Edit", "edit file", "/src/main.go", "", SourceUser, ReasonUserDenied)
	tracker.Reset()

	if tracker.ConsecutiveDenials() != 0 {
		t.Fatalf("expected 0 denials after reset, got %d", tracker.ConsecutiveDenials())
	}
}

func TestDenialTracker_WriteToEditDowngrade(t *testing.T) {
	tracker := NewDenialTracker()

	// User denies Write on /src/main.go
	tracker.RecordDenial("Write", "write file", "/src/main.go", "", SourceUser, ReasonUserDenied)

	// Model tries Edit on same file — should be blocked
	isDown, reason := tracker.IsDowngrade("Edit", "edit file", "/src/main.go")
	if !isDown {
		t.Fatal("expected Write→Edit downgrade to be detected")
	}
	if reason == "" {
		t.Fatal("expected non-empty reason")
	}
}

func TestDenialTracker_WriteToBashRedirectDowngrade(t *testing.T) {
	tracker := NewDenialTracker()

	// User denies Write on /src/main.go
	tracker.RecordDenial("Write", "write file", "/src/main.go", "", SourceUser, ReasonUserDenied)

	// Model tries Bash echo > same file — should be blocked
	isDown, _ := tracker.IsDowngrade("Bash", "echo hello > /src/main.go", "/src/main.go")
	if !isDown {
		t.Fatal("expected Write→Bash redirect downgrade to be detected")
	}
}

func TestDenialTracker_DifferentFileNotDowngrade(t *testing.T) {
	tracker := NewDenialTracker()

	// User denies Write on /src/main.go
	tracker.RecordDenial("Write", "write file", "/src/main.go", "", SourceUser, ReasonUserDenied)

	// Model tries Edit on different file — should NOT be blocked
	isDown, _ := tracker.IsDowngrade("Edit", "edit file", "/src/other.go")
	if isDown {
		t.Fatal("expected different file to NOT be detected as downgrade")
	}
}

func TestDenialTracker_ReadToolNotDowngrade(t *testing.T) {
	tracker := NewDenialTracker()

	// User denies View (read tool)
	tracker.RecordDenial("View", "view file", "/src/main.go", "", SourceUser, ReasonUserDenied)

	// View → Grep is not a write downgrade
	isDown, _ := tracker.IsDowngrade("Grep", "grep pattern", "/src/main.go")
	if isDown {
		t.Fatal("expected read tool denial to NOT trigger downgrade detection")
	}
}

func TestDenialTracker_EditToWriteDowngrade(t *testing.T) {
	tracker := NewDenialTracker()

	// User denies Edit on /src/main.go
	tracker.RecordDenial("Edit", "edit file", "/src/main.go", "", SourceUser, ReasonUserDenied)

	// Model tries Write on same file — should be blocked (Edit→Write is also a write→write downgrade)
	isDown, _ := tracker.IsDowngrade("Write", "write file", "/src/main.go")
	if !isDown {
		t.Fatal("expected Edit→Write downgrade to be detected")
	}
}

func TestBashTargetsFile(t *testing.T) {
	tests := []struct {
		command string
		file    string
		targets bool
	}{
		{"echo hello > /src/main.go", "/src/main.go", true},
		{"cat data >> /src/main.go", "/src/main.go", true},
		{"tee /src/main.go", "/src/main.go", true},
		{"echo hello > /other/file.go", "/src/main.go", false},
		{"git status", "/src/main.go", false},
		{"", "/src/main.go", false},
		{"echo hello", "", false},
	}

	for _, tt := range tests {
		got := bashTargetsFile(tt.command, tt.file)
		if got != tt.targets {
			t.Errorf("bashTargetsFile(%q, %q) = %v, want %v", tt.command, tt.file, got, tt.targets)
		}
	}
}

func TestDenialTracker_ConsecutiveResetsOnGrant(t *testing.T) {
	tracker := NewDenialTracker()

	tracker.RecordDenial("Write", "write", "/src/a.go", "", SourceUser, ReasonUserDenied)
	tracker.RecordDenial("Write", "write", "/src/b.go", "", SourceUser, ReasonUserDenied)
	if tracker.ConsecutiveDenials() != 2 {
		t.Fatalf("expected 2, got %d", tracker.ConsecutiveDenials())
	}

	tracker.RecordGrant("")
	if tracker.ConsecutiveDenials() != 0 {
		t.Fatalf("expected 0 after grant, got %d", tracker.ConsecutiveDenials())
	}

	tracker.RecordDenial("Edit", "edit", "/src/c.go", "", SourceUser, ReasonUserDenied)
	if tracker.ConsecutiveDenials() != 1 {
		t.Fatalf("expected 1, got %d", tracker.ConsecutiveDenials())
	}
}

func TestShouldFallbackToPrompting(t *testing.T) {
	tracker := NewDenialTracker()
	sid := "sess-A"

	// 初始状态不应回退
	if tracker.ShouldFallbackToPrompting(sid) {
		t.Error("should not fallback initially")
	}

	// 连续拒绝 3 次应触发持久回退
	tracker.RecordDenial("Bash", "cmd1", "/path1", sid, SourceUser, ReasonUserDenied)
	tracker.RecordDenial("Bash", "cmd2", "/path2", sid, SourceUser, ReasonUserDenied)
	tracker.RecordDenial("Bash", "cmd3", "/path3", sid, SourceUser, ReasonUserDenied)

	if !tracker.ShouldFallbackToPrompting(sid) {
		t.Error("should fallback after 3 consecutive denials")
	}

	// Grant 不应清除 fallback（持久降级）
	tracker.RecordGrant(sid)
	if !tracker.ShouldFallbackToPrompting(sid) {
		t.Error("should still fallback after grant — fallback is persistent")
	}

	// ClearFallback 显式恢复
	tracker.ClearFallback(sid)
	if tracker.ShouldFallbackToPrompting(sid) {
		t.Error("should not fallback after ClearFallback")
	}
}

func TestShouldFallbackTotalRecords(t *testing.T) {
	tracker := NewDenialTracker()
	sid := "sess-A"

	// 累计 20 条记录应触发（即使 consecutive 被 grant 打断）
	for i := range 20 {
		tracker.RecordDenial("Bash", "cmd", "/path", sid, SourceUser, ReasonUserDenied)
		if i%3 == 2 {
			tracker.RecordGrant(sid) // 每 3 次 grant 一次，重置 consecutive
		}
	}

	if !tracker.ShouldFallbackToPrompting(sid) {
		t.Error("should fallback after 20 total denial records")
	}
}

func TestShouldFallbackSessionIsolation(t *testing.T) {
	tracker := NewDenialTracker()

	// sess-A 连续拒绝 3 次 → 触发 fallback
	tracker.RecordDenial("Bash", "cmd1", "/p1", "sess-A", SourceUser, ReasonUserDenied)
	tracker.RecordDenial("Bash", "cmd2", "/p2", "sess-A", SourceUser, ReasonUserDenied)
	tracker.RecordDenial("Bash", "cmd3", "/p3", "sess-A", SourceUser, ReasonUserDenied)

	if !tracker.ShouldFallbackToPrompting("sess-A") {
		t.Error("sess-A should fallback after 3 consecutive denials")
	}

	// sess-B 不应被 sess-A 的拒绝影响
	if tracker.ShouldFallbackToPrompting("sess-B") {
		t.Error("sess-B should not fallback — sess-A's denials should not affect it")
	}

	// sess-B 自己的拒绝不影响 sess-A 的 fallback 状态
	tracker.RecordDenial("Bash", "cmd1", "/p1", "sess-B", SourceUser, ReasonUserDenied)
	if !tracker.ShouldFallbackToPrompting("sess-A") {
		t.Error("sess-A should still be in fallback")
	}
	if tracker.ShouldFallbackToPrompting("sess-B") {
		t.Error("sess-B should not fallback with only 1 denial")
	}
}

// TestModeDenyNotCountedForFallback verifies that mode-level denials (e.g., plan mode)
// do not contribute to the fallback threshold.
func TestModeDenyNotCountedForFallback(t *testing.T) {
	tracker := NewDenialTracker()
	sid := "sess-plan"

	// 3 mode denials should NOT trigger fallback
	tracker.RecordDenial("Bash", "mkdir -p /tmp/x", "/tmp/x", sid, SourceMode, ReasonPlanMode)
	tracker.RecordDenial("Bash", "touch /tmp/y", "/tmp/y", sid, SourceMode, ReasonPlanMode)
	tracker.RecordDenial("Bash", "cp a b", "cp a b", sid, SourceMode, ReasonPlanMode)

	if tracker.ShouldFallbackToPrompting(sid) {
		t.Error("mode denials should not trigger fallback")
	}
	if tracker.ConsecutiveDenials() != 0 {
		t.Error("mode denials should not increment consecutive count")
	}

	// 1 real user denial should count
	tracker.RecordDenial("Bash", "rm stuff", "rm stuff", sid, SourceUser, ReasonUserDenied)
	if tracker.ConsecutiveDenials() != 1 {
		t.Errorf("expected 1, got %d", tracker.ConsecutiveDenials())
	}
}
