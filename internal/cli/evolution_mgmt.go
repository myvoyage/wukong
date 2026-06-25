// Package cli provides the "wukong evolution" command for
// evolution engine management.
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/km269/wukong/internal/config"
)

// newEvolutionCmd creates the "wukong evolution" command group.
func newEvolutionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "evolution",
		Short: "Manage the skill evolution engine",
		Long: `View status and configuration of the skill self-evolution
engine that analyzes execution traces and patches skills.

Subcommands:
  status   Show evolution engine configuration and status`,
	}

	cmd.AddCommand(newEvolutionStatusCmd())

	return cmd
}

func newEvolutionStatusCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show evolution engine status",
		Long: `Display the current evolution engine configuration and
operational parameters.

Examples:
  wukong evolution status`,
		RunE: runEvolutionStatus,
	}

	cmd.Flags().StringVarP(
		&configPath, "config", "c", "",
		"Path to config file")

	return cmd
}

func runEvolutionStatus(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")

	loader, err := config.NewLoader(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	wukongCfg, err := loader.Load()
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	ec := &wukongCfg.Evolution

	fmt.Println(strings.Repeat("─", 55))
	fmt.Println("  Evolution Engine Status")
	fmt.Println(strings.Repeat("─", 55))

	if !ec.Enabled {
		fmt.Println("\n  Status: disabled (experimental)")
		fmt.Println("\n  Enable with:")
		fmt.Println("    evolution:")
		fmt.Println("      enabled: true")
		fmt.Println("      analysis_provider: lmstudio")
		fmt.Println("      min_confidence: 0.7")
		return nil
	}

	fmt.Println("\n  Status: enabled")

	fmt.Println("\n  [Analysis]")
	fmt.Printf("  Auto Patch:        %v\n", ec.AutoPatch)
	fmt.Printf("  Min Confidence:    %.1f\n", ec.MinConfidence)
	fmt.Printf("  Analysis Timeout:  %s\n", ec.AnalysisTimeout)

	if ec.AnalysisProvider != "" {
		fmt.Printf("  Analysis Provider: %s\n", ec.AnalysisProvider)
	}
	if ec.AnalysisModel != "" {
		fmt.Printf("  Analysis Model:    %s\n", ec.AnalysisModel)
	}

	fmt.Println("\n  [Rate Limiting]")
	fmt.Printf("  Cooldown Period:   %s\n", ec.CooldownPeriod)
	fmt.Printf("  Max Patches/Day:    %d\n", ec.MaxPatchesPerDay)

	fmt.Println("\n  [Version Control]")
	fmt.Printf("  Max Versions Kept: %d\n", ec.MaxVersionsKept)
	fmt.Printf("  Max Patch Size:    %d chars\n", ec.MaxPatchSize)

	fmt.Println("\n  [Problem Types Detected]")
	fmt.Println("  missing_prerequisite     — Skill lacks a necessary step")
	fmt.Println("  outdated_instruction     — References deprecated APIs")
	fmt.Println("  parameter_error          — Default parameters are wrong")
	fmt.Println("  ambiguous_wording        — Unclear instructions")
	fmt.Println("  missing_error_handling   — No failure handling guidance")

	fmt.Println()
	return nil
}
