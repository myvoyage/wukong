// Package cortex provides CortexDB-backed intelligent recall.
//
// This file implements the SessionExtractor interface for MemoryFlow,
// using the Wukong LLM factory to extract promotion candidates from
// conversation transcripts.
package cortex

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/km269/wukong/internal/provider"
	"github.com/km269/wukong/internal/util"

	"github.com/liliang-cn/cortexdb/v2/pkg/memoryflow"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// LLMSessionExtractor implements memoryflow.SessionExtractor by
// using an LLM to analyze conversation transcripts and identify
// facts worth promoting from ephemeral chat to long-term memory
// or the knowledge base.
type LLMSessionExtractor struct {
	factory   *provider.Factory
	modelName string
}

// NewLLMSessionExtractor creates a session extractor backed by
// a Wukong LLM provider.
func NewLLMSessionExtractor(
	factory *provider.Factory,
	modelName string,
) *LLMSessionExtractor {
	return &LLMSessionExtractor{
		factory:   factory,
		modelName: modelName,
	}
}

// llmExtractResult is the JSON structure returned by the LLM
// for fact extraction from conversation transcripts.
type llmExtractResult struct {
	Facts []llmExtractFact `json:"facts"`
}

type llmExtractFact struct {
	Content    string `json:"content"`
	Kind       string `json:"kind"`
	Collection string `json:"collection"`
}

// minExtractContentLen is the minimum content length for a
// heuristic extract candidate to be considered meaningful.
const minExtractContentLen = 20

// transientPrefixes are common short questions/greetings that
// should never be promoted to long-term memory.
var transientPrefixes = []string{
	"what", "how", "when", "where", "who", "why",
	"can you", "could you", "will you", "would you",
	"hello", "hi ", "hey", "good morning", "good afternoon",
	"thanks", "thank you", "ok", "okay",
	"yes", "no", "yep", "nope",
	"什么", "怎么", "为什么", "你好", "谢谢",
	"可以", "能不能", "能否",
}

// Extract analyzes a conversation transcript and identifies facts,
// decisions, preferences, or context worth promoting to long-term
// knowledge.
//
// Strategy (in priority order):
//  1. LLM Extraction: uses the configured extraction model to
//     semantically analyze the transcript and produce structured
//     PromotionCandidates via JSON output.
//  2. Heuristic Fallback: when no factory/model is available, or
//     the LLM call fails, falls back to deterministic keyword
//     matching for English and Chinese inputs with deduplication.
func (e *LLMSessionExtractor) Extract(
	ctx context.Context,
	transcript memoryflow.Transcript,
	state memoryflow.SessionState,
) ([]memoryflow.PromotionCandidate, error) {
	// Build the transcript text for analysis.
	var transcriptText strings.Builder
	for _, turn := range transcript.Turns {
		transcriptText.WriteString(
			fmt.Sprintf("%s: %s\n", turn.Role, turn.Content),
		)
	}

	// Attempt LLM-based extraction when factory is available.
	if e.factory != nil && e.modelName != "" {
		mdl, err := e.factory.CreateModel(e.modelName)
		if err == nil && mdl != nil {
			candidates, llmErr := e.llmExtract(
				ctx, mdl, transcriptText.String(), state,
			)
			if llmErr == nil {
				return candidates, nil
			}
			util.Logger.Warn("memoryflow: LLM extract failed, "+
				"falling back to heuristic",
				slog.String("error", llmErr.Error()))
		}
	}

	// Fallback: deterministic extraction using pattern matching.
	return e.heuristicExtract(transcript, state), nil
}

