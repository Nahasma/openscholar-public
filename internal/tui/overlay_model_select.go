package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
	"github.com/Nahasma/openscholar-public/internal/tui/components"
)

// ModelSelectChoice represents the selected model.
type ModelSelectChoice struct {
	Provider models.ModelProvider
	ModelID  models.ModelID
}

// ModelSelectOverlay implements Overlay for the model selection dialog.
type ModelSelectOverlay struct {
	providers       []models.ModelProvider
	providerIdx     int
	list            []models.Model
	listIdx         int
	listScroll      int
	currentProvider models.ModelProvider
	currentModelID  models.ModelID
	selectedModelID models.ModelID
	discovered      map[models.ModelProvider][]models.Model
	loadingProvider models.ModelProvider
	listWarning     string
}

// NewModelSelectOverlay creates a new model selection overlay, pre-populated
// with provider/model lists based on the current model.
func NewModelSelectOverlay(currentProvider models.ModelProvider, currentModelID models.ModelID) *ModelSelectOverlay {
	o := &ModelSelectOverlay{
		currentProvider: currentProvider,
		currentModelID:  currentModelID,
		selectedModelID: currentModelID,
		discovered:      make(map[models.ModelProvider][]models.Model),
	}

	cfg := config.Get()
	for _, prov := range models.ProviderDisplayOrder() {
		if len(config.ModelOptionsForProvider(cfg, prov, nil)) > 0 {
			o.providers = append(o.providers, prov)
		}
	}

	// Find current provider
	for i, p := range o.providers {
		if p == currentProvider {
			o.providerIdx = i
			break
		}
	}

	o.setupModelList()
	return o
}

func (o *ModelSelectOverlay) ID() string        { return "model-select" }
func (o *ModelSelectOverlay) Kind() OverlayKind { return OverlayBlocking }
func (o *ModelSelectOverlay) BlocksInput() bool { return true }

func (o *ModelSelectOverlay) Update(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
	if loaded, ok := msg.(modelListLoadedMsg); ok {
		o.applyModelListLoaded(loaded)
		return o, nil, nil
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil, nil
	}

	switch keyMsg.Type {
	case tea.KeyEsc:
		return o, &OverlayResult{Action: "dismiss"}, nil

	case tea.KeyUp:
		if len(o.list) == 0 {
			return o, nil, nil
		}
		if o.listIdx > 0 {
			o.listIdx--
		} else {
			o.listIdx = len(o.list) - 1
		}
		o.ensureVisible()
		return o, nil, nil

	case tea.KeyDown:
		if len(o.list) == 0 {
			return o, nil, nil
		}
		if o.listIdx < len(o.list)-1 {
			o.listIdx++
		} else {
			o.listIdx = 0
		}
		o.ensureVisible()
		return o, nil, nil

	case tea.KeyLeft:
		if len(o.providers) > 1 {
			if o.providerIdx > 0 {
				o.providerIdx--
			} else {
				o.providerIdx = len(o.providers) - 1
			}
			o.setupModelList()
			return o, nil, o.discoverCurrentProviderCmd()
		}
		return o, nil, nil

	case tea.KeyRight:
		if len(o.providers) > 1 {
			if o.providerIdx < len(o.providers)-1 {
				o.providerIdx++
			} else {
				o.providerIdx = 0
			}
			o.setupModelList()
			return o, nil, o.discoverCurrentProviderCmd()
		}
		return o, nil, nil

	case tea.KeyEnter:
		if len(o.list) > 0 && o.listIdx < len(o.list) {
			selected := o.list[o.listIdx]
			o.selectedModelID = selected.ID
			return o, &OverlayResult{
				Action: "accept",
				Data:   ModelSelectChoice{Provider: selected.Provider, ModelID: selected.ID},
			}, nil
		}
		return o, nil, nil
	}

	// vim-style keys
	switch keyMsg.String() {
	case "j":
		keyMsg.Type = tea.KeyDown
		return o.Update(keyMsg)
	case "k":
		keyMsg.Type = tea.KeyUp
		return o.Update(keyMsg)
	case "h":
		keyMsg.Type = tea.KeyLeft
		return o.Update(keyMsg)
	case "l":
		keyMsg.Type = tea.KeyRight
		return o.Update(keyMsg)
	}

	return o, nil, nil
}

