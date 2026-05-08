package tools

import "sync"

// DoomLoopDetector 检测工具调用重复模式。
// 可配置窗口大小和重复阈值。
type DoomLoopDetector struct {
	mu        sync.Mutex
	calls     []string
	window    int // 检查窗口大小（默认 5）
	threshold int // 重复次数阈值（默认 3）
}

// NewDoomLoopDetector 创建默认配置（window=5, threshold=3）的检测器。
func NewDoomLoopDetector() *DoomLoopDetector {
	return &DoomLoopDetector{
		window:    5,
		threshold: 3,
	}
}

// NewDoomLoopDetectorWithConfig 创建自定义配置的检测器。
func NewDoomLoopDetectorWithConfig(window, threshold int) *DoomLoopDetector {
	return &DoomLoopDetector{
		window:    window,
		threshold: threshold,
	}
}

// RecordCall 记录一次调用并返回是否检测到重复模式（保持向后兼容）。
func (d *DoomLoopDetector) RecordCall(signature string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.calls = append(d.calls, signature)

	if len(d.calls) < d.window {
		return false
	}

	last := d.calls[len(d.calls)-1]
	repeats := 0
	for i := len(d.calls) - 2; i >= 0 && i >= len(d.calls)-d.window; i-- {
		if d.calls[i] == last {
			repeats++
		}
	}

	return repeats >= d.threshold
}

// Observe 检查给定的签名列表是否存在重复模式，但不修改内部状态。
// 用于 RepeatedToolPatternHook 从 LoopState.LastRoundToolSignatures 检查。
func (d *DoomLoopDetector) Observe(signatures []string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 合并内部 calls 和传入 signatures 到临时 slice
	combined := make([]string, len(d.calls)+len(signatures))
	copy(combined, d.calls)
	copy(combined[len(d.calls):], signatures)

	if len(combined) < d.window {
		return false
	}

	// 在 combined 的最后 window 个元素中统计每个签名出现次数
	start := len(combined) - d.window
	counts := make(map[string]int, d.window)
	for _, sig := range combined[start:] {
		counts[sig]++
		if counts[sig] >= d.threshold {
			return true
		}
	}

	return false
}

// Reset 清空内部状态。
func (d *DoomLoopDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = nil
}
