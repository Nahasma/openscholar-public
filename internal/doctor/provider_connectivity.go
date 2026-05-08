package doctor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/llm/connectivity"
	"github.com/Nahasma/openscholar-public/internal/llm/minimax"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

type MiniMaxProbe struct {
	EndpointFlavor string
	BaseURL        string
	AuthMode       connectivity.ProbeAuthMode
	HTTPStatus     int
	ErrorType      string
	ErrorMessage   string
	RequestID      string
	Skipped        string
}

type MiniMaxConnectivity struct {
	Provider          models.ModelProvider
	Model             string
	ConfiguredBaseURL string
	AuthSource        string
	KeyPresent        bool
	KeyLength         int
	KeyFingerprint    string
	Probes            []MiniMaxProbe
	Recommendation    string
}

type ProviderConnectivityRow struct {
	Provider string
	Model    string
	Status   string
	Message  string
	MiniMax  *MiniMaxConnectivity
}

type ProviderConnectivityReport struct {
	Rows []ProviderConnectivityRow
}

func CollectProviderConnectivity(ctx context.Context, cfg *config.Config) ProviderConnectivityReport {
	report := ProviderConnectivityReport{}
	if cfg == nil {
		return report
	}
	for providerName, providerCfg := range cfg.Providers {
		if providerCfg.Disabled {
			continue
		}
		if providerName == models.ProviderMiniMax {
			report.Rows = append(report.Rows, runMiniMaxConnectivity(ctx, cfg, providerCfg))
			continue
		}
		report.Rows = append(report.Rows, runGenericConnectivity(ctx, providerName, providerCfg))
	}
	return report
}

func FormatProviderConnectivityReport(report ProviderConnectivityReport) string {
	var sb strings.Builder
	sb.WriteString("Provider connectivity:\n")
	for _, row := range report.Rows {
		if row.MiniMax == nil {
			sb.WriteString(fmt.Sprintf("- %s/%s: %s", row.Provider, row.Model, row.Status))
			if row.Message != "" {
				sb.WriteString(" (" + row.Message + ")")
			}
			sb.WriteString("\n")
			continue
		}
		m := row.MiniMax
		sb.WriteString(fmt.Sprintf("- minimax/%s: MATRIX\n", m.Model))
		sb.WriteString(fmt.Sprintf("  configured_base_url: %s\n", m.ConfiguredBaseURL))
		sb.WriteString(fmt.Sprintf("  auth_source: %s\n", m.AuthSource))
		sb.WriteString(fmt.Sprintf("  key_present: %t\n", m.KeyPresent))
		sb.WriteString(fmt.Sprintf("  key_length: %d\n", m.KeyLength))
		sb.WriteString(fmt.Sprintf("  key_fingerprint: %s\n", m.KeyFingerprint))
		for _, probe := range m.Probes {
			sb.WriteString(fmt.Sprintf("  - flavor=%s auth_mode=%s base_url=%s", probe.EndpointFlavor, probe.AuthMode, probe.BaseURL))
			if probe.Skipped != "" {
				sb.WriteString(" skipped=" + probe.Skipped + "\n")
				continue
			}
			sb.WriteString(fmt.Sprintf(" http_status=%d", probe.HTTPStatus))
			if probe.ErrorType != "" {
				sb.WriteString(" error_type=" + probe.ErrorType)
			}
			if probe.ErrorMessage != "" {
				sb.WriteString(" error_message=" + probe.ErrorMessage)
			}
			if probe.RequestID != "" {
				sb.WriteString(" request_id=" + probe.RequestID)
			}
			sb.WriteString("\n")
		}
		if m.Recommendation != "" {
			sb.WriteString("  recommendation: " + m.Recommendation + "\n")
		}
	}
	return sb.String()
}

func runGenericConnectivity(ctx context.Context, providerName models.ModelProvider, providerCfg config.Provider) ProviderConnectivityRow {
	modelID := providerCfg.Model
	if modelID == "" {
		if spec, ok := models.ProviderSpecByID(providerName); ok && spec.DefaultModel != "" {
			modelID = spec.DefaultModel
		} else {
			modelID = "ping"
		}
	}
	apiKey, _, err := config.ResolveAPIKey(providerName, providerCfg)
	if err != nil {
		return ProviderConnectivityRow{Provider: string(providerName), Model: modelID, Status: "SKIP", Message: err.Error()}
	}
	res := connectivity.PingChat(ctx, connectivity.TestRequest{
		Provider: providerName,
		ModelID:  modelID,
		APIKey:   apiKey,
		BaseURL:  providerCfg.BaseURL,
		Timeout:  10 * time.Second,
	})
	status := "FAIL"
	if res.ProviderOK && res.ModelOK {
		status = "OK"
	}
	if res.Warning {
		status = "WARN"
	}
	msg := res.Message
	if msg == "" {
		msg = string(res.Category)
	}
	return ProviderConnectivityRow{Provider: string(providerName), Model: modelID, Status: status, Message: msg}
}

