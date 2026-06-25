// Package cli provides the "wukong ard" command for ARD resource
// discovery management.
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/km269/wukong/internal/ard"
	"github.com/km269/wukong/internal/config"
)

// newARDCCmd creates the "wukong ard" command group.
func newARDCCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ard",
		Short: "Manage Agentic Resource Discovery",
		Long: `Manage the ARD (Agentic Resource Discovery) system for
federated agent/server discovery and registration.

Subcommands:
  status   Show ARD configuration and status
  catalog  List locally registered resources`,
	}

	cmd.AddCommand(newARDStatusCmd())
	cmd.AddCommand(newARDCatalogCmd())

	return cmd
}

// ==========================================================================
// ard status
// ==========================================================================

func newARDStatusCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show ARD configuration and status",
		Long: `Display the current ARD configuration including enabled
status, registry URL, publish settings, and port.

Examples:
  wukong ard status`,
		RunE: runARDStatus,
	}

	cmd.Flags().StringVarP(
		&configPath, "config", "c", "",
		"Path to config file")

	return cmd
}

func runARDStatus(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")

	loader, err := config.NewLoader(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	wukongCfg, err := loader.Load()
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	ac := &wukongCfg.ARD

	fmt.Println(strings.Repeat("─", 50))
	fmt.Println("  ARD Status (Agentic Resource Discovery)")
	fmt.Println(strings.Repeat("─", 50))

	if !ac.Enabled {
		fmt.Println("\n  Status: disabled")
		fmt.Println("\n  Enable with:")
		fmt.Println("    ard:")
		fmt.Println("      enabled: true")
		fmt.Println("      registry_url: https://remote.registry")
		fmt.Println("      publish_enabled: true")
		return nil
	}

	fmt.Println("\n  Status: enabled")

	fmt.Println("\n  [Outbound — Discover Others]")
	if ac.RegistryURL != "" {
		fmt.Printf("  Registry URL:  %s\n", ac.RegistryURL)
	} else {
		fmt.Println("  Registry URL:  (not configured)")
	}

	fmt.Println("\n  [Inbound — Let Others Discover You]")
	if ac.PublishEnabled {
		port := ac.PublishPort
		if port == 0 {
			port = 8081
		}
		fmt.Printf("  Publishing:     enabled (port %d)\n", port)
		fmt.Printf("  Catalog URL:    http://localhost:%d/.well-known/ai-catalog.json\n", port)
	} else {
		fmt.Println("  Publishing:     disabled")
	}

	fmt.Printf("  Catalog Path:   %s\n", ac.CatalogPath)

	fmt.Println("\n  [Available Tools]")
	fmt.Println("  ard_search      — Semantic search remote resources")
	fmt.Println("  ard_discover    — Federated discovery across registries")
	fmt.Println("  ard_list        — List local catalog entries")
	fmt.Println("  ard_get         — Get resource details")
	fmt.Println("  ard_register    — Register new resource")
	fmt.Println("  ard_unregister  — Remove resource registration")
	fmt.Println("  ard_export      — Export catalog")

	fmt.Println()
	return nil
}

// ==========================================================================
// ard catalog
// ==========================================================================

func newARDCatalogCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:     "catalog",
		Aliases: []string{"ls", "list"},
		Short:   "List locally registered ARD resources",
		Long: `Display all resources registered in the local ARD
catalog, including agents, servers, and MCP servers.

Examples:
  wukong ard catalog
  wukong ard ls`,
		RunE: runARDCatalog,
	}

	cmd.Flags().StringVarP(
		&configPath, "config", "c", "",
		"Path to config file")

	return cmd
}

func runARDCatalog(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")

	loader, err := config.NewLoader(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	wukongCfg, err := loader.Load()
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	if !wukongCfg.ARD.Enabled {
		fmt.Println("ARD is disabled. Enable it in config.yaml:")
		fmt.Println("  ard:")
		fmt.Println("    enabled: true")
		return nil
	}

	catalogPath := wukongCfg.ARD.CatalogPath
	if catalogPath == "" {
		catalogPath = ".wukong/ard/catalog.json"
	}

	catalogPath = config.ResolvePath(catalogPath)

	// Create a catalog manager to load entries
	cm, err := ard.NewCatalogManager(catalogPath)
	if err != nil {
		return fmt.Errorf("open catalog %q: %w", catalogPath, err)
	}

	entries := cm.List()

	if len(entries) == 0 {
		fmt.Printf("No entries in ARD catalog (%s)\n", catalogPath)
		fmt.Println("\nResources are auto-registered when:")
		fmt.Println("  - MCP servers connect")
		fmt.Println("  - A2A remote agents are configured")
		return nil
	}

	fmt.Printf("ARD Catalog (%s) — %d entries:\n\n", catalogPath, len(entries))
	fmt.Printf("  %-30s %-15s %s\n", "NAME", "TYPE", "IDENTIFIER")
	fmt.Println("  " + strings.Repeat("-", 70))

	for _, e := range entries {
		entryType := string(e.Type)
		if len(entryType) > 13 {
			entryType = entryType[:10] + "..."
		}
		identifier := e.Identifier
		if len(identifier) > 30 {
			identifier = identifier[:27] + "..."
		}
		fmt.Printf("  %-30s %-15s %s\n",
			e.DisplayName, entryType, identifier)
	}

	fmt.Println()
	return nil
}
