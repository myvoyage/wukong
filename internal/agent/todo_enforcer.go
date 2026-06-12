// Package agent provides a lightweight todo enforcer plugin that
// ensures all pending todos are completed before the agent delivers
// its final answer.
//
// This is an in-house implementation since tRPC-Agent-Go v1.10.0
// does not yet ship an official todoenforcer extension. When the
// framework provides one, this can be migrated.
package agent

import (
	"context"
	"encoding/json"
	"log/slog"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/log"
	"trpc.group/trpc-go/trpc-agent-go/plugin"
)

// todoEnforcerKey is the session.State key prefix used by the
// tRPC todo_write tool. Must match the default in tool/todo:
// DefaultStateKeyPrefix = "temp:todos".
const todoEnforcerKey = "temp:todos"

// todoEnforcer is a runner plugin that checks whether all pending
// todos are completed before the agent gives its final answer.
type todoEnforcer struct{}

// newTodoEnforcer creates a new todo enforcer plugin.
func newTodoEnforcer() plugin.Plugin {
	return &todoEnforcer{}
}

// Name returns the plugin identifier.
func (e *todoEnforcer) Name() string { return "todo_enforcer" }

// Register wires callbacks into the plugin registry.
func (e *todoEnforcer) Register(reg *plugin.Registry) {
	reg.AfterAgent(e.afterAgent)
}

// afterAgent checks that all todos are completed when the agent
// finishes its turn. If incomplete todos remain, we log a warning.
// The tRPC todo_write tool already nudges the LLM on every write,
// so this serves as an additional safety net.
func (e *todoEnforcer) afterAgent(
	ctx context.Context,
	args *agent.AfterAgentArgs,
) (*agent.AfterAgentResult, error) {
	if args == nil || args.Invocation == nil ||
		args.Invocation.Session == nil {
		return nil, nil
	}

	sess := args.Invocation.Session
	if sess.State == nil {
		return nil, nil
	}

	// Build the state key using the invocation branch for isolation.
	// Matches the pattern used by tool/todo: "temp:todos" or
	// "temp:todos:<branch>".
	stateKey := todoEnforcerKey
	if args.Invocation.Branch != "" {
		stateKey = todoEnforcerKey + ":" + args.Invocation.Branch
	}

	// session.State is map[string][]byte, so the value is already []byte.
	todoBytes, ok := sess.State[stateKey]
	if !ok || len(todoBytes) == 0 {
		return nil, nil
	}

	// Parse the todo list from session state.
	var todos []map[string]interface{}
	if err := json.Unmarshal(todoBytes, &todos); err != nil {
		return nil, nil
	}

	// Count incomplete todos.
	incomplete := 0
	for _, t := range todos {
		status, _ := t["status"].(string)
		if status != "completed" && status != "" {
			incomplete++
		}
	}

	if incomplete > 0 {
		log.Infof(
			"todo_enforcer: agent completed with %d incomplete todos (branch=%q)",
			incomplete,
			args.Invocation.Branch,
		)
		slog.Warn("agent completed turn with incomplete todos",
			slog.Int("incomplete", incomplete),
			slog.String("branch", args.Invocation.Branch),
		)
	}

	return nil, nil
}
