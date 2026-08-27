# Specification: OKF MCP Server

## 1. Overview

OKF MCP Server 是 Model Context Protocol (MCP) 的 Server 端实现，为 AI Agents 提供标准化的 okf 知识库访问接口。Server 暴露三类核心原语：

- **Tools**：可执行函数，用于查询、创建、更新、验证 okf 概念和知识库
- **Resources**：只读数据，通过 URI 寻址，提供 bundle、concept、index、log 资源
- **Prompts**：参数化模板，提供概念解释、知识库总结、变更审查等预设工作流

## 2. Server Capabilities

```json
{
  "capabilities": {
    "tools": { "listChanged": true },
    "resources": { "subscribe": true, "listChanged": true },
    "prompts": { "listChanged": true },
    "completion": {}
  },
  "serverInfo": {
    "name": "okf-mcp-server",
    "version": "0.1.0"
  }
}
```

## 3. Tools Specification

工具按功能域分组为 7 个工具集(toolsets)，可通过配置独立开关。

### 3.1 Bundle Management (bundle)

知识库 bundle 的加载、保存、统计。

#### 3.1.1 okf_load_bundle

加载知识库 bundle 到内存。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| path | string | yes | 知识库根目录路径 |
| recursive | boolean | no | 是否递归加载子目录，默认 true |
| skip_index | boolean | no | 是否跳过 index.md 解析，默认 false |

**Returns:**
- bundle_info: 包含路径、概念数量、统计信息
- errors: 加载过程中的错误列表

**Example:**
```json
{
  "path": "/path/to/knowledge-base",
  "recursive": true
}
```

#### 3.1.2 okf_save_bundle

保存内存中的 bundle 到磁盘。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| path | string | yes | 保存目标路径 |
| pretty_print | boolean | no | 是否格式化输出 markdown，默认 true |
| dry_run | boolean | no | 仅预览不实际写入，默认 false |

**Returns:**
- saved_files: 已保存文件列表
- errors: 保存错误列表

#### 3.1.3 okf_bundle_stats

获取当前加载 bundle 的统计信息。

**Parameters:** 无

**Returns:**
- total_concepts: 概念总数
- by_type: 按类型分组统计
- trust_tier_counts: 按信任等级统计 (unverified/machine-confirmed/human-reviewed)
- status_counts: 按状态统计 (active/deprecated/archived)
- stale_count: 已过期概念数
- attested_computation_count: Attested Computation 概念数
- sources_count: 来源引用总数

#### 3.1.4 okf_reload_bundle

重新加载当前 bundle，刷新索引。

**Parameters:** 无

**Returns:**
- reloaded: 是否成功
- changes_detected: 检测到的变更数

### 3.2 Concept Operations (concepts)

概念的 CRUD 操作。

#### 3.2.1 okf_get_concept

获取单个概念的完整内容。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| path | string | yes* | 概念文件路径（path 或 id 二选一） |
| id | string | yes* | 概念唯一标识 |
| include_body | boolean | no | 是否包含 markdown body，默认 true |
| include_custom_fields | boolean | no | 是否包含自定义字段，默认 true |

**Returns:**
- concept: 完整概念对象（type, title, description, tags, sources, generated, verified, status, stale_after, runtime, parameters, computation, executor, attester, body, custom_fields）
- trust_tier: 派生的信任等级
- is_stale: 是否已过期
- is_attested_computation: 是否为 Attested Computation

#### 3.2.2 okf_list_concepts

列出概念，支持过滤和分页。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| type | string | no | 按类型过滤 |
| tags | string[] | no | 按标签过滤（AND 逻辑） |
| trust_tier | string | no | 按信任等级过滤 (unverified/machine-confirmed/human-reviewed) |
| status | string | no | 按状态过滤 (active/deprecated/archived) |
| exclude_stale | boolean | no | 排除已过期概念，默认 false |
| has_sources | boolean | no | 仅返回有来源引用的概念 |
| has_verified | boolean | no | 仅返回有验证记录的概念 |
| limit | integer | no | 返回数量限制，默认 50 |
| offset | integer | no | 分页偏移，默认 0 |
| sort_by | string | no | 排序字段 (path/title/type/generated_at)，默认 path |
| sort_order | string | no | 排序方向 (asc/desc)，默认 asc |

**Returns:**
- concepts: 概念摘要列表（path, type, title, tags, trust_tier, status, is_stale）
- total: 符合条件的总数
- has_more: 是否有更多结果

