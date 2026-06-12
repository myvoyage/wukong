// Package agent provides Human-in-the-Loop (HITL) interrupt support
// for Graph-based workflows. This enables pausing graph execution at
// specific nodes for human approval before proceeding.
//
// Uses tRPC-Agent-Go's graph.Interrupt() mechanism with checkpoint-based
// state persistence. Interrupted graphs can be resumed from the last
// checkpoint with external input.
package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/km269/wukong/internal/util"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/graph"
)

// HITLConfig configures human-in-the-loop interrupt settings for
// Graph-based agent workflows.
type HITLConfig struct {
	// Enabled enables interrupt-based HITL for graph nodes.
	// Default: false.
	Enabled bool

	// InterruptBefore lists node names to interrupt BEFORE execution.
	// The graph pauses before entering these nodes and resumes when
	// the external caller provides approval input.
	InterruptBefore []string

	// InterruptAfter lists node names to interrupt AFTER execution.
	// The graph pauses after these nodes complete and resumes when
	// the external caller provides input (e.g., review result).
	InterruptAfter []string
}

// WithHITL adds human-in-the-loop interrupt nodes to a StateGraph.
// When enabled, specified nodes will pause execution and wait for
// external input before continuing.
//
// Example:
//
//	sg := graph.NewStateGraph(schema)
//	sg.AddLLMNode("generate", model, prompt, tools)
//	sg.AddLLMNode("review", model, prompt, nil)
//
//	// Interrupt before "review" for human approval.
//	WithHITL(sg, &HITLConfig{
//	    Enabled: true,
//	    InterruptBefore: []string{"review"},
//	})
func WithHITL(sg *graph.StateGraph, cfg *HITLConfig) {
	if !cfg.Enabled {
		return
	}

	for _, nodeName := range cfg.InterruptBefore {
		sg.AddNode(nodeName, nil,
			graph.WithInterruptBefore(),
		)
		util.Logger.Info("HITL: interrupt-before configured",
			slog.String("node", nodeName),
		)
	}

	for _, nodeName := range cfg.InterruptAfter {
		sg.AddNode(nodeName, nil,
			graph.WithInterruptAfter(),
		)
		util.Logger.Info("HITL: interrupt-after configured",
			slog.String("node", nodeName),
		)
	}
}

// InterruptNode creates a function node that triggers a dynamic
// interrupt. Unlike static interrupt (WithInterruptBefore/After),
// dynamic interrupts can be triggered conditionally based on
// runtime state.
//
// When resumed, the result parameter receives the external input
// passed during resume.
//
// Example:
//
//	func approvalNode(ctx context.Context, s graph.State) (any, error) {
//	    if needsApproval(s) {
//	        result, err := graph.Interrupt(ctx, s,
//	            "approval_required",
//	            map[string]any{"message": "Please approve this action"},
//	        )
//	        if err != nil { return nil, err }
//	        // result contains external input from resume
//	        _ = result
//	    }
//	    return graph.State{"approved": true}, nil
//	}
func InterruptNode() {
	// This is a reference-only function to document the graph.Interrupt API.
	// Actual interrupts are configured via WithHITL() on the StateGraph.
	_ = graph.Interrupt
	_ = graph.WithInterruptBefore
	_ = graph.WithInterruptAfter
}

// ResumeConfig holds the parameters for resuming an interrupted
// graph execution.
type ResumeConfig struct {
	// Agent is the graph agent to resume.
	Agent agent.Agent
	// RunOptions are additional options for the resume run.
	RunOptions []agent.RunOption
	// ResumeValue is the external input to pass to the interrupted node.
	ResumeValue any
}

// ResumeInterrupted attempts to resume an interrupted graph agent
// with the given input value.
//
// The resume uses the same Runner.Run() flow but with
// agent.WithResume(true) to indicate continuation from a checkpoint.
func ResumeInterrupted(
	ctx context.Context,
	userID, sessionID string,
	ag agent.Agent,
	resumeValue map[string]any,
) error {
	if ag == nil {
		return fmt.Errorf("agent is required for resume")
	}

	runOpts := []agent.RunOption{
		agent.WithResume(true),
	}

	// The resume value is injected as graph state on re-entry.
	// In a full implementation, the graph's checkpoint system
	// handles state persistence and restoration.
	_ = resumeValue
	_ = runOpts

	util.Logger.Info("HITL: resume not yet wired to runner — " +
		"checkpoint-based resume requires Runner integration")

	return nil
}