func runMiniMaxConnectivity(ctx context.Context, cfg *config.Config, providerCfg config.Provider) ProviderConnectivityRow {
	modelID := providerCfg.Model
	if modelID == "" {
		if spec, ok := models.ProviderSpecByID(models.ProviderMiniMax); ok && spec.DefaultModel != "" {
			modelID = spec.DefaultModel
		} else {
			modelID = "ping"
		}
	}
	key, source, err := config.ResolveAPIKey(models.ProviderMiniMax, providerCfg)
	authSource := config.ProviderAuthSource(cfg, models.ProviderMiniMax)
	if authSource == "missing" && source != "" {
		authSource = string(source)
	}
	if authSource == "" {
		authSource = "missing"
	}
	m := &MiniMaxConnectivity{
		Provider:          models.ProviderMiniMax,
		Model:             modelID,
		ConfiguredBaseURL: effectiveConfiguredBaseURL(providerCfg.BaseURL, models.ProviderMiniMax, providerCfg.Profile, providerCfg.AuthMode),
		AuthSource:        authSource,
		KeyPresent:        strings.TrimSpace(key) != "",
		KeyLength:         len(strings.TrimSpace(key)),
		KeyFingerprint:    connectivity.KeyFingerprint(key),
	}
	m.Probes = append(m.Probes,
		MiniMaxProbe{EndpointFlavor: "token-plan", BaseURL: minimax.TokenPlanBaseURL, AuthMode: connectivity.ProbeAuthModeBearer},
		MiniMaxProbe{EndpointFlavor: "token-plan", BaseURL: minimax.TokenPlanBaseURL, AuthMode: connectivity.ProbeAuthModeAnthropicXAPIKey},
		MiniMaxProbe{EndpointFlavor: "legacy", BaseURL: minimax.LegacyBaseURL, AuthMode: connectivity.ProbeAuthModeAnthropicXAPIKey},
		MiniMaxProbe{EndpointFlavor: "legacy", BaseURL: minimax.LegacyBaseURL, AuthMode: connectivity.ProbeAuthModeBearer},
	)
	if base := strings.TrimSpace(providerCfg.BaseURL); base != "" && !isMiniMaxFixedBaseURL(base) {
		m.Probes = append(m.Probes,
			MiniMaxProbe{EndpointFlavor: "configured", BaseURL: base, AuthMode: connectivity.ProbeAuthModeAnthropicXAPIKey},
			MiniMaxProbe{EndpointFlavor: "configured", BaseURL: base, AuthMode: connectivity.ProbeAuthModeBearer},
		)
	}

	for i := range m.Probes {
		if err != nil || !m.KeyPresent {
			m.Probes[i].Skipped = "missing_api_key"
			continue
		}
		res := connectivity.ProbeChat(ctx, connectivity.ProbeRequest{
			TestRequest: connectivity.TestRequest{
				Provider: models.ProviderMiniMax,
				ModelID:  modelID,
				APIKey:   key,
				BaseURL:  m.Probes[i].BaseURL,
				Timeout:  10 * time.Second,
			},
			AuthMode: m.Probes[i].AuthMode,
		})
		m.Probes[i].HTTPStatus = res.HTTPStatus
		m.Probes[i].ErrorType = res.ErrorType
		m.Probes[i].ErrorMessage = res.Message
		m.Probes[i].RequestID = res.RequestID
	}
	m.Recommendation = minimaxRecommendation(m)
	return ProviderConnectivityRow{Provider: "minimax", Model: modelID, Status: "MATRIX", MiniMax: m}
}

func minimaxRecommendation(m *MiniMaxConnectivity) string {
	if m.AuthSource == string(config.AuthEnv) {
		return "env key overrides config key"
	}
	if !m.KeyPresent {
		return "missing API key: set MINIMAX_API_KEY or providers.minimax.apiKey"
	}
	all401 := true
	var tokenPlanXAPI401 bool
	var legacy401 bool
	for _, p := range m.Probes {
		if p.Skipped != "" {
			continue
		}
		if p.HTTPStatus != 401 {
			all401 = false
		}
		if p.BaseURL == minimax.TokenPlanBaseURL && p.AuthMode == connectivity.ProbeAuthModeAnthropicXAPIKey && p.HTTPStatus == 401 {
			tokenPlanXAPI401 = true
		}
		if p.BaseURL == minimax.LegacyBaseURL && p.HTTPStatus == 401 {
			legacy401 = true
		}
	}
	switch {
	case all401:
		return "all matrix probes returned 401: key may be revoked/expired or account/project permission mismatched"
	case tokenPlanXAPI401:
		return "official MiniMax endpoint requires bearer auth: use authMode=bearer (recommended), then compare matrix results"
	case legacy401:
		return "deprecated api.minimax.io endpoint returned 401: use https://api.minimaxi.com/anthropic with bearer auth unless a legacy profile is intentional"
	default:
		return ""
	}
}

func effectiveConfiguredBaseURL(baseURL string, provider models.ModelProvider, profile, authMode string) string {
	if provider == models.ProviderMiniMax {
		if rt, err := minimax.ResolveRuntime(baseURL, profile, authMode); err == nil && strings.TrimSpace(rt.BaseURL) != "" {
			return rt.BaseURL
		}
	}
	if strings.TrimSpace(baseURL) != "" {
		return strings.TrimSpace(baseURL)
	}
	return models.ProviderDefaultBaseURL(provider)
}

func isMiniMaxFixedBaseURL(baseURL string) bool {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return trimmed == minimax.LegacyBaseURL || trimmed == minimax.TokenPlanBaseURL
}