#### 3.2.3 okf_create_concept

创建新概念。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| path | string | yes | 概念文件路径（相对 bundle 根目录） |
| type | string | yes | 概念类型（必填，SPEC v0.2 唯一必填字段） |
| title | string | no | 概念标题（推荐但可选） |
| description | string | no | 概念描述 |
| tags | string[] | no | 标签列表 |
| resource | string | no | 关联资源路径 |
| sources | object[] | no | 来源引用列表 |
| generated | object | no | 生成信息（by, at, method） |
| verified | object[] | no | 验证事件列表 |
| status | string | no | 概念状态 (active/deprecated/archived)，默认 active |
| stale_after | string | no | 过期日期 (YYYY-MM-DD) |
| runtime | string | no | Attested Computation 运行时 |
| parameters | object[] | no | Attested Computation 参数列表 |
| computation | object | no | Attested Computation 计算定义 |
| executor | object | no | Attested Computation 执行器引用 |
| attester | object | no | Attested Computation 证明者引用 |
| body | string | no | markdown body 内容 |
| custom_fields | object | no | 自定义 frontmatter 字段 |
| dry_run | boolean | no | 仅验证不实际写入，默认 false |

**Returns:**
- created: 是否成功
- path: 创建的文件路径
- concept: 创建后的概念对象
- lint_issues: 创建后 lint 检查结果

#### 3.2.4 okf_update_concept

更新现有概念。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| path | string | yes | 概念文件路径 |
| updates | object | yes | 更新字段（部分更新，仅包含需要修改的字段） |
| merge_mode | string | no | 数组字段合并模式 (replace/merge)，默认 replace |
| dry_run | boolean | no | 仅预览不实际写入，默认 false |

**Returns:**
- updated: 是否成功
- path: 文件路径
- before: 更新前概念摘要
- after: 更新后概念对象
- changes: 变更字段列表
- lint_issues: 更新后 lint 检查结果

#### 3.2.5 okf_delete_concept

删除概念。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| path | string | yes | 概念文件路径 |
| force | boolean | no | 强制删除（不确认），默认 false |
| dry_run | boolean | no | 仅预览不实际删除，默认 false |

**Returns:**
- deleted: 是否成功
- path: 删除的文件路径
- concept: 被删除的概念摘要

#### 3.2.6 okf_serialize_concept

将概念序列化为 markdown 字符串。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| path | string | yes* | 概念路径（path 或 concept 二选一） |
| concept | object | yes* | 概念对象 |
| pretty_print | boolean | no | 格式化输出，默认 true |

**Returns:**
- content: 序列化后的 markdown 字符串
- frontmatter: 提取的 frontmatter YAML

### 3.3 Query & Search (query)

查询和搜索能力。

#### 3.3.1 okf_search

全文搜索概念。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| query | string | yes | 搜索关键词 |
| search_in | string[] | no | 搜索范围 (title/description/body/tags/sources/custom_fields)，默认全部 |
| case_sensitive | boolean | no | 区分大小写，默认 false |
| regex | boolean | no | 使用正则表达式，默认 false |
| limit | integer | no | 结果数量限制，默认 20 |
| include_snippets | boolean | no | 包含匹配片段，默认 true |

**Returns:**
- results: 搜索结果列表（path, title, type, score, matched_fields, snippets）
- total: 匹配总数

#### 3.3.2 okf_query

结构化查询，使用查询构建器。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| type | string | no | 按类型过滤 |
| tags | string[] | no | 按标签过滤 |
| resource | string | no | 按关联资源过滤 |
| text | string | no | 全文文本过滤 |
| title_regex | string | no | 标题正则 |
| description_regex | string | no | 描述正则 |
| content_regex | string | no | 内容正则 |
| code_language | string | no | 代码语言过滤 |
| code_file_path | string | no | 代码文件路径过滤 |
| code_symbol_kind | string | no | 代码符号类型过滤 |
| code_qualified_name | string | no | 代码限定名过滤 |
| code_relation_kind | string | no | 代码关系类型过滤 |
| code_relation_source | string | no | 代码关系源过滤 |
| code_relation_target | string | no | 代码关系目标过滤 |
| limit | integer | no | 结果限制，默认 50 |

**Returns:**
- concepts: 匹配的概念列表
- total: 匹配总数

#### 3.3.3 okf_filter_by_trust_tier

按信任等级过滤概念。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| trust_tier | string | yes | 信任等级 (unverified/machine-confirmed/human-reviewed) |
| min_tier | string | no | 最低信任等级（包含及以上） |

