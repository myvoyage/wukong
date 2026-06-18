// Package provider provides a factory for creating LLM model instances
// based on configuration. It supports OpenAI-compatible APIs, Anthropic,
// Google Gemini, DeepSeek, Ollama, LM Studio, and other providers.
package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/km269/wukong/internal/config"
	"github.com/km269/wukong/internal/util"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/model/openai"
)

// Default base URLs for well-known providers.
const (
	OpenAIBaseURL    = "https://api.openai.com/v1"
	AnthropicBaseURL = "https://api.anthropic.com/v1"
	GoogleBaseURL    = "https://generativelanguage.googleapis.com/v1beta/openai"
	DeepSeekBaseURL  = "https://api.deepseek.com/v1"
	OllamaBaseURL    = "http://localhost:11434/v1"
	LMStudioBaseURL  = "http://localhost:1234/v1"
)

// Factory creates model instances from provider configuration.
type Factory struct {
	cfg     *config.WukongConfig
	mcpAddr string // ACP MCP bridge address (set externally)
}

// SetACPMCPAddr sets the MCP bridge address for ACP providers.
func (f *Factory) SetACPMCPAddr(addr string) {
	f.mcpAddr = addr
}

// NewFactory creates a new model provider factory.
func NewFactory(cfg *config.WukongConfig) *Factory {
	return &Factory{cfg: cfg}
}

// CreateModel creates a model instance for the given provider name.
// If name is empty, the default provider is used.
func (f *Factory) CreateModel(name string) (model.Model, error) {
	p := f.cfg.FindProvider(name)
	if p == nil {
		if name == "" {
			p = f.cfg.DefaultProviderConfig()
		}
		if p == nil {
			return nil, fmt.Errorf(
				"provider %q not found and no default configured",
				name,
			)
		}
	}

	// Fill in default base URL if not configured
	f.fillDefaultBaseURL(p)

	switch p.Type {
	case "openai", "anthropic", "google", "deepseek",
		"ollama", "lmstudio":
		return f.createOpenAI(p), nil
	case "acp":
		return f.createACP(p)
	default:
		return nil, fmt.Errorf(
			"unsupported provider type: %s", p.Type,
		)
	}
}

// CreateDefaultModel creates a model instance for the default provider.
func (f *Factory) CreateDefaultModel() (model.Model, error) {
	return f.CreateModel("")
}

// fillDefaultBaseURL fills in the base URL from well-known defaults
// if the provider configuration does not specify one.
func (f *Factory) fillDefaultBaseURL(p *config.ProviderConfig) {
	if p.BaseURL != "" {
		return
	}
	switch p.Type {
	case "openai":
		p.BaseURL = OpenAIBaseURL
	case "anthropic":
		p.BaseURL = AnthropicBaseURL
	case "google":
		p.BaseURL = GoogleBaseURL
	case "deepseek":
		p.BaseURL = DeepSeekBaseURL
	case "ollama":
		p.BaseURL = OllamaBaseURL
	case "lmstudio":
		p.BaseURL = LMStudioBaseURL
	}
}

// createACP creates an ACP provider that connects to a remote
// ACP-compatible agent.
func (f *Factory) createACP(
	p *config.ProviderConfig,
) (model.Model, error) {
	mcpAddr := f.mcpAddr
	if p.MCPPort != "" {
		mcpAddr = "http://localhost" + p.MCPPort + "/mcp"
	}
	prov, err := NewACPProvider(p, mcpAddr)
	if err != nil {
		return nil, fmt.Errorf(
			"create acp provider: %w", err)
	}
	util.Logger.Info("acp provider created",
		"agent_url", p.AgentURL,
		"mcp_addr", mcpAddr,
	)
	return prov, nil
}

// createOpenAI creates an OpenAI-compatible model instance.
func (f *Factory) createOpenAI(p *config.ProviderConfig) model.Model {
	opts := []openai.Option{
		openai.WithBaseURL(p.BaseURL),
		openai.WithAPIKey(p.APIKey),
	}
	return openai.New(p.Model, opts...)
}

