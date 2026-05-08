package tui

import (
	"fmt"

	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/tui/components"
)

func (m *Model) saveFullAndApplyRuntimeConfig(newCfg *config.Config) (output string, summary string, applied bool) {
	cwd := config.WorkingDirectory()
	if err := config.SaveFull(cwd, *newCfg); err != nil {
		return fmt.Sprintf("保存配置失败: %v", err), "Config wizard failed", false
	}

	config.Reset()
	if _, err := config.Load(cwd); err != nil {
		return fmt.Sprintf("配置已保存，但重新加载失败: %v", err), "Config wizard failed", false
	}
	if m.app == nil {
		return "配置已保存，但运行时重载失败: app 未初始化", "Config wizard failed", false
	}
	if err := m.app.ReloadProvider(); err != nil {
		return fmt.Sprintf("配置已保存，但运行时重载失败: %v", err), "Config wizard failed", false
	}

	m.applyRuntimeConfigChange()
	return "配置已保存并生效", "Config wizard saved", true
}

func (m *Model) applyRuntimeConfigChange() {
	m.captureRuntimeConfigScrollAnchor()
	m.chat.renderedContent = ""
	m.chat.contentLines = nil
	m.resetMainScreenIntroCache()
	m.chat.renderCache = components.NewRenderCache()
	if m.chat.heightCache != nil {
		m.chat.heightCache.InvalidateAll()
	}
	if m.chat.blockList != nil {
		blocks := m.chat.blockList.All()
		for i := range blocks {
			blocks[i].MarkDirty()
		}
	}
}

func (m *Model) captureRuntimeConfigScrollAnchor() {
	if m.chat.scrollMode != ScrollManualLocked || m.chat.pendingAnchor != nil {
		return
	}
	if m.chat.blockList == nil || m.chat.heightCache == nil || m.chat.blockList.Len() == 0 {
		return
	}
	defaultHeight := 1
	if m.usesVirtualTranscript() {
		defaultHeight = components.DefaultBlockHeight
	}
	anchor := components.AnchorFromDisplayLine(
		m.chat.heightCache,
		m.chat.blockList.All(),
		m.chat.viewport.YOffset,
		defaultHeight,
	)
	m.chat.pendingAnchor = &anchor
}
