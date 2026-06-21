// Package agent provides the recipeTool wrapper that adds parameter
// templating support to recipe sub-agents.
//
// When a recipe defines a non-empty Parameters list, NewRecipeToolSet
// wraps the underlying agenttool.Tool with a recipeTool. The recipeTool
// exposes a JSON-schema declaration to the main agent so it can pass
// named parameters; on each Call it validates the parameters, fills
// defaults, renders the prompt template, and forwards the rendered
// text to the inner agenttool.Tool as the sub-agent's user message.
//
// Recipes without Parameters bypass recipeTool entirely and use
// agenttool.NewTool directly, preserving 100% backward compatibility.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"trpc.group/trpc-go/trpc-agent-go/tool"
	agenttool "trpc.group/trpc-go/trpc-agent-go/tool/agent"
)

// RecipeParameter defines a dynamic parameter for a parameterized recipe.
//
// Parameters are rendered into the recipe's Prompt template via Go
// text/template syntax ({{.KeyName}}). Supported types: string, number,
// boolean, select.
//
// Note on conditionals: Go templates treat non-empty strings as truthy.
// Since all parameters are rendered as strings, a boolean parameter
// with value "false" is non-empty and thus truthy in {{if .flag}}.
// To get falsy behavior, omit the parameter (it renders as "") or
// use {{if eq .flag "true"}} for explicit comparison.
type RecipeParameter struct {
	// Key is the parameter identifier used in the prompt template
	// (e.g. {{.language}}) and the JSON field name the main agent
	// must supply.
	Key string `yaml:"key"`
	// Description explains the parameter to the main agent.
	Description string `yaml:"description"`
	// Type is one of: string, number, boolean, select.
	// Default: string.
	Type string `yaml:"type"`
	// Required controls whether the main agent must supply this
	// parameter. Required parameters cannot have a Default.
	Required bool `yaml:"required"`
	// Default is used when the parameter is omitted. Only valid
	// when Required is false.
	Default string `yaml:"default"`
	// Options constrains allowed values when Type is select.
	// Ignored for other types.
	Options []string `yaml:"options"`
}

// RecipeResponseConfig defines structured output for a recipe sub-agent.
//
// When set, the sub-agent's final output is constrained to conform to
// the provided JSON Schema via the model-native response_format
// mechanism (when supported by the provider).
type RecipeResponseConfig struct {
	// JSONSchema is the JSON Schema object the output must satisfy.
	JSONSchema map[string]any `yaml:"json_schema"`
	// Strict enables strict mode (no extra fields) for providers
	// that support it.
	Strict bool `yaml:"strict"`
	// Description is an optional human-readable description of the
	// schema, used by some providers.
	Description string `yaml:"description"`
	// ValidateOutput enables post-execution validation of the
	// sub-agent's output against the JSON Schema. When true, the
	// output is checked for valid JSON and presence of required
	// top-level fields. If validation fails and a retry config is
	// set, the recipe is retried. This is a lightweight safety net
	// — the model-native structured output mechanism handles
	// strict enforcement at the provider level.
	ValidateOutput bool `yaml:"validate_output"`
}

// Supported parameter type constants.
const (
	paramTypeNumber  = "number"
	paramTypeBoolean = "boolean"
	paramTypeSelect  = "select"
)

// recipeInner is the minimal interface recipeTool requires from the
// underlying agent tool. *agenttool.Tool satisfies this interface;
// the indirection enables unit testing with stubs.
type recipeInner interface {
	Declaration() *tool.Declaration
	Call(ctx context.Context, jsonArgs []byte) (any, error)
}

// recipeTool wraps an agent tool with parameter support.
//
// It implements tool.CallableTool. The inner tool retains all its
// original call semantics (history scope, response mode, streaming);
// recipeTool only customizes the Declaration (to expose parameters to
// the main agent) and preprocesses Call arguments (validate, fill
// defaults, render prompt template).
type recipeTool struct {
	inner      recipeInner
	params     []RecipeParameter
	promptTmpl *template.Template
	decl       *tool.Declaration
}

// newRecipeTool builds a recipeTool from a recipe configuration and
// an already-constructed agenttool.Tool.
//
// Returns an error if the prompt template fails to parse.
func newRecipeTool(
	inner *agenttool.Tool,
	recipe *RecipeConfig,
) (*recipeTool, error) {
	return newRecipeToolWithInner(inner, recipe)
}

// newRecipeToolWithInner is the testable constructor that accepts
// any recipeInner implementation.
func newRecipeToolWithInner(
	inner recipeInner,
	recipe *RecipeConfig,
) (*recipeTool, error) {
	tmpl, err := template.New("recipe-" + recipe.Name).
		Parse(recipe.Prompt)
	if err != nil {
		return nil, fmt.Errorf(
			"parse prompt template: %w", err)
	}

	rt := &recipeTool{
		inner:      inner,
		params:     recipe.Parameters,
		promptTmpl: tmpl,
		decl:       buildRecipeDeclaration(recipe, inner),
	}
	return rt, nil
}

// Declaration returns the tool metadata exposed to the main agent.
//
// The InputSchema describes each parameter as a JSON Schema property,
// allowing the main agent's LLM to generate well-formed arguments.
func (rt *recipeTool) Declaration() *tool.Declaration {
	return rt.decl
}