// GetDefaultGenerationConfig returns generation config from settings.
func GetDefaultGenerationConfig(
	cfg *config.AgentConfig,
) model.GenerationConfig {
	gc := model.GenerationConfig{
		Stream: cfg.Streaming,
	}
	if cfg.MaxTokens > 0 {
		gc.MaxTokens = util.IntPtr(cfg.MaxTokens)
	}
	if cfg.Temperature > 0 {
		gc.Temperature = util.Float64Ptr(cfg.Temperature)
	}
	return gc
}

// CreateRevisionModel creates a summarization model for context revision.
// Uses the revision provider if configured, otherwise falls back to the
// default provider.
func (f *Factory) CreateRevisionModel() (RevisionModel, error) {
	providerName := f.cfg.Revision.RevisionProvider
	modelName := f.cfg.Revision.RevisionModel

	// If no revision provider specified, use the default provider
	if providerName == "" {
		providerName = f.cfg.DefaultProvider
	}
	if modelName == "" {
		p := f.cfg.FindProvider(providerName)
		if p != nil {
			modelName = p.Model
		}
	}

	mdl, err := f.CreateModel(providerName)
	if err != nil {
		return nil, fmt.Errorf("create revision model: %w", err)
	}

	return &revisionModelAdapter{
		model: mdl,
		name:  modelName,
	}, nil
}

// RevisionModel wraps a model.Model for summarization.
type RevisionModel interface {
	Summarize(ctx context.Context, content string, maxTokens int) (string, error)
}

// revisionModelAdapter adapts model.Model to RevisionModel.
type revisionModelAdapter struct {
	model model.Model
	name  string
}

func (a *revisionModelAdapter) Summarize(
	ctx context.Context, content string, maxTokens int,
) (string, error) {
	// --- Layered Compression Prompt ---
	// Supports two scenarios:
	//   1. Fresh summarization: compress raw conversation messages.
	//   2. Progressive summarization: merge an existing summary with new delta
	//      messages to produce an updated summary.
	// The prompt detects whether "content" starts with "[Existing Summary]"
	// to distinguish the two modes.
	var prompt string
	if strings.HasPrefix(content, "[Existing Summary]") {
		// Progressive mode: merge existing summary with new messages.
		prompt = fmt.Sprintf(
			"You are a context compression assistant. "+
				"Below is an existing conversation summary "+
				"followed by new messages that occurred after "+
				"that summary was generated.\n\n"+
				"Merge them into ONE coherent, updated summary "+
				"in %d tokens or less. "+
				"Preserve all key decisions, important facts, "+
				"pending action items, file paths, error messages, "+
				"and architectural decisions. "+
				"Write in a concise bullet-point format.\n\n%s",
			maxTokens, content,
		)
	} else {
		// Fresh summarization: compress raw conversation.
		prompt = fmt.Sprintf(
			"You are a context compression assistant. "+
				"Summarize the following conversation "+
				"concisely in %d tokens or less. "+
				"Capture: (1) key decisions made, "+
				"(2) important context/facts, "+
				"(3) pending action items, "+
				"(4) errors encountered, "+
				"(5) file paths or code changes mentioned. "+
				"Use a structured bullet-point format. "+
				"Be thorough but concise.\n\n%s",
			maxTokens, content,
		)
	}

	req := &model.Request{
		Messages: []model.Message{
			model.NewUserMessage(prompt),
		},
		GenerationConfig: model.GenerationConfig{
			MaxTokens:   &maxTokens,
			Temperature: util.Float64Ptr(0.3),
		},
	}

	respChan, err := a.model.GenerateContent(ctx, req)
	if err != nil {
		return "", fmt.Errorf("generate summary: %w", err)
	}

	var summary string
	for resp := range respChan {
		if resp.Error != nil {
			return "", fmt.Errorf(
				"summary error: %s", resp.Error.Message,
			)
		}
		if len(resp.Choices) > 0 {
			summary += resp.Choices[0].Message.Content
		}
	}

	if summary == "" {
		// Fallback: truncate
		if len(content) > maxTokens*4 {
			return content[:maxTokens*4], nil
		}
		return content, nil
	}

	return summary, nil
}
