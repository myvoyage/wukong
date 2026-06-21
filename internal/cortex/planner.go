// Package cortex provides CortexDB-backed intelligent recall.
//
// This file implements the QueryPlanner interface for MemoryFlow,
// using the Wukong LLM factory to plan retrieval strategies.
package cortex

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/km269/wukong/internal/provider"
	"github.com/km269/wukong/internal/util"

	cortexdb "github.com/liliang-cn/cortexdb/v2/pkg/cortexdb"
	"github.com/liliang-cn/cortexdb/v2/pkg/memoryflow"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// Retrieval mode string constants used by CortexDB's RetrievalPlan.
const (
	retrievalModeLexical = "lexical"
	retrievalModeVector  = "vector"
	retrievalModeHybrid  = "hybrid"
)

// LLMQueryPlanner implements memoryflow.QueryPlanner using an LLM
// to decide the best retrieval strategy based on the user query
// and session context.
type LLMQueryPlanner struct {
	factory   *provider.Factory
	modelName string
}

// NewLLMQueryPlanner creates a query planner backed by a Wukong
// LLM provider. Uses modelName for the planning model
// (typically a fast/cheap model like gpt-4o-mini).
func NewLLMQueryPlanner(
	factory *provider.Factory,
	modelName string,
) *LLMQueryPlanner {
	return &LLMQueryPlanner{
		factory:   factory,
		modelName: modelName,
	}
}

// Plan determines the optimal retrieval strategy for a given query
// and session state using either LLM-based planning or deterministic
// heuristics when no LLM is available.
func (p *LLMQueryPlanner) Plan(
	ctx context.Context,
	query string,
	state memoryflow.SessionState,
) (*cortexdb.RetrievalPlan, error) {
	// When no LLM is available, use deterministic heuristics.
	if p.factory == nil || p.modelName == "" {
		return p.heuristicPlan(query), nil
	}

	// Attempt LLM-based planning.
	plan, llmErr := p.llmPlan(ctx, query, state)
	if llmErr == nil {
		return plan, nil
	}

	// Fall back to heuristic on any failure.
	return p.heuristicPlan(query), nil
}

// llmPlan calls the LLM to decide the best retrieval strategy.
// Uses a compact prompt with max_tokens=8 to minimise token cost.
func (p *LLMQueryPlanner) llmPlan(
	ctx context.Context,
	query string,
	state memoryflow.SessionState,
) (*cortexdb.RetrievalPlan, error) {
	mdl, err := p.factory.CreateModel(p.modelName)
	if err != nil {
		return nil, fmt.Errorf(
			"planner: create model: %w", err)
	}
	if mdl == nil {
		return nil, fmt.Errorf("planner: model is nil")
	}

	prompt := fmt.Sprintf(
		"You are a retrieval strategy planner. "+
			"Given a user query, decide the best search mode.\n\n"+
			"Query: %s\n\n"+
			"Modes:\n"+
			"- lexical: for exact keyword/entity searches\n"+
			"- vector: for conceptual or abstract queries\n"+
			"- hybrid: for long, complex queries\n\n"+
			"Respond with exactly one word: "+
			"lexical, vector, or hybrid.",
		query,
	)

	planCtx, cancel := context.WithTimeout(
		ctx, 15*time.Second,
	)
	defer cancel()

	req := &model.Request{
		Messages: []model.Message{
			model.NewUserMessage(prompt),
		},
		GenerationConfig: model.GenerationConfig{
			MaxTokens:   util.IntPtr(8),
			Temperature: util.Float64Ptr(0.0),
			Stream:      false,
		},
	}

	respChan, err := mdl.GenerateContent(planCtx, req)
	if err != nil {
		return nil, fmt.Errorf(
			"planner: generate: %w", err)
	}

	var response string
	for resp := range respChan {
		if resp.Error != nil {
			return nil, fmt.Errorf(
				"planner: API error: %s",
				resp.Error.Message)
		}
		if len(resp.Choices) > 0 {
			response += resp.Choices[0].Message.Content
		}
	}

	mode := normalizeMode(response)
	if mode == "" {
		return nil, fmt.Errorf(
			"planner: unrecognised mode: %q", response)
	}

	plan := &cortexdb.RetrievalPlan{
		RetrievalMode: mode,
		Keywords:      extractKeywords(query),
	}
	return plan, nil
}

// normalizeMode maps the LLM response to a recognised retrieval mode.
func normalizeMode(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	switch {
	case strings.Contains(raw, "hybrid"):
		return retrievalModeHybrid
	case strings.Contains(raw, "vector"):
		return retrievalModeVector
	case strings.Contains(raw, "lexical"):
		return retrievalModeLexical
	default:
		return ""
	}
}

// heuristicPlan provides a deterministic fallback based on query
// characteristics.
func (p *LLMQueryPlanner) heuristicPlan(
	query string,
) *cortexdb.RetrievalPlan {
	plan := &cortexdb.RetrievalPlan{
		RetrievalMode: retrievalModeLexical,
	}

	q := strings.ToLower(query)
	wordCount := len(strings.Fields(q))

	if wordCount > 10 {
		plan.RetrievalMode = retrievalModeHybrid
		plan.Keywords = extractKeywords(query)
		return plan
	}

	abstractMarkers := []string{
		"why", "how", "explain", "concept", "idea",
		"mean", "difference", "relation", "summary",
	}
	for _, marker := range abstractMarkers {
		if strings.Contains(q, marker) {
			plan.RetrievalMode = retrievalModeVector
			plan.Keywords = extractKeywords(query)
			return plan
		}
	}

	plan.Keywords = extractKeywords(query)
	return plan
}

// extractKeywords extracts simple keyword tokens from a query.
func extractKeywords(query string) []string {
	words := strings.Fields(strings.ToLower(query))
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true,
		"are": true, "was": true, "were": true, "be": true,
		"to": true, "of": true, "in": true, "for": true,
		"on": true, "with": true, "at": true, "by": true,
		"this": true, "that": true, "it": true, "and": true,
		"or": true, "but": true, "not": true, "so": true,
	}
	var keywords []string
	seen := make(map[string]bool)
	for _, w := range words {
		if len(w) > 2 && !stopWords[w] && !seen[w] {
			keywords = append(keywords, w)
			seen[w] = true
			if len(keywords) >= 5 {
				break
			}
		}
	}
	return keywords
}
