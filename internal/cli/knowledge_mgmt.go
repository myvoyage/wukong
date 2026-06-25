// Package cli provides the "wukong knowledge" command for
// knowledge base (RAG) management.
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/km269/wukong/internal/config"
)

// newKnowledgeCmd creates the "wukong knowledge" command group.
func newKnowledgeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "knowledge",
		Short: "Manage the knowledge base (RAG)",
		Long: `View configuration and status of the knowledge base
(Retrieval-Augmented Generation) system.

Subcommands:
  status   Show knowledge base configuration and status`,
	}

	cmd.AddCommand(newKnowledgeStatusCmd())

	return cmd
}

func newKnowledgeStatusCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show knowledge base status",
		Long: `Display the current knowledge base configuration including
embedder model, vector store, and configured document sources.

Examples:
  wukong knowledge status`,
		RunE: runKnowledgeStatus,
	}

	cmd.Flags().StringVarP(
		&configPath, "config", "c", "",
		"Path to config file")

	return cmd
}

func runKnowledgeStatus(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")

	loader, err := config.NewLoader(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	wukongCfg, err := loader.Load()
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	kc := &wukongCfg.Knowledge

	fmt.Println(strings.Repeat("─", 50))
	fmt.Println("  Knowledge Base Status")
	fmt.Println(strings.Repeat("─", 50))

	if !kc.Enabled {
		fmt.Println("\n  Status: disabled")
		fmt.Println("\n  Enable with:")
		fmt.Println("    knowledge:")
		fmt.Println("      enabled: true")
		fmt.Println("      embedder_provider: lmstudio")
		fmt.Println("      embedder_model: text-embedding-nomic-embed-text-v1.5")
		return nil
	}

	fmt.Println("\n  Status: enabled")
	fmt.Printf("  Embedder Provider: %s\n", kc.EmbedderProvider)
	fmt.Printf("  Embedder Model:    %s\n", kc.EmbedderModel)
	fmt.Printf("  Vector Store:      %s\n", kc.VectorStore)
	fmt.Printf("  Max Results:       %d\n", kc.MaxResults)
	fmt.Printf("  Source Sync:       %v\n", kc.EnableSourceSync)
	fmt.Printf("  Search Tool:       %s\n", kc.SearchToolName)

	// Document sources
	fmt.Println("\n  [Document Sources]")
	if len(kc.Sources) == 0 && len(kc.SourceURLs) == 0 {
		fmt.Println("    (no sources configured)")
		fmt.Println("\n  Add sources in config.yaml:")
		fmt.Println("    knowledge:")
		fmt.Println("      sources:")
		fmt.Println("        - ./docs")
		fmt.Println("      source_urls:")
		fmt.Println("        - https://example.com/doc.md")
	} else {
		if len(kc.Sources) > 0 {
			for _, s := range kc.Sources {
				fmt.Printf("    📁 %s\n", s)
			}
		}
		if len(kc.SourceURLs) > 0 {
			for _, u := range kc.SourceURLs {
				fmt.Printf("    🌐 %s\n", u)
			}
		}
	}

	fmt.Println()
	return nil
}
