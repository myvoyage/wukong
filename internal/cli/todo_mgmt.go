// Package cli provides the "wukong todo" command for task
// management overview.
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/km269/wukong/internal/config"
)

// newTodoCmd creates the "wukong todo" command group.
func newTodoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "todo",
		Short: "Show task management status",
		Long: `Display the configuration and status of the agent's
task management system including the native TODO tool
and the TODO enforcer.

The TODO system ensures the agent maintains a structured
task list during multi-step execution.

Subcommands:
  status   Show TODO system configuration`,
	}

	cmd.AddCommand(newTodoStatusCmd())

	return cmd
}

func newTodoStatusCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show TODO system status",
		Long: `Display the task management configuration including
backend, tool availability, and enforcer settings.

Examples:
  wukong todo status`,
		RunE: runTodoStatus,
	}

	cmd.Flags().StringVarP(
		&configPath, "config", "c", "",
		"Path to config file")

	return cmd
}

func runTodoStatus(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")

	loader, err := config.NewLoader(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	wukongCfg, err := loader.Load()
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	tc := &wukongCfg.Todo
	ac := &wukongCfg.Agent

	fmt.Println(strings.Repeat("─", 50))
	fmt.Println("  Task Management (TODO) Status")
	fmt.Println(strings.Repeat("─", 50))

	fmt.Println("\n  [Storage]")
	fmt.Printf("  Backend:      %s\n", tc.Backend)
	fmt.Printf("  Database:     %s\n", tc.DBPath)

	fmt.Println("\n  [Agent Integration]")
	fmt.Printf("  Native TODO Tool:      %v\n", tc.EnableNativeTodo)
	fmt.Printf("  TODO Enforcer:         %v\n", tc.EnableEnforcer)
	fmt.Printf("  Agent TODO Enabled:    %v\n", ac.TodoToolEnabled)
	fmt.Printf("  Agent Enforcer:        %v\n", ac.TodoEnforcerEnabled)

	// Compute effective status
	nativeOK := tc.EnableNativeTodo && ac.TodoToolEnabled
	enforcerOK := tc.EnableEnforcer && ac.TodoEnforcerEnabled

	fmt.Println("\n  [Effective Status]")
	if nativeOK {
		fmt.Println("  ✓ TODO tool:       Active — agent can manage tasks")
	} else {
		fmt.Println("  ✗ TODO tool:       Inactive")
		if !tc.EnableNativeTodo {
			fmt.Println("     → enable todo.enable_native_todo in config")
		}
		if !ac.TodoToolEnabled {
			fmt.Println("     → enable agent.todo_tool_enabled in config")
		}
	}

	if enforcerOK {
		fmt.Println("  ✓ TODO enforcer:   Active — agent must maintain tasks")
	} else {
		fmt.Println("  ✗ TODO enforcer:   Inactive")
		if !tc.EnableEnforcer {
			fmt.Println("     → enable todo.enable_enforcer in config")
		}
		if !ac.TodoEnforcerEnabled {
			fmt.Println("     → enable agent.todo_enforcer_enabled in config")
		}
	}

	fmt.Println()
	return nil
}