// llmExtract calls the LLM to analyze a transcript and extract
// structured PromotionCandidates. Uses a guided prompt with JSON
// output format for reliable parsing.
func (e *LLMSessionExtractor) llmExtract(
	ctx context.Context,
	mdl model.Model,
	transcriptText string,
	state memoryflow.SessionState,
) ([]memoryflow.PromotionCandidate, error) {
	prompt := fmt.Sprintf(
		"You are a Knowledge Extraction Assistant. "+
			"Analyze the following conversation transcript and "+
			"extract important facts, decisions, and preferences "+
			"that are worth remembering across sessions.\n\n"+
			"<rules>\n"+
			"1. Only extract information shared or confirmed by "+
			"the user, not generic statements.\n"+
			"2. One fact per entry. Be specific and concise.\n"+
			"3. Kind must be one of: preference, decision, note.\n"+
			"4. If nothing significant is found, return an empty "+
			"facts array.\n"+
			"5. Output ONLY valid JSON, no extra text.\n"+
			"6. Do NOT extract transient queries like \"what time "+
			"is it\" or greetings.\n"+
			"</rules>\n\n"+
			"Transcript:\n%s\n\n"+
			"Output format:\n"+
			`{"facts": [{"content": "...", "kind": "...", `+
			`"collection": "..."}]}`,
		transcriptText,
	)

	llmCtx, cancel := context.WithTimeout(
		ctx, 120*time.Second,
	)
	defer cancel()

	req := &model.Request{
		Messages: []model.Message{
			model.NewUserMessage(prompt),
		},
		GenerationConfig: model.GenerationConfig{
			Temperature: util.Float64Ptr(0.3),
			Stream:      false,
		},
	}

	respChan, err := mdl.GenerateContent(llmCtx, req)
	if err != nil {
		return nil, fmt.Errorf(
			"llm extract: generate: %w", err)
	}

	var fullResponse string
	for resp := range respChan {
		if resp.Error != nil {
			return nil, fmt.Errorf(
				"llm extract: API error: %s",
				resp.Error.Message)
		}
		if len(resp.Choices) > 0 {
			fullResponse += resp.Choices[0].Message.Content
		}
	}

	if fullResponse == "" {
		return nil, fmt.Errorf("llm extract: empty response")
	}

	// Parse JSON response into structured candidates.
	var result llmExtractResult
	fullResponse = extractJSON(fullResponse)
	if err := json.Unmarshal(
		[]byte(fullResponse), &result,
	); err != nil {
		return nil, fmt.Errorf(
			"llm extract: parse JSON: %w", err)
	}

	var candidates []memoryflow.PromotionCandidate
	seen := make(map[string]bool)
	for _, f := range result.Facts {
		if f.Content == "" || len(f.Content) < minExtractContentLen {
			continue
		}
		// Deduplicate: skip identical content within this batch.
		key := strings.ToLower(strings.TrimSpace(f.Content))
		if seen[key] {
			continue
		}
		seen[key] = true

		if f.Collection == "" {
			f.Collection = f.Kind + "s"
		}
		candidates = append(candidates,
			memoryflow.PromotionCandidate{
				Content:    f.Content,
				Kind:       toPromotionKind(f.Kind),
				Author:     state.UserID,
				Collection: f.Collection,
			},
		)
	}

	return candidates, nil
}

// toPromotionKind maps string kind values to memoryflow.PromotionKind.
// Falls back to PromotionKindNote for unrecognised kinds.
func toPromotionKind(kind string) memoryflow.PromotionKind {
	switch kind {
	case "decision":
		return memoryflow.PromotionKindDecision
	case "preference":
		return memoryflow.PromotionKindPreference
	case "milestone":
		return memoryflow.PromotionKindMilestone
	case "problem":
		return memoryflow.PromotionKindProblem
	case "note", "fact":
		return memoryflow.PromotionKindNote
	default:
		return memoryflow.PromotionKindNote
	}
}

// extractJSON extracts JSON content from an LLM response that may
// contain markdown code fences or surrounding text.
func extractJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, "```json"); idx >= 0 {
		start := idx + 7
		if end := strings.Index(
			raw[start:], "```",
		); end >= 0 {
			return strings.TrimSpace(raw[start : start+end])
		}
	}
	if idx := strings.Index(raw, "```"); idx >= 0 {
		start := idx + 3
		if end := strings.Index(
			raw[start:], "```",
		); end >= 0 {
			return strings.TrimSpace(raw[start : start+end])
		}
	}
	if start := strings.Index(raw, "{"); start >= 0 {
		if end := strings.LastIndex(raw, "}"); end > start {
			return raw[start : end+1]
		}
	}
	return raw
}

