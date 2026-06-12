// Package observability provides enhanced observability integrations
// including Langfuse LLM tracing via OpenTelemetry OTLP.
//
// Usage:
//
//	// Start Langfuse tracing
//	cleanup, err := observability.StartLangfuse(ctx, cfg)
//	defer cleanup(ctx)
//
// This module is complementary to the existing telemetry package and
// provides higher-level integrations specifically for LLM applications.
package observability

import (
	"context"
	"fmt"
	"os"

	"github.com/km269/wukong/internal/config"
	"trpc.group/trpc-go/trpc-agent-go/telemetry/langfuse"
)

// StartLangfuse initializes Langfuse tracing via OpenTelemetry OTLP.
// Returns a cleanup function that should be deferred in main.
//
// Credentials are resolved in order:
//  1. Config file values
//  2. Environment variables (LANGFUSE_PUBLIC_KEY, LANGFUSE_SECRET_KEY,
//     LANGFUSE_HOST, LANGFUSE_INSECURE)
//
// When cfg.LangfuseEnabled is false, returns nil cleanup with no error.
func StartLangfuse(
	ctx context.Context,
	cfg *config.ObservabilityConfig,
) (cleanup func(context.Context) error, err error) {
	if !cfg.LangfuseEnabled {
		// Not enabled, return no-op.
		return func(_ context.Context) error { return nil }, nil
	}

	// Set environment variables from config if not already set.
	setIfEmpty("LANGFUSE_PUBLIC_KEY", cfg.LangfusePublicKey)
	setIfEmpty("LANGFUSE_SECRET_KEY", cfg.LangfuseSecretKey)
	setIfEmpty("LANGFUSE_HOST", cfg.LangfuseHost)

	// Langfuse uses OTLP HTTP by default. Set INSECURE for local dev.
	if os.Getenv("LANGFUSE_INSECURE") == "" &&
		os.Getenv("LANGFUSE_HOST") == "" {
		// For local Langfuse deployments, set insecure=true.
		_ = os.Setenv("LANGFUSE_INSECURE", "true")
	}

	cleanup, err = langfuse.Start(ctx)
	if err != nil {
		return nil, fmt.Errorf("start langfuse: %w", err)
	}

	return cleanup, nil
}

// setIfEmpty sets an environment variable only if it's not already set
// and the value is non-empty.
func setIfEmpty(key, value string) {
	if value != "" && os.Getenv(key) == "" {
		_ = os.Setenv(key, value)
	}
}
