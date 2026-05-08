package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/fileop"
	"github.com/Nahasma/openscholar-public/internal/permission"
)

// --- Image provider interface ---

// imageProvider abstracts different image generation APIs.
type imageProvider interface {
	name() string
	generate(ctx context.Context, apiKey, model, prompt, size string) (imageURL string, err error)
	defaultModel() string
	supportedSizes() []string
}

// providerEntry maps an env var key to an imageProvider implementation.
type providerEntry struct {
	envKey   string
	provider imageProvider
}

// imageProviders defines the auto-detection order.
// The first provider with a configured API key will be used.
var imageProviders = []providerEntry{
	{"GLM_API_KEY", &glmCogViewProvider{}},
	{"MINIMAX_API_KEY", &minimaxImageProvider{}},
	{"OPENAI_API_KEY", &openaiDalleProvider{}},
}

// --- Tool implementation ---

type imageGenTool struct {
	permissions permission.Service
}

type imageGenParams struct {
	Prompt      string `json:"prompt"`
	Filename    string `json:"filename"`
	Directory   string `json:"directory,omitempty"`
	Size        string `json:"size,omitempty"`
	Model       string `json:"model,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Quality     string `json:"quality,omitempty"`
	StylePreset string `json:"style_preset,omitempty"`
	TextPolicy  string `json:"text_policy,omitempty"`
}

func NewImageGenTool(perms permission.Service) BaseTool {
	return &imageGenTool{permissions: perms}
}

func (t *imageGenTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "ImageGen",
		Description: "Generate images using AI image generation APIs (GLM CogView, MiniMax image-01, OpenAI DALL-E). Use for conceptual illustrations, application scenario diagrams, and other images that cannot be drawn with TikZ. The provider is auto-selected based on available API keys, or can be specified explicitly.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "Detailed description of the image to generate. Be specific about composition, style, and content. English prompts generally produce better results.",
				},
				"filename": map[string]any{
					"type":        "string",
					"description": "Output filename without extension (e.g. 'robot_manipulation'). Saved as <filename>.png in the output directory.",
				},
				"directory": map[string]any{
					"type":        "string",
					"description": "Optional output directory path. If omitted, saves to the current working directory.",
				},
				"size": map[string]any{
					"type":        "string",
					"description": "Image size, default '1024x1024'. Supported sizes vary by provider.",
				},
				"model": map[string]any{
					"type":        "string",
					"description": "Model to use. Defaults depend on provider: GLM → cogview-4-plus, MiniMax → image-01, OpenAI → dall-e-3.",
				},
				"provider": map[string]any{
					"type":        "string",
					"description": "Image provider to use: 'glm', 'minimax', or 'openai'. If omitted, auto-selects based on available API keys.",
				},
				"quality": map[string]any{
					"type":        "string",
					"description": "Quality hint: standard|high. Default: high.",
				},
				"style_preset": map[string]any{
					"type":        "string",
					"description": "Style preset: paper|minimal|none. Default: paper.",
				},
				"text_policy": map[string]any{
					"type":        "string",
					"description": "Text handling: auto|forbid|short_text|overlay_base. Default: auto. Use short_text only for a few explicitly requested words; use overlay_base for images that will receive labels later in TikZ/LaTeX. Precise labels, formulas, axes, legends, or data should be routed to DiagramGen/matplotlib/TikZ instead of ImageGen.",
				},
			},
			"required": []string{"prompt", "filename"},
		},
		Required: []string{"prompt", "filename"},
	}
}

func (t *imageGenTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	if IsResearchMode(ctx) {
		return NewTextErrorResponse("Research mode is active. Image generation is not allowed in research mode. Use Shift+Tab to switch to default mode."), nil
	}

	var params imageGenParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		// Attempt fuzzy extraction from malformed JSON (BUG-7: weak models produce garbled tool calls)
		params.Prompt = extractJSONStringField(call.Input, "prompt")
		params.Filename = extractJSONStringField(call.Input, "filename")
		if params.Prompt == "" {
			return NewTextErrorResponse(fmt.Sprintf("Invalid parameters (JSON parse failed): %v", err)), nil
		}
	}

	if params.Prompt == "" {
		return NewTextErrorResponse("prompt is required"), nil
	}
	if params.Filename == "" {
		return NewTextErrorResponse("filename is required"), nil
	}
	params.Filename = strings.TrimSpace(params.Filename)
	if err := validateArtifactFilename(params.Filename); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid filename: %v", err)), nil
	}
	quality := strings.ToLower(strings.TrimSpace(params.Quality))
	if quality == "" {
		quality = "high"
	}
	if quality != "high" && quality != "standard" {
		return NewTextErrorResponse("quality must be one of: high, standard"), nil
	}
	stylePreset := strings.ToLower(strings.TrimSpace(params.StylePreset))
	if stylePreset == "" {
		stylePreset = "paper"
	}
	if stylePreset != "paper" && stylePreset != "minimal" && stylePreset != "none" {
		return NewTextErrorResponse("style_preset must be one of: paper, minimal, none"), nil
	}
	textPolicy := strings.ToLower(strings.TrimSpace(params.TextPolicy))
	if textPolicy == "" {
		textPolicy = imageTextPolicyAuto
	}
	if !validImageTextPolicy(textPolicy) {
		return NewTextErrorResponse("text_policy must be one of: auto, forbid, short_text, overlay_base"), nil
	}
	textRouteWarning := imageTextRouteWarning(params.Prompt, textPolicy)

	// Determine and validate output path before provider calls, so invalid
	// filenames cannot spend API quota or escape the workspace.
	cwd, _ := os.Getwd()
	outDir := cwd
	if params.Directory != "" {
		if filepath.IsAbs(params.Directory) {
			outDir = params.Directory
		} else {
			outDir = filepath.Join(cwd, params.Directory)
		}
	}
	if err := ValidateWorkspacePath(ctx, outDir); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Workspace boundary: %v", err)), nil
	}
	outputPath := filepath.Join(outDir, params.Filename+".png")
	if err := ValidateWorkspacePath(ctx, outputPath); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Workspace boundary: %v", err)), nil
	}

	// Permission check
	if t.permissions != nil {
		sessionID, _ := GetContextValues(ctx)
		allowed := t.permissions.Request(permission.CreatePermissionRequest{
			SessionID:   sessionID,
			ToolName:    "ImageGen",
			Description: fmt.Sprintf("Generate image: %s → %s.png", truncatePrompt(params.Prompt, 80), params.Filename),
			Action:      "write",
			Params:      params,
		})
		if !allowed {
			return ToolResponse{}, permission.ErrorPermissionDenied
		}
	}

	// Select provider and API key.
	// Priority: explicit param > config imageProvider > auto-detect
	providerName := params.Provider
	if providerName == "" {
		providerName = config.DefaultImageProvider()
	}
	prov, apiKey, err := resolveImageProvider(providerName)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	// Apply defaults
	model := params.Model
	if model == "" {
		model = prov.defaultModel()
	}
	size := resolveImageSize(prov, params.Size, stylePreset)
	if err := validateImageSize(prov, size); err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	// Enhance prompt for publication-friendly output while preserving explicit intent.
	enhancedPrompt := enhanceImagePromptWithOptions(params.Prompt, imagePromptOptions{
		StylePreset: stylePreset,
		Quality:     quality,
		TextPolicy:  textPolicy,
	})

	// Generate image
	imageURL, err := prov.generate(ctx, apiKey, model, enhancedPrompt, size)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Image generation failed (%s): %v", prov.name(), err)), nil
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to create output directory: %v", err)), nil
	}

	// Download image
	if err := downloadImage(ctx, imageURL, outputPath); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to download image: %v", err)), nil
	}

	// Return success with LaTeX usage hint
	relDir, _ := filepath.Rel(cwd, outDir)
	var relPath string
	if relDir != "" && relDir != "." {
		relPath = filepath.Join(relDir, params.Filename+".png")
	} else {
		relPath = params.Filename + ".png"
	}
	result := fmt.Sprintf("Image saved to: %s (provider: %s, model: %s)\n\nLaTeX usage:\n\\begin{figure}[t]\n\\centering\n\\includegraphics[width=0.8\\textwidth]{%s}\n\\caption{TODO: Add caption}\n\\label{fig:%s}\n\\end{figure}\n\nQuality reminder: This is an AI-generated image; a human must review factual accuracy before publication.\nCompletion nudge: Reply with the saved path above and continue writing. Do not run extra file checks.",
		outputPath, prov.name(), model, relPath, params.Filename)
	if textRouteWarning != "" {
		result += "\n\nText policy warning: " + textRouteWarning
	}

	return WithResponseMetadata(
		NewTextResponse(result),
		map[string]any{
			"path":             outputPath,
			"provider":         prov.name(),
			"model":            model,
			"size":             size,
			"durable_progress": true,
			"artifact_kind":    "image",
			"artifact_paths":   []string{outputPath},
			"style_preset":     stylePreset,
			"quality":          quality,
			"text_policy":      textPolicy,
			"text_warning":     textRouteWarning,
		},
	), nil
}

// resolveImageProvider selects the provider and API key.
// If providerName is specified, use that; otherwise auto-detect from available API keys.
func resolveImageProvider(providerName string) (imageProvider, string, error) {
	if providerName != "" {
		for _, entry := range imageProviders {
			if entry.provider.name() == providerName {
				apiKey := os.Getenv(entry.envKey)
				if apiKey == "" {
					return nil, "", fmt.Errorf("provider '%s' selected but %s is not configured", providerName, entry.envKey)
				}
				return entry.provider, apiKey, nil
			}
		}
		return nil, "", fmt.Errorf("unknown image provider '%s'. Available: glm, minimax, openai", providerName)
	}

	// Auto-detect: use the first provider with a configured API key
	for _, entry := range imageProviders {
		if apiKey := os.Getenv(entry.envKey); apiKey != "" {
			return entry.provider, apiKey, nil
		}
	}
	return nil, "", fmt.Errorf("no image generation provider configured. Set GLM_API_KEY, MINIMAX_API_KEY, or OPENAI_API_KEY in .openscholar/config.json or as an environment variable")
}

// --- GLM CogView provider ---

type glmCogViewProvider struct{}

func (p *glmCogViewProvider) name() string         { return "glm" }
func (p *glmCogViewProvider) defaultModel() string { return "cogview-4-plus" }
func (p *glmCogViewProvider) supportedSizes() []string {
	return []string{"1024x1024", "768x1344", "1344x768", "864x1152", "1152x864"}
}

func (p *glmCogViewProvider) generate(ctx context.Context, apiKey, model, prompt, size string) (string, error) {
	return callImageAPI(ctx, callImageAPIParams{
		endpoint: "https://open.bigmodel.cn/api/paas/v4/images/generations",
		apiKey:   apiKey,
		model:    model,
		prompt:   prompt,
		size:     size,
	})
}

// --- OpenAI DALL-E provider ---

type openaiDalleProvider struct{}

func (p *openaiDalleProvider) name() string         { return "openai" }
func (p *openaiDalleProvider) defaultModel() string { return "dall-e-3" }
func (p *openaiDalleProvider) supportedSizes() []string {
	return []string{"1024x1024", "1024x1792", "1792x1024"}
}

func (p *openaiDalleProvider) generate(ctx context.Context, apiKey, model, prompt, size string) (string, error) {
	return callImageAPI(ctx, callImageAPIParams{
		endpoint: "https://api.openai.com/v1/images/generations",
		apiKey:   apiKey,
		model:    model,
		prompt:   prompt,
		size:     size,
	})
}

// --- MiniMax image-01 provider ---

type minimaxImageProvider struct{}

func (p *minimaxImageProvider) name() string         { return "minimax" }
func (p *minimaxImageProvider) defaultModel() string { return "image-01" }
func (p *minimaxImageProvider) supportedSizes() []string {
	return []string{"1024x1024", "1280x720", "1152x864", "1248x832", "832x1248", "864x1152", "720x1280"}
}

// sizeToAspectRatio maps pixel sizes to MiniMax aspect_ratio values.
var sizeToAspectRatio = map[string]string{
	"1024x1024": "1:1",
	"1280x720":  "16:9",
	"1152x864":  "4:3",
	"1248x832":  "3:2",
	"832x1248":  "2:3",
	"864x1152":  "3:4",
	"720x1280":  "9:16",
	"1344x576":  "21:9",
}

type minimaxImageRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	AspectRatio    string `json:"aspect_ratio,omitempty"`
	ResponseFormat string `json:"response_format"`
	N              int    `json:"n"`
}

type minimaxImageResponse struct {
	Data struct {
		ImageURLs []string `json:"image_urls"`
	} `json:"data"`
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

func (p *minimaxImageProvider) generate(ctx context.Context, apiKey, model, prompt, size string) (string, error) {
	aspectRatio := sizeToAspectRatio[size]
	if aspectRatio == "" {
		aspectRatio = "1:1"
	}

	reqBody := minimaxImageRequest{
		Model:          model,
		Prompt:         prompt,
		AspectRatio:    aspectRatio,
		ResponseFormat: "url",
		N:              1,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.minimaxi.com/v1/image_generation", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp minimaxImageResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if apiResp.BaseResp.StatusCode != 0 {
		return "", fmt.Errorf("API error [%d]: %s", apiResp.BaseResp.StatusCode, apiResp.BaseResp.StatusMsg)
	}

	if len(apiResp.Data.ImageURLs) == 0 || apiResp.Data.ImageURLs[0] == "" {
		return "", fmt.Errorf("no image URL in response")
	}

	return apiResp.Data.ImageURLs[0], nil
}

// --- Shared HTTP helpers ---

type callImageAPIParams struct {
	endpoint string
	apiKey   string
	model    string
	prompt   string
	size     string
}

// imageAPIRequest is the common request format for OpenAI-compatible image APIs.
type imageAPIRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Size   string `json:"size,omitempty"`
}

// imageAPIResponse is the common response format for OpenAI-compatible image APIs.
type imageAPIResponse struct {
	Data []struct {
		URL string `json:"url"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Code    any    `json:"code"` // can be string or int depending on provider
	} `json:"error,omitempty"`
}

