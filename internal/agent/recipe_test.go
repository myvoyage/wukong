package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"text/template"

	"gopkg.in/yaml.v3"

	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// stubInner is a test double for recipeInner. It records call
// arguments and returns a canned result.
type stubInner struct {
	decl         *tool.Declaration
	callResult   any
	callErr      error
	lastCallArgs []byte
}

func (s *stubInner) Declaration() *tool.Declaration {
	return s.decl
}

func (s *stubInner) Call(
	_ context.Context,
	jsonArgs []byte,
) (any, error) {
	s.lastCallArgs = jsonArgs
	if s.callErr != nil {
		return nil, s.callErr
	}
	return s.callResult, nil
}

// TestValidateParams_Required verifies that missing required
// parameters produce an error.
func TestValidateParams_Required(t *testing.T) {
	params := []RecipeParameter{
		{Key: "code", Type: "string", Required: true},
		{Key: "focus", Type: "string", Required: false},
	}
	args, _ := json.Marshal(map[string]string{"focus": "security"})

	_, err := validateAndExtractParams(params, args)
	if err == nil {
		t.Fatal("expected error for missing required parameter")
	}
	if !strings.Contains(err.Error(), "code") {
		t.Errorf("error should mention missing key, got: %v", err)
	}
}

// TestValidateParams_Default verifies optional parameters fall back
// to their configured default.
func TestValidateParams_Default(t *testing.T) {
	params := []RecipeParameter{
		{Key: "focus", Type: "string", Required: false, Default: "all"},
		{Key: "code", Type: "string", Required: true},
	}
	args, _ := json.Marshal(map[string]string{"code": "x = 1"})

	result, err := validateAndExtractParams(params, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["focus"] != "all" {
		t.Errorf("expected default 'all', got %q", result["focus"])
	}
	if result["code"] != "x = 1" {
		t.Errorf("expected 'x = 1', got %q", result["code"])
	}
}

// TestValidateParams_SelectValid accepts a valid select option.
func TestValidateParams_SelectValid(t *testing.T) {
	params := []RecipeParameter{
		{
			Key:      "language",
			Type:     paramTypeSelect,
			Required: true,
			Options:  []string{"go", "python", "rust"},
		},
	}
	args, _ := json.Marshal(map[string]string{"language": "go"})

	result, err := validateAndExtractParams(params, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["language"] != "go" {
		t.Errorf("expected 'go', got %q", result["language"])
	}
}

// TestValidateParams_SelectInvalid rejects a value outside options.
func TestValidateParams_SelectInvalid(t *testing.T) {
	params := []RecipeParameter{
		{
			Key:      "language",
			Type:     paramTypeSelect,
			Required: true,
			Options:  []string{"go", "python"},
		},
	}
	args, _ := json.Marshal(map[string]string{"language": "java"})

	_, err := validateAndExtractParams(params, args)
	if err == nil {
		t.Fatal("expected error for invalid select option")
	}
	if !strings.Contains(err.Error(), "must be one of") {
		t.Errorf("error should mention options, got: %v", err)
	}
}

// TestValidateParams_Boolean coerces JSON booleans to strings.
func TestValidateParams_Boolean(t *testing.T) {
	params := []RecipeParameter{
		{Key: "verbose", Type: paramTypeBoolean, Required: true},
	}
	args, _ := json.Marshal(map[string]bool{"verbose": true})

	result, err := validateAndExtractParams(params, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["verbose"] != "true" {
		t.Errorf("expected 'true', got %q", result["verbose"])
	}
}

// TestValidateParams_Number coerces JSON numbers to strings.
func TestValidateParams_Number(t *testing.T) {
	params := []RecipeParameter{
		{Key: "count", Type: paramTypeNumber, Required: true},
	}
	args, _ := json.Marshal(map[string]float64{"count": 42.0})

	result, err := validateAndExtractParams(params, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["count"] != "42" {
		t.Errorf("expected '42', got %q", result["count"])
	}
}

// TestValidateParams_NumberFractional preserves fractional numbers.
func TestValidateParams_NumberFractional(t *testing.T) {
	params := []RecipeParameter{
		{Key: "ratio", Type: paramTypeNumber, Required: true},
	}
	args, _ := json.Marshal(map[string]float64{"ratio": 0.75})

	result, err := validateAndExtractParams(params, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["ratio"] != "0.75" {
		t.Errorf("expected '0.75', got %q", result["ratio"])
	}
}

// TestValidateParams_TypeMismatch rejects a string where a number
// is expected.
func TestValidateParams_TypeMismatch(t *testing.T) {
	params := []RecipeParameter{
		{Key: "count", Type: paramTypeNumber, Required: true},
	}
	args, _ := json.Marshal(map[string]string{"count": "not-a-number"})

	_, err := validateAndExtractParams(params, args)
	if err == nil {
		t.Fatal("expected type mismatch error")
	}
	if !strings.Contains(err.Error(), "expects number") {
		t.Errorf("error should mention type, got: %v", err)
	}
}

// TestValidateParams_EmptyArgs handles empty JSON gracefully.
func TestValidateParams_EmptyArgs(t *testing.T) {
	params := []RecipeParameter{
		{Key: "x", Type: "string", Required: false, Default: "def"},
	}

	result, err := validateAndExtractParams(params, []byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["x"] != "def" {
		t.Errorf("expected default 'def', got %q", result["x"])
	}
}

// TestValidateParams_InvalidJSON rejects malformed JSON.
func TestValidateParams_InvalidJSON(t *testing.T) {
	params := []RecipeParameter{
		{Key: "x", Type: "string", Required: false},
	}

	_, err := validateAndExtractParams(params, []byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse recipe arguments") {
		t.Errorf("error should mention parse failure, got: %v", err)
	}
}

// TestRenderPrompt_Basic verifies simple variable substitution.
func TestRenderPrompt_Basic(t *testing.T) {
	tmpl := template.Must(
		template.New("t").Parse("Review {{.language}} code: {{.code}}"))

	result, err := renderPrompt(tmpl, map[string]string{
		"language": "go",
		"code":     "x := 1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "Review go code: x := 1"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestRenderPrompt_Conditional verifies Go template conditionals
// work in the prompt. Note: Go templates treat non-empty strings as
// truthy, so callers use empty string to indicate "false".
func TestRenderPrompt_Conditional(t *testing.T) {
	tmpl := template.Must(template.New("t").Parse(
		`{{if .verbose}}Verbose mode on. {{end}}Task: {{.task}}`))

	// Non-empty verbose -> condition true.
	result, err := renderPrompt(tmpl, map[string]string{
		"verbose": "true",
		"task":    "review",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Verbose mode on") {
		t.Errorf("expected verbose prefix, got %q", result)
	}

	// Empty verbose -> condition false.
	result, err = renderPrompt(tmpl, map[string]string{
		"verbose": "",
		"task":    "review",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result, "Verbose mode on") {
		t.Errorf("expected no verbose prefix, got %q", result)
	}
}

// TestRenderPrompt_EmptyValue renders empty string for empty values.
func TestRenderPrompt_EmptyValue(t *testing.T) {
	tmpl := template.Must(
		template.New("t").Parse("Hello {{.name}}!"))

	result, err := renderPrompt(tmpl, map[string]string{"name": ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Hello !") {
		t.Errorf("expected greeting with empty name, got %q", result)
	}
}

// TestBuildRecipeDeclaration verifies the Declaration structure
// generated for a parameterized recipe.
func TestBuildRecipeDeclaration(t *testing.T) {
	recipe := &RecipeConfig{
		Name:        "reviewer",
		Description: "Code reviewer",
		Parameters: []RecipeParameter{
			{
				Key:         "language",
				Description: "Programming language",
				Type:        paramTypeSelect,
				Required:    true,
				Options:     []string{"go", "python"},
			},
			{
				Key:         "focus",
				Description: "Review focus",
				Type:        "string",
				Required:    false,
				Default:     "all",
			},
		},
	}
	inner := &stubInner{decl: &tool.Declaration{
		Name:        "recipe-reviewer",
		Description: "Code reviewer",
	}}

	result := buildRecipeDeclaration(recipe, inner)

	if result.Name != "recipe-reviewer" {
		t.Errorf("expected name 'recipe-reviewer', got %q",
			result.Name)
	}
	if result.Description != "Code reviewer" {
		t.Errorf("expected description 'Code reviewer', got %q",
			result.Description)
	}
	if result.InputSchema == nil {
		t.Fatal("expected non-nil InputSchema")
	}
	if result.InputSchema.Type != "object" {
		t.Errorf("expected object type, got %q",
			result.InputSchema.Type)
	}
	langSchema, ok := result.InputSchema.Properties["language"]
	if !ok {
		t.Fatal("expected 'language' property")
	}
	if langSchema.Type != "string" {
		t.Errorf("expected string type for select, got %q",
			langSchema.Type)
	}
	if len(langSchema.Enum) != 2 {
		t.Errorf("expected 2 enum values, got %d",
			len(langSchema.Enum))
	}
	if langSchema.Enum[0] != "go" {
		t.Errorf("expected first enum 'go', got %v",
			langSchema.Enum[0])
	}
	focusSchema, ok := result.InputSchema.Properties["focus"]
	if !ok {
		t.Fatal("expected 'focus' property")
	}
	if focusSchema.Default != "all" {
		t.Errorf("expected default 'all', got %v",
			focusSchema.Default)
	}
	if len(result.InputSchema.Required) != 1 ||
		result.InputSchema.Required[0] != "language" {
		t.Errorf("expected required=[language], got %v",
			result.InputSchema.Required)
	}
}

// TestBuildRecipeDeclaration_EmptyParams handles a recipe with no
// parameters (defensive case).
func TestBuildRecipeDeclaration_EmptyParams(t *testing.T) {
	recipe := &RecipeConfig{
		Name:        "simple",
		Description: "Simple recipe",
	}
	inner := &stubInner{decl: &tool.Declaration{
		Name:        "recipe-simple",
		Description: "Simple recipe",
	}}

	result := buildRecipeDeclaration(recipe, inner)

	if result.InputSchema == nil {
		t.Fatal("expected non-nil InputSchema")
	}
	if len(result.InputSchema.Properties) != 0 {
		t.Errorf("expected 0 properties, got %d",
			len(result.InputSchema.Properties))
	}
	if len(result.InputSchema.Required) != 0 {
		t.Errorf("expected 0 required, got %d",
			len(result.InputSchema.Required))
	}
}

// TestBuildRecipeDeclaration_PreservesInnerNameDesc verifies that
// the name and description come from the inner tool's Declaration,
// not the recipe (the inner tool already prefixed with recipe-).
func TestBuildRecipeDeclaration_PreservesInnerNameDesc(t *testing.T) {
	recipe := &RecipeConfig{
		Name:        "ignored-name",
		Description: "ignored-desc",
		Parameters: []RecipeParameter{
			{Key: "x", Type: "string", Required: true},
		},
	}
	inner := &stubInner{decl: &tool.Declaration{
		Name:        "recipe-real",
		Description: "Real description",
	}}

	result := buildRecipeDeclaration(recipe, inner)

	if result.Name != "recipe-real" {
		t.Errorf("expected inner name, got %q", result.Name)
	}
	if result.Description != "Real description" {
		t.Errorf("expected inner description, got %q",
			result.Description)
	}
}

// TestParamTypeToJSONType covers all type mappings.
func TestParamTypeToJSONType(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{paramTypeNumber, "number"},
		{paramTypeBoolean, "boolean"},
		{paramTypeSelect, "string"},
		{"string", "string"},
		{"", "string"}, // unknown defaults to string
	}
	for _, c := range cases {
		got := paramTypeToJSONType(c.input)
		if got != c.want {
			t.Errorf("paramTypeToJSONType(%q) = %q, want %q",
				c.input, got, c.want)
		}
	}
}

// TestNewRecipeTool_InvalidTemplate returns an error for malformed
// prompt templates.
func TestNewRecipeTool_InvalidTemplate(t *testing.T) {
	recipe := &RecipeConfig{
		Name:        "bad",
		Description: "Bad template",
		Prompt:      "Hello {{ .name ", // unclosed action
		Parameters: []RecipeParameter{
			{Key: "name", Type: "string", Required: true},
		},
	}
	inner := &stubInner{decl: &tool.Declaration{Name: "recipe-bad"}}
	_, err := newRecipeToolWithInner(inner, recipe)
	if err == nil {
		t.Fatal("expected error for invalid template")
	}
	if !strings.Contains(err.Error(), "parse prompt template") {
		t.Errorf("error should mention template parse, got: %v", err)
	}
}

// TestNewRecipeTool_Valid builds a recipeTool successfully.
func TestNewRecipeTool_Valid(t *testing.T) {
	recipe := &RecipeConfig{
		Name:        "good",
		Description: "Good template",
		Prompt:      "Review {{.lang}} code",
		Parameters: []RecipeParameter{
			{Key: "lang", Type: "string", Required: true},
		},
	}
	inner := &stubInner{decl: &tool.Declaration{
		Name:        "recipe-good",
		Description: "Good template",
	}}
	rt, err := newRecipeToolWithInner(inner, recipe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt == nil {
		t.Fatal("expected non-nil recipeTool")
	}
	d := rt.Declaration()
	if d.Name != "recipe-good" {
		t.Errorf("expected name 'recipe-good', got %q", d.Name)
	}
}

// TestRecipeTool_DeclarationHasParams verifies the exposed
// Declaration reflects parameters, not the inner tool's schema.
func TestRecipeTool_DeclarationHasParams(t *testing.T) {
	recipe := &RecipeConfig{
		Name:        "reviewer",
		Description: "Reviewer",
		Prompt:      "{{.code}}",
		Parameters: []RecipeParameter{
			{Key: "code", Type: "string", Required: true},
		},
	}
	inner := &stubInner{decl: &tool.Declaration{
		Name:        "recipe-reviewer",
		Description: "Reviewer",
		InputSchema: &tool.Schema{Type: "string"}, // inner has no params
	}}
	rt, err := newRecipeToolWithInner(inner, recipe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	d := rt.Declaration()
	if _, ok := d.InputSchema.Properties["code"]; !ok {
		t.Error("expected 'code' property in Declaration")
	}
}

// TestRecipeTool_CallValidatesParams verifies Call rejects missing
// required parameters without invoking the inner tool.
func TestRecipeTool_CallValidatesParams(t *testing.T) {
	recipe := &RecipeConfig{
		Name:        "reviewer",
		Description: "Reviewer",
		Prompt:      "{{.code}}",
		Parameters: []RecipeParameter{
			{Key: "code", Type: "string", Required: true},
		},
	}
	inner := &stubInner{decl: &tool.Declaration{
		Name:        "recipe-reviewer",
		Description: "Reviewer",
	}}
	rt, err := newRecipeToolWithInner(inner, recipe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Missing required 'code' parameter.
	_, err = rt.Call(context.Background(), []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for missing required param")
	}
	if !strings.Contains(err.Error(), "missing required parameter") {
		t.Errorf("unexpected error: %v", err)
	}
	// Inner should NOT have been called.
	if inner.lastCallArgs != nil {
		t.Error("expected inner.Call to not be invoked on validation failure")
	}
}

// TestRecipeTool_CallForwardsRenderedPrompt verifies Call renders
// the prompt and forwards it to the inner tool.
func TestRecipeTool_CallForwardsRenderedPrompt(t *testing.T) {
	recipe := &RecipeConfig{
		Name:        "reviewer",
		Description: "Reviewer",
		Prompt:      "Review {{.lang}} code: {{.code}}",
		Parameters: []RecipeParameter{
			{Key: "lang", Type: "string", Required: true},
			{Key: "code", Type: "string", Required: true},
		},
	}
	inner := &stubInner{
		decl: &tool.Declaration{
			Name:        "recipe-reviewer",
			Description: "Reviewer",
		},
		callResult: "review done",
	}
	rt, err := newRecipeToolWithInner(inner, recipe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	args, _ := json.Marshal(map[string]string{
		"lang": "go",
		"code": "x := 1",
	})
	result, err := rt.Call(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "review done" {
		t.Errorf("expected 'review done', got %v", result)
	}
	if inner.lastCallArgs == nil {
		t.Fatal("expected inner.Call to be invoked")
	}
	rendered := string(inner.lastCallArgs)
	if !strings.Contains(rendered, "Review go code: x := 1") {
		t.Errorf("expected rendered prompt forwarded, got %q",
			rendered)
	}
}

// TestRecipeTool_CallUsesDefault verifies defaults are applied
// during Call.
func TestRecipeTool_CallUsesDefault(t *testing.T) {
	recipe := &RecipeConfig{
		Name:        "reviewer",
		Description: "Reviewer",
		Prompt:      "Focus: {{.focus}}",
		Parameters: []RecipeParameter{
			{
				Key:      "focus",
				Type:     "string",
				Required: false,
				Default:  "security",
			},
		},
	}
	inner := &stubInner{
		decl: &tool.Declaration{
			Name:        "recipe-reviewer",
			Description: "Reviewer",
		},
		callResult: "ok",
	}
	rt, err := newRecipeToolWithInner(inner, recipe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No 'focus' supplied; default should be used.
	_, err = rt.Call(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(inner.lastCallArgs), "Focus: security") {
		t.Errorf("expected default applied, got %q",
			string(inner.lastCallArgs))
	}
}

// TestRecipeTool_CallPropagatesInnerError verifies errors from the
// inner tool are returned to the caller.
func TestRecipeTool_CallPropagatesInnerError(t *testing.T) {
	recipe := &RecipeConfig{
		Name:        "reviewer",
		Description: "Reviewer",
		Prompt:      "{{.code}}",
		Parameters: []RecipeParameter{
			{Key: "code", Type: "string", Required: true},
		},
	}
	inner := &stubInner{
		decl: &tool.Declaration{
			Name:        "recipe-reviewer",
			Description: "Reviewer",
		},
		callErr: errors.New("inner boom"),
	}
	rt, err := newRecipeToolWithInner(inner, recipe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	args, _ := json.Marshal(map[string]string{"code": "x"})
	_, err = rt.Call(context.Background(), args)
	if err == nil {
		t.Fatal("expected inner error to propagate")
	}
	if !strings.Contains(err.Error(), "inner boom") {
		t.Errorf("expected inner error message, got: %v", err)
	}
}

// TestRecipeConfig_YAMLParsing verifies the new fields parse from
// YAML correctly.
func TestRecipeConfig_YAMLParsing(t *testing.T) {
	yamlContent := `
name: reviewer
description: "Parameterized reviewer"
instruction: "You are a reviewer."
prompt: "Review {{.lang}} code"
parameters:
  - key: lang
    description: "Language"
    type: select
    required: true
    options: [go, python]
response:
  json_schema:
    type: object
    properties:
      issues: {type: array}
    required: [issues]
  strict: true
  description: "Review output"
tools: [file_read]
temperature: 0.2
`
	var recipe RecipeConfig
	if err := yaml.Unmarshal([]byte(yamlContent), &recipe); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recipe.Name != "reviewer" {
		t.Errorf("expected name 'reviewer', got %q", recipe.Name)
	}
	if recipe.Prompt != "Review {{.lang}} code" {
		t.Errorf("unexpected prompt: %q", recipe.Prompt)
	}
	if len(recipe.Parameters) != 1 {
		t.Fatalf("expected 1 parameter, got %d",
			len(recipe.Parameters))
	}
	p := recipe.Parameters[0]
	if p.Key != "lang" || p.Type != paramTypeSelect || !p.Required {
		t.Errorf("unexpected parameter: %+v", p)
	}
	if len(p.Options) != 2 || p.Options[0] != "go" {
		t.Errorf("unexpected options: %v", p.Options)
	}
	if recipe.Response == nil {
		t.Fatal("expected non-nil Response")
	}
	if !recipe.Response.Strict {
		t.Error("expected strict=true")
	}
	if recipe.Response.Description != "Review output" {
		t.Errorf("unexpected description: %q",
			recipe.Response.Description)
	}
	if _, ok := recipe.Response.JSONSchema["type"]; !ok {
		t.Error("expected 'type' in json_schema")
	}
}

// TestRecipeConfig_BackwardCompat verifies recipes without the new
// fields still parse and behave as before.
func TestRecipeConfig_BackwardCompat(t *testing.T) {
	yamlContent := `
name: simple
description: "Simple recipe"
instruction: "You are a helper."
tools: [file_read]
`
	var recipe RecipeConfig
	if err := yaml.Unmarshal([]byte(yamlContent), &recipe); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recipe.Prompt != "" {
		t.Errorf("expected empty prompt, got %q", recipe.Prompt)
	}
	if len(recipe.Parameters) != 0 {
		t.Errorf("expected 0 parameters, got %d",
			len(recipe.Parameters))
	}
	if recipe.Response != nil {
		t.Error("expected nil Response")
	}
}

// TestStructuredOutput_ConfigPresent verifies the response config
// is well-formed when set.
func TestStructuredOutput_ConfigPresent(t *testing.T) {
	recipe := &RecipeConfig{
		Name: "reviewer",
		Response: &RecipeResponseConfig{
			JSONSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"summary": map[string]any{"type": "string"},
				},
				"required": []string{"summary"},
			},
			Strict:      true,
			Description: "Review summary",
		},
	}
	if recipe.Response == nil {
		t.Fatal("expected non-nil Response")
	}
	if len(recipe.Response.JSONSchema) == 0 {
		t.Fatal("expected non-empty JSONSchema")
	}
	if !recipe.Response.Strict {
		t.Error("expected strict=true")
	}
}

// TestStructuredOutput_ConfigAbsent verifies nil Response is safe.
func TestStructuredOutput_ConfigAbsent(t *testing.T) {
	recipe := &RecipeConfig{
		Name: "simple",
	}
	if recipe.Response != nil {
		t.Error("expected nil Response for simple recipe")
	}
}

// TestStubInner_CallError verifies the test stub returns errors
// when configured.
func TestStubInner_CallError(t *testing.T) {
	inner := &stubInner{
		decl:    &tool.Declaration{Name: "x"},
		callErr: errors.New("boom"),
	}
	_, err := inner.Call(context.Background(), []byte("{}"))
	if err == nil || err.Error() != "boom" {
		t.Errorf("expected 'boom' error, got %v", err)
	}
}
