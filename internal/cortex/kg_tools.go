// Package cortex provides CortexDB-backed intelligent recall.
//
// This file implements KG tools — exposing knowledge graph query
// and analysis capabilities as callable tools for the agent.
package cortex

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// KGToolManager wraps GraphFlowService and provides knowledge graph
// tools for the agent, including SPARQL queries and graph analysis.
type KGToolManager struct {
	graphFlow *GraphFlowService
}

// NewKGToolManager creates a KG tool manager.
func NewKGToolManager(
	graphFlow *GraphFlowService,
) *KGToolManager {
	return &KGToolManager{graphFlow: graphFlow}
}

// Tools returns knowledge graph function tools.
func (m *KGToolManager) Tools() []tool.Tool {
	return []tool.Tool{
		function.NewFunctionTool(
			m.queryKnowledgeGraph,
			function.WithName("knowledge_graph_query"),
			function.WithDescription(
				"Query the knowledge graph using SPARQL. "+
					"Use this to find entities, relationships, "+
					"and patterns across all stored knowledge. "+
					"The knowledge graph contains entities "+
					"extracted from conversations, files, "+
					"and system interactions.",
			),
		),
		function.NewFunctionTool(
			m.analyzeGraph,
			function.WithName("knowledge_graph_analyze"),
			function.WithDescription(
				"Analyze the knowledge graph to discover "+
					"patterns, key entities, and structural "+
					"insights. Returns a summary of graph "+
					"statistics and important connections.",
			),
		),
	}
}

// KGQueryReq is the input for querying the knowledge graph.
type KGQueryReq struct {
	Query string `json:"query" jsonschema:"description=SPARQL query to execute against the knowledge graph"`
}

// KGQueryRsp is the output for querying the knowledge graph.
type KGQueryRsp struct {
	Success bool   `json:"success"`
	Result  string `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
}

// queryKnowledgeGraph executes a SPARQL query against the KG.
func (m *KGToolManager) queryKnowledgeGraph(
	ctx context.Context, req KGQueryReq,
) (KGQueryRsp, error) {
	result, err := m.graphFlow.QueryKnowledge(ctx, req.Query)
	if err != nil {
		return KGQueryRsp{
			Success: false,
			Error:   err.Error(),
		}, nil
	}
	return KGQueryRsp{
		Success: true,
		Result:  result,
	}, nil
}

// KGAnalyzeReq is the input for graph analysis.
type KGAnalyzeReq struct{}

// KGAnalyzeRsp is the output for graph analysis.
type KGAnalyzeRsp struct {
	Success     bool   `json:"success"`
	Summary     string `json:"summary,omitempty"`
	EntityCount int    `json:"entity_count,omitempty"`
	EdgeCount   int    `json:"edge_count,omitempty"`
	Error       string `json:"error,omitempty"`
}

// analyzeGraph analyzes the knowledge graph and returns insights.
func (m *KGToolManager) analyzeGraph(
	ctx context.Context, req KGAnalyzeReq,
) (KGAnalyzeRsp, error) {
	// Run a statistics SPARQL query.
	countQuery := `
		SELECT (COUNT(DISTINCT ?s) AS ?entities)
			   (COUNT(DISTINCT ?p) AS ?predicates)
			   (COUNT(*) AS ?triples)
		WHERE { ?s ?p ?o }
	`
	result, err := m.graphFlow.QueryKnowledge(ctx, countQuery)
	if err != nil {
		return KGAnalyzeRsp{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	// Parse SPARQL JSON result to extract entity/edge counts.
	entityCount, predicateCount := parseSPARQLCounts(result)

	return KGAnalyzeRsp{
		Success:     true,
		Summary:     result,
		EntityCount: entityCount,
		EdgeCount:   predicateCount,
	}, nil
}

// parseSPARQLCounts extracts entity and predicate counts from
// a SPARQL SELECT query result. The result is expected to be a
// JSON object with bindings for ?entities, ?predicates, ?triples.
// Falls back to 0 counts on parse failure.
func parseSPARQLCounts(raw string) (entities int, edges int) {
	// Try JSON parsing first (standard SPARQL JSON result format).
	var result struct {
		Results struct {
			Bindings []map[string]struct {
				Value string `json:"value"`
			} `json:"bindings"`
		} `json:"results"`
	}
	// Extract JSON if embedded in text.
	jsonStr := extractJSON(raw)
	if jsonStr == "" {
		return 0, 0
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return 0, 0
	}
	if len(result.Results.Bindings) == 0 {
		return 0, 0
	}
	bindings := result.Results.Bindings[0]
	if v, ok := bindings["entities"]; ok {
		entities, _ = strconv.Atoi(strings.Split(v.Value, ".")[0])
	}
	if v, ok := bindings["predicates"]; ok {
		edges, _ = strconv.Atoi(strings.Split(v.Value, ".")[0])
	}
	return entities, edges
}