**Returns:**
- concepts: 过滤后的概念列表
- count: 数量

#### 3.3.4 okf_filter_by_status

按状态过滤概念。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| status | string | yes | 状态 (active/deprecated/archived) |

**Returns:**
- concepts: 过滤后的概念列表
- count: 数量

#### 3.3.5 okf_filter_fresh

过滤未过期概念。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| reference_date | string | no | 参考日期 (YYYY-MM-DD)，默认今天 |

**Returns:**
- concepts: 未过期概念列表
- stale_count: 已过期数量
- fresh_count: 未过期数量

#### 3.3.6 okf_filter_by_source

按来源过滤概念。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| source_url | string | yes* | 来源 URL（url 或 author 或 title 至少一个） |
| source_author | string | no | 来源作者 |
| source_title | string | no | 来源标题 |

**Returns:**
- concepts: 匹配来源的概念列表
- count: 数量

### 3.4 Lint & Validation (lint)

lint 检查和 spec 验证。

#### 3.4.1 okf_lint_concept

Lint 单个概念。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| path | string | yes* | 概念路径（path 或 concept 二选一） |
| concept | object | yes* | 概念对象 |
| config | object | no | 自定义 lint 配置（规则开关、严重级别覆盖） |

**Returns:**
- issues: lint 问题列表（rule, severity, message, line, column, fix_suggestion）
- error_count: 错误数
- warning_count: 警告数
- info_count: 信息数
- has_errors: 是否有错误

#### 3.4.2 okf_lint_bundle

Lint 整个知识库。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| config | object | no | 自定义 lint 配置 |
| fail_on_error | boolean | no | 遇到错误即停止，默认 false |
| include_paths | string[] | no | 仅检查指定路径 |
| exclude_paths | string[] | no | 排除指定路径 |

**Returns:**
- results: 按文件分组的 lint 结果
- summary: 汇总统计（total_files, error_count, warning_count, info_count, files_with_errors）
- has_errors: 是否有错误

#### 3.4.3 okf_validate_spec

验证概念是否符合 OKF SPEC 版本。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| path | string | yes* | 概念路径 |
| concept | object | yes* | 概念对象 |
| spec_version | string | no | 目标 spec 版本 (v0.1/v0.2)，默认 v0.2 |

**Returns:**
- valid: 是否符合 spec
- spec_version: 检测到的 spec 版本
- conformance: 一致性检查结果（required_fields, recommended_fields, optional_fields, deprecated_fields）
- issues: 不符合项列表
- migration_suggestions: 迁移建议（v0.1→v0.2）

#### 3.4.4 okf_check_compatibility

检查 v0.1/v0.2 向后兼容性。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| path | string | no | 指定概念路径，不填则检查整个 bundle |

**Returns:**
- compatible: 是否兼容
- v01_concepts: v0.1 格式概念列表
- v02_concepts: v0.2 格式概念列表
- migration_needed: 需要迁移的概念列表
- legacy_fields: 使用的遗留字段（timestamp, # Citations）

### 3.5 Tool Service (tools)

okf 内置工具服务的查询和上下文获取。

#### 3.5.1 okf_tool_status

获取工具服务状态。

**Parameters:** 无

**Returns:**
- initialized: 是否已初始化
- indexed_tools: 已索引工具数
- indexed_concepts: 已索引概念数
- last_refresh: 最后刷新时间
- config: 当前配置

#### 3.5.2 okf_tool_init

初始化工具服务。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| config | object | no | 工具服务配置 |
| force | boolean | no | 强制重新初始化，默认 false |

**Returns:**
- initialized: 是否成功
- indexed_tools: 索引工具数
- indexed_concepts: 索引概念数

#### 3.5.3 okf_tool_refresh

刷新工具索引。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| paths | string[] | no | 仅刷新指定路径，不填则全量刷新 |

**Returns:**
- refreshed: 是否成功
- new_tools: 新增工具数
- updated_tools: 更新工具数
- removed_tools: 移除工具数

#### 3.5.4 okf_tool_query

查询已索引的工具。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| query | string | yes | 查询关键词 |
| limit | integer | no | 结果限制，默认 10 |

**Returns:**
- tools: 工具列表（name, description, source_path, score）
- total: 匹配总数

#### 3.5.5 okf_tool_context