func (o *ModelSelectOverlay) View(width, height int) string {
	warning := o.listWarning
	if o.loadingProvider != "" && o.currentProviderIndexValid() && o.providers[o.providerIdx] == o.loadingProvider {
		warning = "正在刷新 provider 模型列表..."
	}
	return components.RenderModelDialogWithNotice(
		o.providers, o.providerIdx,
		o.list, o.listIdx, o.listScroll,
		o.currentModelID, innerDialogWidth(width), warning,
	)
}

// setupModelList populates the model list for the currently selected provider.
func (o *ModelSelectOverlay) setupModelList() {
	if o.providerIdx >= len(o.providers) {
		return
	}
	prov := o.providers[o.providerIdx]
	options := config.ModelOptionsForProvider(config.Get(), prov, o.discovered[prov])
	modelList := make([]models.Model, 0, len(options))
	for _, opt := range options {
		modelList = append(modelList, opt.Model)
	}

	o.list = modelList
	if len(modelList) == 0 {
		o.listIdx = 0
		o.listScroll = 0
		return
	}

	selectedIdx := -1
	for i, mdl := range modelList {
		if mdl.ID == o.selectedModelID {
			selectedIdx = i
			break
		}
	}
	if selectedIdx == -1 {
		selectedIdx = 0
		o.selectedModelID = modelList[0].ID
	}
	o.listIdx = selectedIdx
	o.ensureVisible()
}

func (o *ModelSelectOverlay) currentProviderIndexValid() bool {
	return o.providerIdx >= 0 && o.providerIdx < len(o.providers)
}

func (o *ModelSelectOverlay) discoverCurrentProviderCmd() tea.Cmd {
	if !o.currentProviderIndexValid() {
		return nil
	}
	prov := o.providers[o.providerIdx]
	cmd := discoverModelsForProviderCmd(prov)
	if cmd == nil {
		o.loadingProvider = ""
		return nil
	}
	o.loadingProvider = prov
	o.listWarning = ""
	return cmd
}

func (o *ModelSelectOverlay) applyModelListLoaded(msg modelListLoadedMsg) {
	if o.loadingProvider == msg.Provider {
		o.loadingProvider = ""
	}
	if msg.Err != nil {
		o.listWarning = "模型列表刷新失败: " + msg.Err.Error()
		return
	}
	o.discovered[msg.Provider] = append([]models.Model(nil), msg.Models...)
	config.MergeProviderModelsInMemory(msg.Provider, modelConfigsFromDiscovered(msg.Models))
	if len(msg.Models) > 0 {
		o.listWarning = "已加载 provider 返回的模型列表"
	} else {
		o.listWarning = ""
	}
	if o.currentProviderIndexValid() && o.providers[o.providerIdx] == msg.Provider {
		o.setupModelList()
	}
}

func (o *ModelSelectOverlay) ensureVisible() {
	const maxVisible = 10
	n := len(o.list)
	if n == 0 {
		o.listIdx = 0
		o.listScroll = 0
		return
	}
	o.listIdx = max(0, min(o.listIdx, n-1))
	visible := min(maxVisible, n)
	minScroll := o.listIdx - visible + 1
	if minScroll < 0 {
		minScroll = 0
	}
	maxScroll := o.listIdx
	if o.listScroll < minScroll {
		o.listScroll = minScroll
	}
	if o.listScroll > maxScroll {
		o.listScroll = maxScroll
	}
	maxAllowedScroll := max(0, n-visible)
	o.listScroll = max(0, min(o.listScroll, maxAllowedScroll))
	o.selectedModelID = o.list[o.listIdx].ID
}
