// Package cli provides the "wukong cortex" command for
// CortexDB memory stack status.
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/km269/wukong/internal/config"
)

// newCortexCmd creates the "wukong cortex" command group.
func newCortexCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cortex",
		Short: "Show CortexDB memory stack status",
		Long: `Display the configuration and status of the CortexDB
memory stack including HNSW vector indexing, FTS5 full-text
search, MemoryFlow transcription, GraphFlow knowledge
graphs, and ImportFlow structured data ingestion.

Subcommands:
  status   Show CortexDB stack status`,
	}

	cmd.AddCommand(newCortexStatusCmd())

	return cmd
}

func newCortexStatusCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show CortexDB memory stack status",
		Long: `Display all CortexDB subsystem configurations and
enabled status.

Examples:
  wukong cortex status`,
		RunE: runCortexStatus,
	}

	cmd.Flags().StringVarP(
		&configPath, "config", "c", "",
		"Path to config file")

	return cmd
}

func runCortexStatus(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")

	loader, err := config.NewLoader(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	wukongCfg, err := loader.Load()
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	fmt.Println(strings.Repeat("═", 60))
	fmt.Println("  CortexDB Memory Stack Status")
	fmt.Println(strings.Repeat("═", 60))

	// CortexDB HNSW + FTS5
	printSubsystem("CortexDB (HNSW + FTS5)",
		wukongCfg.Cortex.Enabled,
		fmt.Sprintf("model: %s, max_results: %d",
			wukongCfg.Cortex.EmbeddingModel,
			wukongCfg.Cortex.MaxResults),
	)

	// MemoryFlow
	mfDesc := fmt.Sprintf("namespace: %s, dimensions: %d",
		wukongCfg.MemoryFlow.Namespace,
		wukongCfg.MemoryFlow.EmbeddingDimensions)
	printSubsystem("MemoryFlow (Transcription + WakeUp)",
		wukongCfg.MemoryFlow.Enabled, mfDesc)

	// GraphFlow
	gfDesc := fmt.Sprintf("max_chars: %d, auto_extract: %v",
		wukongCfg.GraphFlow.MaxCharsPerDoc,
		wukongCfg.GraphFlow.AutoExtract)
	printSubsystem("GraphFlow (RDF Knowledge Graph)",
		wukongCfg.GraphFlow.Enabled, gfDesc)

	// ImportFlow
	printSubsystem("ImportFlow (DDL → KG)",
		wukongCfg.ImportFlow.Enabled, "")

	// Recall FTS5
	rcDesc := fmt.Sprintf("mode: %s, max_results: %d",
		wukongCfg.Recall.SearchMode, wukongCfg.Recall.MaxResults)
	printSubsystem("Recall (FTS5 Full-Text Search)",
		wukongCfg.Recall.Enabled, rcDesc)

	fmt.Println()

	// Count active subsystems
	active := 0
	if wukongCfg.Cortex.Enabled {
		active++
	}
	if wukongCfg.MemoryFlow.Enabled {
		active++
	}
	if wukongCfg.GraphFlow.Enabled {
		active++
	}
	if wukongCfg.ImportFlow.Enabled {
		active++
	}
	if wukongCfg.Recall.Enabled {
		active++
	}

	fmt.Printf("  Active: %d/5 subsystems\n", active)
	fmt.Println()

	return nil
}

func printSubsystem(name string, enabled bool, extra string) {
	icon := "●"
	status := "enabled"
	if !enabled {
		icon = "○"
		status = "disabled"
	}

	line := fmt.Sprintf("  %s %s   [%s]", icon, name, status)
	if enabled && extra != "" {
		line += " — " + extra
	}
	fmt.Println(line)
}