// isTransient checks whether the content looks like a transient
// question, greeting, or trivial message that should not be
// promoted to long-term memory.
func isTransient(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	if len(lower) < minExtractContentLen {
		return true
	}
	for _, prefix := range transientPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// heuristicExtract provides a deterministic fallback extraction
// when no LLM is available. Supports both English and Chinese
// keyword patterns with deduplication and transient filtering.
func (e *LLMSessionExtractor) heuristicExtract(
	transcript memoryflow.Transcript,
	state memoryflow.SessionState,
) []memoryflow.PromotionCandidate {
	var candidates []memoryflow.PromotionCandidate
	seen := make(map[string]bool)

	for _, turn := range transcript.Turns {
		if turn.Role != "user" {
			continue
		}

		content := strings.ToLower(turn.Content)

		// Skip transient queries, greetings, and trivial messages.
		if isTransient(turn.Content) {
			continue
		}

		// Deduplicate: skip if this content was already extracted.
		key := strings.ToLower(strings.TrimSpace(turn.Content))
		if seen[key] {
			continue
		}
		seen[key] = true

		var matched bool

		// Detect preference statements (English).
		if containsAny(content,
			"prefer", "like to", "favorite",
			"i'd rather", "i would rather",
		) {
			candidates = append(candidates,
				memoryflow.PromotionCandidate{
					Content:    turn.Content,
					Kind:       memoryflow.PromotionKindPreference,
					Author:     state.UserID,
					Collection: "preferences",
				},
			)
			matched = true
		}

		// Detect preference statements (Chinese).
		if !matched && containsAnyCJK(content,
			"喜欢", "偏好", "更喜欢", "习惯",
			"常用", "最爱", "觉得好",
		) {
			candidates = append(candidates,
				memoryflow.PromotionCandidate{
					Content:    turn.Content,
					Kind:       memoryflow.PromotionKindPreference,
					Author:     state.UserID,
					Collection: "preferences",
				},
			)
			matched = true
		}

		// Detect decision statements (English).
		if !matched && containsAny(content,
			"decide", "let's", "we'll",
			"we will", "agreed", "choice",
		) {
			candidates = append(candidates,
				memoryflow.PromotionCandidate{
					Content:    turn.Content,
					Kind:       memoryflow.PromotionKindDecision,
					Author:     state.UserID,
					Collection: "decisions",
				},
			)
			matched = true
		}

		// Detect decision statements (Chinese).
		if !matched && containsAnyCJK(content,
			"决定", "确定", "选择",
			"我们就", "方案", "定了",
		) {
			candidates = append(candidates,
				memoryflow.PromotionCandidate{
					Content:    turn.Content,
					Kind:       memoryflow.PromotionKindDecision,
					Author:     state.UserID,
					Collection: "decisions",
				},
			)
			matched = true
		}

		// Detect factual statements (English).
		if !matched && containsAny(content,
			"note that", "remember",
			"for the record", "by the way",
			"works as", "located at",
		) {
			candidates = append(candidates,
				memoryflow.PromotionCandidate{
					Content:    turn.Content,
					Kind:       memoryflow.PromotionKindNote,
					Author:     state.UserID,
					Collection: "notes",
				},
			)
			matched = true
		}

		// Detect factual statements (Chinese).
		if !matched && containsAnyCJK(content,
			"注意", "记住", "顺便说一下",
			"我在", "我的", "公司",
			"项目", "地址", "电话",
		) {
			candidates = append(candidates,
				memoryflow.PromotionCandidate{
					Content:    turn.Content,
					Kind:       memoryflow.PromotionKindNote,
					Author:     state.UserID,
					Collection: "notes",
				},
			)
		}
	}

	return candidates
}

// containsAny checks if content contains any of the given sub-strings.
func containsAny(content string, substrs ...string) bool {
	for _, s := range substrs {
		if strings.Contains(content, s) {
			return true
		}
	}
	return false
}

// containsAnyCJK checks if content contains any of the given CJK
// (Chinese/Japanese/Korean) sub-strings.
func containsAnyCJK(content string, substrs ...string) bool {
	for _, s := range substrs {
		if strings.Contains(content, s) {
			return true
		}
	}
	return false
}
