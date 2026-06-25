// Package cli provides the "wukong memory" command for memory
// inspection and management.
package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/km269/wukong/internal/config"
	"github.com/km269/wukong/internal/memory"
	"github.com/km269/wukong/internal/util"

	tRPCMemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

// newMemoryCmd creates the "wukong memory" command group.
func newMemoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Manage agent long-term memories",
		Long: `View, search, and manage the agent's long-term memories
stored across conversation sessions.

Subcommands:
  list    List all memories for the current user
  search  Search memories by keyword
  delete  Delete a specific memory by ID
  clear   Clear all memories for the current user`,
	}

	cmd.AddCommand(newMemoryListCmd())
	cmd.AddCommand(newMemorySearchCmd())
	cmd.AddCommand(newMemoryDeleteCmd())
	cmd.AddCommand(newMemoryClearCmd())

	return cmd
}

// ==========================================================================
// memory list
// ==========================================================================

func newMemoryListCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all stored memories",
		Long: `List all long-term memories stored by the agent
for the current user.

Examples:
  wukong memory list
  wukong memory ls`,
		RunE: runMemoryList,
	}

	cmd.Flags().StringVarP(
		&configPath, "config", "c", "",
		"Path to config file")

	return cmd
}

func runMemoryList(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")

	memSvc, cleanup, err := createMemoryService(configPath)
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	entries, err := memSvc.ReadMemories(ctx, tRPCMemory.UserKey{
		AppName: "wukong-app",
		UserID:  resolveUserID(),
	}, 0)
	if err != nil {
		return fmt.Errorf("read memories: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("No memories stored yet.")
		fmt.Println("Memories are automatically extracted during conversations",
			"when auto_extract is enabled.")
		return nil
	}

	fmt.Printf("Memories (%d total):\n\n", len(entries))

	for i, e := range entries {
		content := ""
		if e.Memory != nil {
			content = e.Memory.Memory
		}
		if len(content) > 120 {
			content = content[:117] + "..."
		}
		fmt.Printf("  %2d. [%s] %s\n", i+1,
			e.UpdatedAt.Format("2006-01-02"), content)
	}

	return nil
}

// ==========================================================================
// memory search
// ==========================================================================

func newMemorySearchCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "search <keyword>",
		Short: "Search memories by keyword",
		Long: `Search stored memories for entries matching the given keyword.

Examples:
  wukong memory search python
  wukong memory search "project structure"`,
		RunE: runMemorySearch,
		Args: cobra.MinimumNArgs(1),
	}

	cmd.Flags().StringVarP(
		&configPath, "config", "c", "",
		"Path to config file")

	return cmd
}

func runMemorySearch(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")
	configPath, _ := cmd.Flags().GetString("config")

	memSvc, cleanup, err := createMemoryService(configPath)
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	entries, err := memSvc.SearchMemories(ctx, tRPCMemory.UserKey{
		AppName: "wukong-app",
		UserID:  resolveUserID(),
	}, query)
	if err != nil {
		return fmt.Errorf("search memories: %w", err)
	}

	if len(entries) == 0 {
		fmt.Printf("No memories found for: %s\n", query)
		return nil
	}

	fmt.Printf("Results for \"%s\" (%d found):\n\n", query, len(entries))

	for i, e := range entries {
		content := ""
		if e.Memory != nil {
			content = e.Memory.Memory
		}
		fmt.Printf("  %2d. [%s] %s\n", i+1,
			e.UpdatedAt.Format("2006-01-02"), content)
	}

	return nil
}

// ==========================================================================
// memory delete
// ==========================================================================

func newMemoryDeleteCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:     "delete <memory-id>",
		Aliases: []string{"rm"},
		Short:   "Delete a specific memory by ID",
		Long: `Delete a specific memory entry by its ID.
Use "wukong memory list" to find memory IDs.

Examples:
  wukong memory delete mem_abc123
  wukong memory rm mem_abc123`,
		RunE: runMemoryDelete,
		Args: cobra.ExactArgs(1),
	}

	cmd.Flags().StringVarP(
		&configPath, "config", "c", "",
		"Path to config file")

	return cmd
}

func runMemoryDelete(cmd *cobra.Command, args []string) error {
	memoryID := args[0]
	configPath, _ := cmd.Flags().GetString("config")

	memSvc, cleanup, err := createMemoryService(configPath)
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := memSvc.DeleteMemory(ctx, tRPCMemory.Key{
		AppName:  "wukong-app",
		UserID:   resolveUserID(),
		MemoryID: memoryID,
	}); err != nil {
		return fmt.Errorf("delete memory: %w", err)
	}

	fmt.Printf("Memory %q deleted.\n", memoryID)
	return nil
}

// ==========================================================================
// memory clear
// ==========================================================================

func newMemoryClearCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear all memories for the current user",
		Long: `Remove all long-term memories for the current user.
Use with caution — this operation cannot be undone.

Examples:
  wukong memory clear
  wukong memory clear --config ./my-config.yaml`,
		RunE: runMemoryClear,
	}

	cmd.Flags().StringVarP(
		&configPath, "config", "c", "",
		"Path to config file")

	return cmd
}

func runMemoryClear(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")

	memSvc, cleanup, err := createMemoryService(configPath)
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userKey := tRPCMemory.UserKey{
		AppName: "wukong-app",
		UserID:  resolveUserID(),
	}

	// Read all memories first
	entries, err := memSvc.ReadMemories(ctx, userKey, 0)
	if err != nil {
		return fmt.Errorf("read memories: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("No memories to clear.")
		return nil
	}

	// Delete each memory
	deleted := 0
	for _, e := range entries {
		if err := memSvc.DeleteMemory(ctx, tRPCMemory.Key{
			AppName:  userKey.AppName,
			UserID:   userKey.UserID,
			MemoryID: e.ID,
		}); err != nil {
			util.Logger.Warn("failed to delete memory",
				"id", e.ID, "error", err.Error())
		} else {
			deleted++
		}
	}

	fmt.Printf("Cleared %d/%d memories.\n", deleted, len(entries))
	return nil
}

// createMemoryService creates a temporary memory service for CLI
// management commands. Returns the service and a cleanup function.
func createMemoryService(
	configPath string,
) (tRPCMemory.Service, func(), error) {
	loader, err := config.NewLoader(configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}
	wukongCfg, err := loader.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("parse config: %w", err)
	}

	pool := util.NewDatabasePool(config.ResolvePath(wukongCfg.Memory.DBPath))

	// Create without extractor model (memory CLI doesn't need auto-extract)
	memMgr, err := memory.NewMemoryManager(
		&wukongCfg.Memory,
		nil, // no extractor model for CLI
		pool,
	)
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("create memory manager: %w", err)
	}

	cleanup := func() {
		_ = memMgr.Close()
	}

	return memMgr.Service(), cleanup, nil
}
