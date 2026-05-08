package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

func MaskSecret(raw string) string {
	if raw == "" {
		return ""
	}
	if len(raw) <= 8 {
		return "****"
	}
	return "****" + raw[len(raw)-4:]
}

func ProviderAuthSource(c *Config, provider models.ModelProvider) string {
	mode := models.ProviderAuthMode(provider)
	if mode == models.AuthNone {
		return "none"
	}
	configKey := ""
	if c != nil {
		if p, ok := c.Providers[provider]; ok {
			configKey = p.APIKey
		}
	}
	for _, env := range models.ProviderAPIKeyEnvVars(provider) {
		envKey := os.Getenv(env)
		if envKey == "" {
			continue
		}
		if configKey != "" && envKey == configKey && providerConfigFileHasAPIKey(c, provider, configKey) {
			continue
		}
		if envKey != "" {
			return "env"
		}
	}
	if configKey != "" {
		return "config"
	}
	return "missing"
}

func providerConfigFileHasAPIKey(c *Config, provider models.ModelProvider, apiKey string) bool {
	if c == nil || c.WorkingDir == "" || apiKey == "" {
		return false
	}
	data, err := os.ReadFile(ConfigFilePath(c.WorkingDir))
	if err != nil {
		return false
	}
	var fileCfg struct {
		Providers map[models.ModelProvider]Provider `json:"providers"`
	}
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return false
	}
	p, ok := fileCfg.Providers[provider]
	return ok && p.APIKey == apiKey
}

func DebugReport(c *Config) string {
	if c == nil {
		return "配置未加载"
	}
	var sb strings.Builder
	configPath := ConfigFilePath(c.WorkingDir)
	_, statErr := os.Stat(configPath)
	source := "missing"
	if statErr == nil {
		source = "file"
	}

	sb.WriteString("Config Debug:\n\n")
	sb.WriteString(fmt.Sprintf("Source:\t%s\n", source))
	sb.WriteString(fmt.Sprintf("Config Path:\t%s\n", configPath))
	sb.WriteString(fmt.Sprintf("Working Dir:\t%s\n", c.WorkingDir))
	sb.WriteString(fmt.Sprintf("Default Provider:\t%s\n\n", models.ProviderDisplayName(c.DefaultProvider)))

	sb.WriteString("Providers:\n")
	ptw := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(ptw, "Provider\tAuth\tDisabled")
	for _, prov := range models.ProviderDisplayOrder() {
		pcfg := c.Providers[prov]
		auth := ProviderAuthSource(c, prov)
		disabled := "no"
		if pcfg.Disabled {
			disabled = "yes"
		}
		_, _ = fmt.Fprintf(ptw, "%s\t%s\t%s\n", models.ProviderDisplayName(prov), auth, disabled)
	}
	_ = ptw.Flush()

	sb.WriteString("\nAgent Mappings:\n")
	atw := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(atw, "Agent\tProvider\tModel")
	for _, name := range []AgentName{
		AgentCoder, AgentGeneral, AgentExplore, AgentSummarizer, AgentTitle, AgentLeader,
		AgentTask, AgentPlan, AgentVerify, AgentCoordinator,
	} {
		a, ok := c.Agents[name]
		if !ok {
			continue
		}
		prov := a.Provider
		if prov == "" {
			prov = c.DefaultProvider
		}
		_, _ = fmt.Fprintf(atw, "%s\t%s\t%s\n", name, prov, a.Model)
	}
	_ = atw.Flush()

	validationErrs := Validate(c)
	if len(validationErrs) > 0 {
		sb.WriteString("\nWarnings:\n")
		for _, w := range validationErrs {
			sb.WriteString(fmt.Sprintf("  - %s\n", w.Error()))
		}
	}
	return sb.String()
}
