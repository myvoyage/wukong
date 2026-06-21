---
name: recipe-p0-refactor
overview: 为 Wukong recipe 系统实现 P0 优化：①参数系统（Parameters + Prompt 模板渲染，让 recipe 可参数化复用）；②结构化输出（Response.JSONSchema，保证子 Agent 返回合规 JSON）。两者均集中在 internal/agent/recipe.go，向后兼容（无参数/无响应配置的 recipe 行为不变）。
todos:
  - id: implement-recipe-tool-and-modify-recipe
    content: 实现 recipe_tool.go (RecipeParameter/RecipeResponseConfig/recipeTool/校验/渲染) 并修改 recipe.go (RecipeConfig 新增字段/createRecipeAgent 结构化输出/NewRecipeToolSet 分发逻辑)
    status: completed
  - id: write-recipe-tests
    content: 编写 recipe_test.go 覆盖参数校验/模板渲染/Declaration 生成/向后兼容/结构化输出配置
    status: completed
    dependencies:
      - implement-recipe-tool-and-modify-recipe
  - id: update-docs-and-example
    content: 更新 recipe.go 包注释 YAML schema 文档，创建含 parameters+response 的示例 recipe YAML
    status: completed
    dependencies:
      - implement-recipe-tool-and-modify-recipe
---

## 用户需求

对 Wukong 系统的 Recipe 子系统进行 P0 级优化重构，补齐与 Goose recipes 对比中识别的核心能力差距。

## 产品概述

在现有 YAML recipe 子 Agent 系统基础上，新增两大核心能力：**参数化模板系统**（让 recipe 可被参数化复用，而非一次配置定型）和**结构化输出**（让子 Agent 返回合规 JSON，支持自动化对接）。改造完全向后兼容——无新字段的 recipe 行为 100% 不变。

## 核心功能

- **参数系统（P0-A）**：Recipe YAML 新增 `parameters` 和 `prompt` 字段，支持 string/number/boolean/select 四种参数类型，主 Agent 调用 recipe 工具时传入参数值，recipeTool 渲染 prompt 模板后传给子 Agent 执行
- **结构化输出（P0-B）**：Recipe YAML 新增 `response` 字段，配置 JSON Schema 后子 Agent 最终输出强制符合 schema，支持 strict 模式和描述
- **向后兼容**：无 `parameters`/`response` 字段的 recipe 继续走 `agenttool.NewTool` 原有路径，零行为变更

## Tech Stack

- 语言：Go 1.26（项目现有）
- Agent 框架：tRPC-Agent-Go v1.10.0（项目现有）
- 模板引擎：Go 标准库 `text/template`（`{{.ParamName}}` 语法，支持条件/循环）
- 序列化：`gopkg.in/yaml.v3`（YAML 配置）+ `encoding/json`（工具参数解析）
- 无新增外部依赖

## Implementation Approach

### 核心技术决策

**P0-A 参数系统**：

1. **模板语法选择**：采用 Go `text/template`（`{{.ParamName}}`），而非 Goose 的 Jinja。理由：标准库零依赖、支持条件/循环等高级功能、与项目 `prompt_template.go` 的 `{{.Var}}` 风格一致。

2. **双字段模型**（借鉴 Goose）：`instruction`（静态系统提示词，保持现有语义不变）+ `prompt`（参数化任务模板，渲染后作为子 Agent 用户消息）。

3. **recipeTool 包装器模式**：实现 `tool.CallableTool` 接口，内部持有 `*agenttool.Tool`。关键发现：`agenttool.Tool.Call`（agent_tool.go:321）执行 `message := model.NewUserMessage(string(jsonArgs))`——将原始 JSON 字节直接转为字符串作为用户消息，不解析 JSON。因此 recipeTool.Call 流程为：解析参数 JSON → 校验 → 渲染 prompt 模板 → 将渲染结果作为 `[]byte` 传给 `inner.Call(ctx, []byte(renderedPrompt))`，inner 把渲染后的 prompt 作为用户消息传给子 Agent。

4. **兼容策略**：无 `parameters` 字段的 recipe 继续用 `agenttool.NewTool`（行为 100% 不变）；有 `parameters` 的 recipe 用 `recipeTool` 包装。

5. **参数类型**：string/number/boolean/select（含 options），对齐 Goose 简化版。select 类型利用 `tool.Schema.Enum` 字段约束可选值。

**P0-B 结构化输出**：

直接利用 tRPC-Agent-Go 原生能力 `llmagent.WithStructuredOutputJSONSchema(name, schema, strict, description)`（option.go:1450）。在 `createRecipeAgent` 中，当 `recipe.Response` 非空时追加此 Option。

### 性能与可靠性

- **模板编译一次**：prompt 模板在 recipe 加载时编译为 `*template.Template`，每次 Call 仅执行渲染，无重复解析开销
- **Declaration 预构建**：recipeTool 的 `*tool.Declaration` 在构造时一次性构建，Call 时零分配
- **参数校验前置**：required/default/select-options 在 Call 入口校验，失败立即返回 error，不浪费 LLM 调用
- **向后兼容无性能影响**：无 parameters 的 recipe 走原有路径，零额外开销

## Architecture Design

### 数据流