获取工具的上下文信息（用于 Agent 调用工具时提供背景）。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| tool_name | string | yes | 工具名称 |
| include_relations | boolean | no | 包含关联概念，默认 true |
| include_trace | boolean | no | 包含追踪信息，默认 false |

**Returns:**
- context: 上下文内容
- source_files: 源文件列表
- related_concepts: 关联概念列表
- omissions: 省略内容说明

### 3.6 Attested Computation (computation)

Attested Computation 概念的管理和执行。

#### 3.6.1 okf_list_computations

列出所有 Attested Computation 概念。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| runtime | string | no | 按运行时过滤 (bigquery/dbt/python/...) |
| status | string | no | 按状态过滤 |
| exclude_stale | boolean | no | 排除已过期，默认 false |

**Returns:**
- computations: Attested Computation 列表（path, title, runtime, executor, attester, status, is_stale）
- count: 总数

#### 3.6.2 okf_get_computation

获取 Attested Computation 详情。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| path | string | yes | 概念路径 |

**Returns:**
- computation: 完整计算定义（runtime, parameters, computation body, executor, attester）
- sources: 来源引用
- verified: 验证记录
- trust_tier: 信任等级
- is_stale: 是否已过期

#### 3.6.3 okf_execute_computation

执行 Attested Computation（需要配置执行器）。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| path | string | yes | 概念路径 |
| parameters | object | no | 运行时参数覆盖 |
| dry_run | boolean | no | 仅生成执行计划不实际执行，默认 false |
| timeout_seconds | integer | no | 超时时间，默认 300 |

**Returns:**
- executed: 是否成功执行
- result: 执行结果
- execution_log: 执行日志
- attester_output: 证明者输出
- duration_ms: 执行耗时
- errors: 错误列表

### 3.7 Git Integration (git)

Git 版本控制集成。

#### 3.7.1 okf_git_status

获取 git 工作区状态。

**Parameters:** 无

**Returns:**
- branch: 当前分支
- modified: 修改的文件列表
- added: 新增的文件列表
- deleted: 删除的文件列表
- untracked: 未跟踪的文件列表
- ahead: 领先远程的提交数
- behind: 落后远程的提交数

#### 3.7.2 okf_git_diff

查看文件差异。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| path | string | no | 指定文件路径，不填则显示所有变更 |
| staged | boolean | no | 显示已暂存的变更，默认 false |
| context_lines | integer | no | 上下文行数，默认 3 |

**Returns:**
- diffs: 按文件分组的 diff 内容
- summary: 变更统计（files_changed, insertions, deletions）

#### 3.7.3 okf_git_commit

提交变更。

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| message | string | yes | 提交信息 |
| paths | string[] | no | 仅提交指定文件，不填则提交所有变更 |
| all | boolean | no | 提交所有变更（git add -A），默认 false |
| dry_run | boolean | no | 仅预览不实际提交，默认 false |

**Returns:**
- committed: 是否成功
- commit_hash: 提交哈希
- branch: 当前分支
- files_committed: 提交的文件列表

## 4. Resources Specification

资源通过 URI 寻址，提供只读数据访问。

### 4.1 Bundle Resource

**URI Template:** `okf://bundle/{path}`

**Description:** 知识库 bundle 的完整元数据和统计信息。

**Content Type:** `application/json`

**Example URI:** `okf://bundle//path/to/knowledge-base`

**Response:**
```json
{
  "path": "/path/to/knowledge-base",
  "total_concepts": 150,
  "stats": { "...": "..." },
  "loaded_at": "2026-08-26T22:00:00Z"
}
```

### 4.2 Concept Resource

**URI Template:** `okf://concept/{bundle_path}/{concept_path}`

**Description:** 单个概念的完整内容（frontmatter + body）。

**Content Type:** `text/markdown`

**Example URI:** `okf://concept//path/to/kb/metrics/revenue.md`

**Response:** 概念的 markdown 原始内容。

### 4.3 Concept JSON Resource

**URI Template:** `okf://concept-json/{bundle_path}/{concept_path}`

**Description:** 单个概念的结构化 JSON 表示。

**Content Type:** `application/json`

**Example URI:** `okf://concept-json//path/to/kb/metrics/revenue.md`

### 4.4 Index Resource

**URI Template:** `okf://index/{bundle_path}/{directory}`

**Description:** 目录的 index.md 内容和概念列表。

**Content Type:** `application/json`

**Example URI:** `okf://index//path/to/kb/metrics`

