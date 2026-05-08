package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/openscholar/openscholar/internal/llm/models"
)

const maxVisibleModels = 10

var (
	modelDialogTitleStyle    lipgloss.Style
	modelDialogItemStyle     lipgloss.Style
	modelDialogSelectedStyle lipgloss.Style
	modelDialogCurrentStyle  lipgloss.Style
	modelDialogProviderStyle lipgloss.Style
)

func initModelDialogStyles() {
	modelDialogTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorBrandPurple)
	modelDialogItemStyle = lipgloss.NewStyle().
		Foreground(ColorGrayMedium).
		PaddingLeft(1)
	modelDialogSelectedStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorWhite).
		Background(ColorBlue).
		PaddingLeft(1).
		PaddingRight(1)
	modelDialogCurrentStyle = lipgloss.NewStyle().
		Foreground(ColorGreen)
	modelDialogProviderStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorWhite)
}

// RenderModelDialog renders the interactive model selection dialog.
func RenderModelDialog(
	providers []models.ModelProvider,
	providerIdx int,
	modelList []models.Model,
	modelIdx int,
	modelScroll int,
	currentModelID models.ModelID,
	width int,
) string {
	return RenderModelDialogWithNotice(providers, providerIdx, modelList, modelIdx, modelScroll, currentModelID, width, "")
}

func RenderModelDialogWithNotice(
	providers []models.ModelProvider,
	providerIdx int,
	modelList []models.Model,
	modelIdx int,
	modelScroll int,
	currentModelID models.ModelID,
	width int,
	notice string,
) string {
	if width < 2 {
		width = 2
	}
	var sb strings.Builder

	sb.WriteString("\n")

	// Provider tabs
	var provTabs []string
	for i, prov := range providers {
		name := models.ProviderDisplayName(prov)
		if i == providerIdx {
			provTabs = append(provTabs, modelDialogProviderStyle.Render("[ "+name+" ]"))
		} else {
			provTabs = append(provTabs, dialogMutedStyle.Render("  "+name+"  "))
		}
	}
	sb.WriteString(" " + modelDialogTitleStyle.Render("Select Model") + "\n")
	sb.WriteString(" " + strings.Join(provTabs, " ") + "\n")
	sb.WriteString(" " + previewDividerStyle.Render(strings.Repeat("─", width-2)) + "\n")

	// Model list
	if len(modelList) == 0 {
		sb.WriteString("   " + dialogMutedStyle.Render("No models available") + "\n")
	} else {
		endIdx := modelScroll + maxVisibleModels
		if endIdx > len(modelList) {
			endIdx = len(modelList)
		}

		// Scroll up indicator
		if modelScroll > 0 {
			sb.WriteString("   " + dialogMutedStyle.Render("↑ more") + "\n")
		}

		for i := modelScroll; i < endIdx; i++ {
			m := modelList[i]
			isCurrent := m.ID == currentModelID
			currentMarker := "  "
			if isCurrent {
				currentMarker = modelDialogCurrentStyle.Render("→ ")
			}

			nameStr := fmt.Sprintf("%-25s %s", m.Name, m.ID)
			if i == modelIdx {
				prefix := " " + btnArrowStyle.Render("❯") + " "
				sb.WriteString(prefix + currentMarker + modelDialogSelectedStyle.Render(nameStr) + "\n")
			} else {
				sb.WriteString("   " + currentMarker + modelDialogItemStyle.Render(nameStr) + "\n")
			}
		}

		// Scroll down indicator
		if endIdx < len(modelList) {
			sb.WriteString("   " + dialogMutedStyle.Render("↓ more") + "\n")
		}
	}

	sb.WriteString("\n")
	if strings.TrimSpace(notice) != "" {
		sb.WriteString(" " + dialogMutedStyle.Render(notice) + "\n")
	}
	sb.WriteString(" " + dialogMutedStyle.Render("↑↓ select · ←→ provider · enter confirm · esc cancel") + "\n")

	return sb.String()
}