func callImageAPI(ctx context.Context, p callImageAPIParams) (string, error) {
	reqBody := imageAPIRequest{
		Model:  p.model,
		Prompt: p.prompt,
		Size:   p.size,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp imageAPIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if apiResp.Error != nil {
		return "", fmt.Errorf("API error [%v]: %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	if len(apiResp.Data) == 0 || apiResp.Data[0].URL == "" {
		return "", fmt.Errorf("no image URL in response")
	}

	return apiResp.Data[0].URL, nil
}

func downloadImage(ctx context.Context, imageURL, outputPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	w, err := fileop.CreateFileAtomic(outputPath, 0o644)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		w.Abort()
		return fmt.Errorf("write file: %w", err)
	}
	return w.Close()
}

const (
	imageTextPolicyAuto        = "auto"
	imageTextPolicyForbid      = "forbid"
	imageTextPolicyShortText   = "short_text"
	imageTextPolicyOverlayBase = "overlay_base"
)

type imagePromptOptions struct {
	StylePreset string
	Quality     string
	TextPolicy  string
}

// enhanceImagePrompt applies prompt engineering best practices for AI image generation.
// Based on research: Ideogram 3.0, FLUX.2, DALL-E 3 prompt guidelines.
// Core strategy: prevent text garbling by default while allowing explicit short
// text requests and overlay-base figures without contradictory no-text suffixes.
func enhanceImagePrompt(prompt, stylePreset, quality string) string {
	return enhanceImagePromptWithOptions(prompt, imagePromptOptions{
		StylePreset: stylePreset,
		Quality:     quality,
		TextPolicy:  imageTextPolicyAuto,
	})
}

func enhanceImagePromptWithOptions(prompt string, opts imagePromptOptions) string {
	stylePreset := strings.ToLower(strings.TrimSpace(opts.StylePreset))
	if stylePreset == "" {
		stylePreset = "paper"
	}
	quality := strings.ToLower(strings.TrimSpace(opts.Quality))
	if quality == "" {
		quality = "high"
	}
	textPolicy := strings.ToLower(strings.TrimSpace(opts.TextPolicy))
	if textPolicy == "" {
		textPolicy = imageTextPolicyAuto
	}
	lower := strings.ToLower(prompt)

	var suffixes []string
	darkIntent := strings.Contains(lower, "dark background") || strings.Contains(lower, "black background")
	photoIntent := strings.Contains(lower, "photorealistic") || strings.Contains(lower, "photo-realistic") || strings.Contains(lower, "photograph")

	switch resolveImageTextPolicy(prompt, textPolicy) {
	case imageTextPolicyShortText:
		if !hasNoTextConstraint(lower) && !strings.Contains(lower, "only the requested text") {
			suffixes = append(suffixes, "Include only the explicitly requested short text, with clean legible typography, and do not add any extra words, labels, numbers, or annotations")
		}
	case imageTextPolicyOverlayBase:
		if hasNoTextConstraint(lower) {
			if !hasOverlaySpaceGuidance(lower) {
				suffixes = append(suffixes, "leave clean open space for later overlay labels")
			}
		} else {
			suffixes = append(suffixes, "Do not include any text, labels, numbers, formulas, legends, axes, or annotations; leave clean open space for later overlay labels")
		}
	default:
		if !hasNoTextConstraint(lower) {
			suffixes = append(suffixes, "Do not include any text, labels, numbers, formulas, legends, axes, or annotations in the image")
		}
	}

	// Academic style for paper figures
	if stylePreset == "paper" && !darkIntent && !strings.Contains(lower, "white background") && !strings.Contains(lower, "light background") {
		suffixes = append(suffixes, "white background")
	}
	if stylePreset == "paper" && !photoIntent && !strings.Contains(lower, "scientific") && !strings.Contains(lower, "academic") {
		suffixes = append(suffixes, "clean scientific illustration style")
	}
	if stylePreset != "none" && !strings.Contains(lower, "low clutter") && !strings.Contains(lower, "minimal clutter") {
		suffixes = append(suffixes, "low clutter composition")
	}

	// Quality boost
	if quality == "high" && !strings.Contains(lower, "high resolution") && !strings.Contains(lower, "high quality") {
		suffixes = append(suffixes, "high resolution, clean lines")
	}

	if len(suffixes) == 0 {
		return prompt
	}
	return prompt + ". " + strings.Join(suffixes, ", ")
}

func validImageTextPolicy(policy string) bool {
	switch policy {
	case imageTextPolicyAuto, imageTextPolicyForbid, imageTextPolicyShortText, imageTextPolicyOverlayBase:
		return true
	default:
		return false
	}
}

func resolveImageTextPolicy(prompt, policy string) string {
	if policy == imageTextPolicyAuto {
		if hasPreciseTextIntent(prompt) {
			return imageTextPolicyOverlayBase
		}
		if hasShortTextIntent(prompt) {
			return imageTextPolicyShortText
		}
		return imageTextPolicyForbid
	}
	if !validImageTextPolicy(policy) {
		return imageTextPolicyAuto
	}
	return policy
}

func hasNoTextConstraint(lowerPrompt string) bool {
	return strings.Contains(lowerPrompt, "no text") ||
		strings.Contains(lowerPrompt, "without text")
}

func hasShortTextIntent(prompt string) bool {
	lower := strings.ToLower(prompt)
	if strings.Contains(lower, "short text") ||
		strings.Contains(lower, "single word") ||
		strings.Contains(lower, "the word ") ||
		strings.Contains(lower, "with the word ") ||
		containsWordPhrase(lower, "with text ") ||
		containsWordPhrase(lower, "text \"") ||
		containsWordPhrase(lower, "label \"") ||
		containsWordPhrase(lower, "says \"") ||
		containsWordPhrase(lower, "reads \"") {
		return true
	}
	return false
}

func containsWordPhrase(s, phrase string) bool {
	start := 0
	for {
		idx := strings.Index(s[start:], phrase)
		if idx < 0 {
			return false
		}
		idx += start
		if idx == 0 || !isASCIIWordByte(s[idx-1]) {
			return true
		}
		start = idx + 1
	}
}

func isASCIIWordByte(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '_'
}

func hasOverlaySpaceGuidance(lowerPrompt string) bool {
	return strings.Contains(lowerPrompt, "leave clean open space") ||
		strings.Contains(lowerPrompt, "open space for later overlay") ||
		strings.Contains(lowerPrompt, "space for later overlay")
}

func hasPreciseTextIntent(prompt string) bool {
	lower := strings.ToLower(prompt)
	needles := []string{
		"axis", "axes", "x-axis", "y-axis", "legend", "tick mark", "tick label",
		"formula", "equation", "latex", "coordinate", "data label", "exact label",
		"precise label", "numbered", "numbered steps", "caption", "table", "bar chart",
		"line chart", "scatter plot", "plot", "graph with", "flowchart", "architecture diagram",
	}
	for _, needle := range needles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func imageTextRouteWarning(prompt, policy string) string {
	resolved := resolveImageTextPolicy(prompt, policy)
	if hasPreciseTextIntent(prompt) {
		return "precise labels, formulas, axes, legends, or data should be rendered with DiagramGen/matplotlib/TikZ, or added as a TikZ/LaTeX overlay on this no-text base image."
	}
	if resolved == imageTextPolicyShortText {
		return "AI-rendered text is unreliable; verify the requested short text visually and prefer overlay text for publication-quality labels."
	}
	return ""
}

func truncatePrompt(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func validateImageSize(prov imageProvider, size string) error {
	supported := prov.supportedSizes()
	for _, s := range supported {
		if s == size {
			return nil
		}
	}
	return fmt.Errorf("unsupported size %q for provider %s (supported: %s)", size, prov.name(), strings.Join(supported, ", "))
}

func resolveImageSize(prov imageProvider, requested, stylePreset string) string {
	if strings.TrimSpace(requested) != "" {
		return strings.TrimSpace(requested)
	}
	if stylePreset != "paper" {
		return "1024x1024"
	}
	switch prov.name() {
	case "openai":
		return "1792x1024"
	case "glm":
		return "1344x768"
	case "minimax":
		return "1248x832"
	default:
		return "1024x1024"
	}
}