```
主 Agent LLM 决定调用 recipe-code-reviewer 工具
  → 生成 JSON 参数: {"language":"go","focus":"security","code":"..."}
  → recipeTool.Call(ctx, jsonArgs)
    → 解析 JSON → 提取参数 → 填充默认值 → 校验 required/select
    → text/template 渲染 prompt 模板 → "Review the following go code focusing on security:\n\n..."
    → inner.Call(ctx, []byte(renderedPrompt))
      → agenttool.Tool.Call: message := NewUserMessage(renderedPrompt)
      → 子 Agent 执行 (带 StructuredOutput 约束)
      → 返回结构化 JSON 结果
  → recipeTool 返回结果给主 Agent
```

### 兼容性分支

```
NewRecipeToolSet
  ├── recipe.Parameters 为空 → agenttool.NewTool (现有路径, 零变更)
  └── recipe.Parameters 非空 → recipeTool 包装 agenttool.NewTool (新路径)
```

## Directory Structure

```
internal/agent/
├── recipe.go          [MODIFY] RecipeConfig 新增 Prompt/Parameters/Response 字段;
│                                createRecipeAgent 追加结构化输出 Option;
│                                NewRecipeToolSet 增加 recipeTool 分发逻辑;
│                                更新包注释 YAML schema 文档
├── recipe_tool.go     [NEW]    RecipeParameter/RecipeResponseConfig 类型定义;
│                                recipeTool 结构体 (实现 tool.CallableTool);
│                                newRecipeTool 构造函数;
│                                buildRecipeDeclaration 辅助函数;
│                                validateAndExtractParams 参数校验;
│                                renderPrompt 模板渲染;
│                                paramTypeToJSONType 类型映射
├── recipe_test.go     [NEW]    参数校验测试 (required/default/select);
│                                模板渲染测试 (正常/条件/缺失参数);
│                                向后兼容测试 (无 parameters 走原路径);
│                                Declaration 生成测试;
│                                结构化输出配置测试
```

### 文件详细说明

**recipe.go [MODIFY]**

- `RecipeConfig` 结构体新增 3 个字段：`Prompt string`、`Parameters []RecipeParameter`、`Response *RecipeResponseConfig`
- `createRecipeAgent` 函数：在 opts 构建后，当 `recipe.Response` 非空且 `JSONSchema` 非空时，追加 `llmagent.WithStructuredOutputJSONSchema(recipe.Name, schema, strict, desc)` Option
- `NewRecipeToolSet` 函数：在 agenttool.NewTool 创建后，当 `recipe.Parameters` 非空时，用 `newRecipeTool(agentTool, recipe)` 包装；否则直接使用 agentTool
- 包注释更新：补充 parameters/prompt/response 字段的 YAML schema 示例与说明

**recipe_tool.go [NEW]**

- `RecipeParameter`：Key/Description/Type/Required/Default/Options 六字段，YAML 标签
- `RecipeResponseConfig`：JSONSchema(map[string]any)/Strict(bool)/Description(string)，YAML 标签
- `recipeTool`：持有 `inner *agenttool.Tool` + `params []RecipeParameter` + `promptTmpl *template.Template` + `decl *tool.Declaration`
- `Declaration()`：返回预构建的 `decl`，InputSchema.Properties 包含各参数 + 可选 `request` 字段
- `Call(ctx, jsonArgs)`：解析 JSON → validateAndExtractParams → renderPrompt → `inner.Call(ctx, []byte(rendered))`
- `buildRecipeDeclaration`：遍历 params 构建 `tool.Schema`，select 类型设置 `Enum`，required 参数加入 `Required` 列表
- `validateAndExtractParams`：检查 required 参数存在性、select 值合法性、填充 default
- `renderPrompt`：`tmpl.Execute(&buf, paramMap)` 渲染模板
- `paramTypeToJSONType`：string→"string", number→"number", boolean→"boolean", select→"string"

**recipe_test.go [NEW]**

- `TestValidateParams_Required`：缺少必需参数返回 error
- `TestValidateParams_Default`：可选参数使用默认值
- `TestValidateParams_Select`：select 参数校验 options
- `TestRenderPrompt`：基本模板渲染 + 条件语法
- `TestBuildRecipeDeclaration`：Declaration 结构正确性
- `TestBackwardCompat`：无 Parameters 的 recipe 不创建 recipeTool

## Key Code Structures

```
// RecipeParameter defines a dynamic parameter for a recipe.
type RecipeParameter struct {
  Key         string   `yaml:"key"`
  Description string   `yaml:"description"`
  Type        string   `yaml:"type"`     // string|number|boolean|select
  Required    bool     `yaml:"required"`
  Default     string   `yaml:"default"`
  Options     []string `yaml:"options"`  // select only
}

// RecipeResponseConfig defines structured output for a recipe.
type RecipeResponseConfig struct {
  JSONSchema  map[string]any `yaml:"json_schema"`
  Strict      bool           `yaml:"strict"`
  Description string         `yaml:"description"`
}

// recipeTool wraps agenttool.Tool with parameter support.
// Implements tool.CallableTool.
type recipeTool struct {
  inner      *agenttool.Tool
  params     []RecipeParameter
  promptTmpl *template.Template
  decl       *tool.Declaration
}
```

## Agent Extensions

### SubAgent

- **code-explorer**
- Purpose: 实现阶段如需进一步探索 tRPC-Agent-Go 框架 API 细节（如 tool.Schema 字段行为、agenttool 选项组合），用 code-explorer 在模块缓存中搜索确认
- Expected outcome: 确认 API 签名与行为，避免编译错误