**Response:**
```json
{
  "directory": "metrics",
  "index_content": "...",
  "concepts": ["revenue.md", "profit.md", "..."],
  "subdirectories": ["..."]
}
```

### 4.5 Log Resource

**URI Template:** `okf://log/{bundle_path}`

**Description:** 知识库的更新日志（log.md）。

**Content Type:** `text/markdown`

**Example URI:** `okf://log//path/to/kb`

### 4.6 Search Resource

**URI Template:** `okf://search/{bundle_path}?q={query}&type={type}&tags={tags}`

**Description:** 搜索结果的资源表示。

**Content Type:** `application/json`

**Example URI:** `okf://search//path/to/kb?q=revenue&type=metric`

## 5. Prompts Specification

参数化提示词模板，为常见工作流提供预设。

### 5.1 okf_explain_concept

解释一个概念的含义和上下文。

**Arguments:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| concept_path | string | yes | 概念路径 |
| depth | string | no | 解释深度 (brief/detailed/comprehensive)，默认 detailed |
| audience | string | no | 目标受众 (beginner/technical/executive)，默认 technical |

### 5.2 okf_summarize_bundle

总结整个知识库或指定子目录。

**Arguments:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| directory | string | no | 指定子目录，不填则整个 bundle |
| include_stats | boolean | no | 包含统计信息，默认 true |
| format | string | no | 输出格式 (markdown/bullets/paragraph)，默认 markdown |

### 5.3 okf_generate_report

生成知识库健康度报告。

**Arguments:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| include_lint | boolean | no | 包含 lint 结果，默认 true |
| include_trust | boolean | no | 包含信任等级分析，默认 true |
| include_stale | boolean | no | 包含过期概念分析，默认 true |
| include_gaps | boolean | no | 包含内容缺口分析，默认 true |

### 5.4 okf_review_changes

审查 git 工作区的变更。

**Arguments:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| staged_only | boolean | no | 仅审查已暂存变更，默认 false |
| include_lint | boolean | no | 对变更文件运行 lint，默认 true |
| focus | string | no | 审查重点 (correctness/completeness/style/all)，默认 all |

### 5.5 okf_migrate_v01_to_v02

生成 v0.1 到 v0.2 的迁移建议。

**Arguments:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| path | string | no | 指定概念路径，不填则整个 bundle |
| apply | boolean | no | 直接应用迁移（不只是建议），默认 false |
| backup | boolean | no | 应用迁移前备份，默认 true |

### 5.6 okf_create_concept_from_source

根据来源引用创建新概念。

**Arguments:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| source_url | string | yes | 来源 URL |
| concept_type | string | yes | 概念类型 |
| title | string | no | 概念标题（不填则从来源提取） |
| path | string | no | 目标路径（不填则自动生成） |

## 6. Configuration

### 6.1 Server Configuration

通过环境变量或配置文件配置。

| Environment Variable | Type | Default | Description |
|---------------------|------|---------|-------------|
| OKF_MCP_BUNDLE_PATH | string | (required) | 默认知识库路径 |
| OKF_MCP_TRANSPORT | string | stdio | 传输方式 (stdio/http) |
| OKF_MCP_HTTP_HOST | string | 127.0.0.1 | HTTP 传输监听地址 |
| OKF_MCP_HTTP_PORT | integer | 8765 | HTTP 传输端口 |
| OKF_MCP_TOOLSETS | string | all | 启用的工具集，逗号分隔 (bundle,concepts,query,lint,tools,computation,git) |
| OKF_MCP_READ_ONLY | boolean | false | 只读模式（禁用写操作） |
| OKF_MCP_ALLOWED_PATHS | string | (all) | 允许访问的路径前缀，逗号分隔 |
| OKF_MCP_LOG_LEVEL | string | info | 日志级别 (debug/info/warn/error) |
| OKF_MCP_MAX_CONCEPTS | integer | 10000 | 最大加载概念数 |
| OKF_MCP_AUTO_RELOAD | boolean | true | 文件变更时自动重载 |

### 6.2 Client Configuration Example

**Claude Desktop (`~/.claude.json`):**
```json
{
  "mcpServers": {
    "okf": {
      "command": "okf",
      "args": ["mcp"],
      "env": {
        "OKF_MCP_BUNDLE_PATH": "/path/to/knowledge-base",
        "OKF_MCP_TOOLSETS": "bundle,concepts,query,lint"
      }
    }
  }
}
```