// Call validates parameters, renders the prompt template, and
// forwards the rendered text to the inner agenttool.Tool.
//
// The main agent supplies a JSON object whose keys match parameter
// Keys. Missing optional parameters are filled with their Default.
// After rendering, the text is passed as []byte to inner.Call,
// which the agenttool treats as the sub-agent's user message.
func (rt *recipeTool) Call(
	ctx context.Context,
	jsonArgs []byte,
) (any, error) {
	paramMap, err := validateAndExtractParams(rt.params, jsonArgs)
	if err != nil {
		return nil, err
	}

	rendered, err := renderPrompt(rt.promptTmpl, paramMap)
	if err != nil {
		return nil, err
	}

	return rt.inner.Call(ctx, []byte(rendered))
}

// buildRecipeDeclaration constructs the tool.Declaration for a
// parameterized recipe.
//
// The InputSchema is an object whose Properties map each parameter
// Key to its JSON Schema. Required parameters are listed in Required.
// A select parameter's allowed values are exposed via Enum.
func buildRecipeDeclaration(
	recipe *RecipeConfig,
	inner recipeInner,
) *tool.Declaration {
	innerDecl := inner.Declaration()

	props := make(map[string]*tool.Schema, len(recipe.Parameters))
	var required []string

	for _, p := range recipe.Parameters {
		schema := &tool.Schema{
			Type:        paramTypeToJSONType(p.Type),
			Description: p.Description,
		}
		if p.Type == paramTypeSelect && len(p.Options) > 0 {
			enum := make([]any, len(p.Options))
			for i, o := range p.Options {
				enum[i] = o
			}
			schema.Enum = enum
		}
		if !p.Required && p.Default != "" {
			schema.Default = p.Default
		}
		props[p.Key] = schema
		if p.Required {
			required = append(required, p.Key)
		}
	}

	return &tool.Declaration{
		Name:        innerDecl.Name,
		Description: innerDecl.Description,
		InputSchema: &tool.Schema{
			Type:       "object",
			Properties: props,
			Required:   required,
		},
	}
}

// validateAndExtractParams parses the JSON arguments from the main
// agent, validates required parameters, enforces select options, and
// fills defaults for omitted optional parameters.
//
// Returns a map[string]string suitable for text/template rendering.
func validateAndExtractParams(
	params []RecipeParameter,
	jsonArgs []byte,
) (map[string]string, error) {
	raw := make(map[string]any)
	if len(jsonArgs) > 0 {
		if err := json.Unmarshal(jsonArgs, &raw); err != nil {
			return nil, fmt.Errorf(
				"parse recipe arguments: %w", err)
		}
	}

	result := make(map[string]string, len(params))

	for _, p := range params {
		val, present := raw[p.Key]

		if !present {
			if p.Required {
				return nil, fmt.Errorf(
					"missing required parameter: %s", p.Key)
			}
			if p.Default != "" {
				result[p.Key] = p.Default
				continue
			}
			// Optional with no default: empty string.
			result[p.Key] = ""
			continue
		}

		strVal, err := coerceParamValue(p, val)
		if err != nil {
			return nil, err
		}
		result[p.Key] = strVal
	}

	return result, nil
}

// coerceParamValue converts a parsed JSON value to its string
// representation and validates select options.
func coerceParamValue(
	p RecipeParameter,
	val any,
) (string, error) {
	var str string
	switch p.Type {
	case paramTypeBoolean:
		b, ok := val.(bool)
		if !ok {
			return "", fmt.Errorf(
				"parameter %s expects boolean, got %T",
				p.Key, val)
		}
		if b {
			str = "true"
		} else {
			str = "false"
		}
	case paramTypeNumber:
		switch n := val.(type) {
		case float64:
			str = formatNumber(n)
		case int:
			str = fmt.Sprintf("%d", n)
		case int64:
			str = fmt.Sprintf("%d", n)
		default:
			return "", fmt.Errorf(
				"parameter %s expects number, got %T",
				p.Key, val)
		}
	default:
		// string and select both expect a string.
		s, ok := val.(string)
		if !ok {
			return "", fmt.Errorf(
				"parameter %s expects string, got %T",
				p.Key, val)
		}
		str = s
	}

	if p.Type == paramTypeSelect && len(p.Options) > 0 {
		valid := false
		for _, opt := range p.Options {
			if str == opt {
				valid = true
				break
			}
		}
		if !valid {
			return "", fmt.Errorf(
				"parameter %s must be one of [%s], got %q",
				p.Key, strings.Join(p.Options, ", "), str)
		}
	}

	return str, nil
}

// formatNumber renders a float64 as a string, trimming trailing
// zeros for integer-valued numbers (e.g. 5.0 -> "5").
func formatNumber(n float64) string {
	if n == float64(int64(n)) {
		return fmt.Sprintf("%d", int64(n))
	}
	return fmt.Sprintf("%g", n)
}

// renderPrompt executes the prompt template with the parameter map.
func renderPrompt(
	tmpl *template.Template,
	params map[string]string,
) (string, error) {
	var buf strings.Builder
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("render prompt template: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// paramTypeToJSONType maps a recipe parameter type to its JSON
// Schema type string.
func paramTypeToJSONType(t string) string {
	switch t {
	case paramTypeNumber:
		return "number"
	case paramTypeBoolean:
		return "boolean"
	case paramTypeSelect:
		return "string"
	default:
		return "string"
	}
}