**Cursor (`.cursor/mcp.json`):**
```json
{
  "mcpServers": {
    "okf": {
      "command": "okf",
      "args": ["mcp", "--bundle", "/path/to/knowledge-base"]
    }
  }
}
```

**HTTP Transport:**
```json
{
  "mcpServers": {
    "okf": {
      "url": "http://127.0.0.1:8765/mcp"
    }
  }
}
```

## 7. Error Handling

所有工具错误使用结构化格式：

```json
{
  "error": {
    "code": "OKF_ERR_CODE",
    "message": "Human-readable error message",
    "details": { "...": "..." },
    "retryable": false
  }
}
```

### 7.1 Error Codes

| Code | Description | Retryable |
|------|-------------|-----------|
| OKF_BUNDLE_NOT_LOADED | 知识库未加载 | false |
| OKF_BUNDLE_NOT_FOUND | 知识库路径不存在 | false |
| OKF_CONCEPT_NOT_FOUND | 概念不存在 | false |
| OKF_CONCEPT_ALREADY_EXISTS | 概念已存在 | false |
| OKF_INVALID_PATH | 路径无效或越界 | false |
| OKF_PARSE_ERROR | 解析错误 | false |
| OKF_SERIALIZE_ERROR | 序列化错误 | false |
| OKF_LINT_ERROR | Lint 检查失败 | false |
| OKF_VALIDATION_ERROR | 验证失败 | false |
| OKF_READ_ONLY | 只读模式下的写操作 | false |
| OKF_PERMISSION_DENIED | 权限不足 | false |
| OKF_GIT_ERROR | Git 操作失败 | true |
| OKF_IO_ERROR | IO 错误 | true |
| OKF_INTERNAL_ERROR | 内部错误 | true |
| OKF_TOOLSET_DISABLED | 工具集未启用 | false |

## 8. Notifications

### 8.1 Tool List Changed

当工具列表变更时（如工具集配置变更），发送 `notifications/tools/list_changed`。

### 8.2 Resource Changed

当知识库文件变更时，发送 `notifications/resources/updated`，包含变更资源的 URI。

### 8.3 Bundle Reloaded

当 bundle 自动重载完成时，发送自定义通知 `notifications/okf/bundle_reloaded`，包含变更统计。

## 9. Implementation Architecture

```
cmd/okf/main.go
  └── mcp command → pkg/mcp/server.go
                      ├── pkg/mcp/transport/ (stdio, http)
                      ├── pkg/mcp/tools/ (7 toolsets)
                      │   ├── bundle.go
                      │   ├── concepts.go
                      │   ├── query.go
                      │   ├── lint.go
                      │   ├── toolsvc.go
                      │   ├── computation.go
                      │   └── git.go
                      ├── pkg/mcp/resources/ (resource handlers)
                      ├── pkg/mcp/prompts/ (prompt templates)
                      ├── pkg/mcp/config.go
                      └── pkg/mcp/errors.go
```

### 9.1 Dependencies

- MCP Go SDK: `github.com/mark3labs/mcp-go` (社区主流) 或官方 SDK
- 现有 okf 包: `pkg/okf`, `pkg/parser`, `pkg/query`, `pkg/lint`, `pkg/tool`, `pkg/git`

### 9.2 Thread Safety

- Bundle 加载后使用读写锁保护
- 写操作（create/update/delete）获取写锁
- 读操作（get/list/search/lint）获取读锁
- 自动重载使用单独的 goroutine，变更时原子切换 bundle 引用

## 10. Testing Strategy

### 10.1 Unit Tests
- 每个工具函数的参数验证和错误处理
- 资源 URI 解析和内容生成
- 提示词模板渲染

### 10.2 Integration Tests
- MCP 协议消息级测试（initialize, tools/list, tools/call）
- stdio 传输端到端测试
- 完整工作流测试（load → list → get → create → lint → save）

### 10.3 Compatibility Tests
- 与 Claude Desktop MCP Client 的兼容性
- 与 Cursor MCP Client 的兼容性
- v0.1 和 v0.2 知识库的工具行为一致性

## 11. Future Enhancements

- **Streaming Responses**: 大查询结果的流式传输
- **Caching Layer**: 频繁查询结果的缓存
- **Multi-bundle Support**: 同时加载多个知识库并支持跨库查询
- **Authentication**: HTTP 传输的 Token 认证
- **Web UI**: 轻量级 Web 管理界面
- **Plugin System**: 自定义工具集的插件机制
- **Vector Search**: 嵌入向量语义搜索集成
