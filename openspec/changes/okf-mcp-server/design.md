# Design: OKF MCP Server

## Architecture Overview

OKF MCP Server 采用分层架构，将 MCP 协议层与 okf 业务逻辑解耦。

```
┌─────────────────────────────────────────────────┐
│              MCP Clients (Claude/Cursor/...)    │
└───────────────────────┬─────────────────────────┘
                        │ JSON-RPC 2.0
                        ▼
┌─────────────────────────────────────────────────┐
│  Transport Layer (stdio / Streamable HTTP)      │
├─────────────────────────────────────────────────┤
│  Protocol Layer (MCP message routing)           │
├─────────────────────────────────────────────────┤
│  Tool Registry (7 toolsets, 30+ tools)         │
├─────────────────────────────────────────────────┤
│  Resource Handlers (6 resource types)            │
├─────────────────────────────────────────────────┤
│  Prompt Templates (6 preset workflows)           │
├─────────────────────────────────────────────────┤
│  OKF Core (pkg/okf, pkg/parser, pkg/query, ...)│
└─────────────────────────────────────────────────┘
```

## Core Design Decisions

### 1. Toolset-based Modularity

参考 GitHub MCP Server 的 toolsets 模式，将 30+ 工具按功能域分为 7 个可独立开关的工具集：

- **bundle**: 知识库生命周期管理（4 tools）
- **concepts**: 概念 CRUD（6 tools）
- **query**: 查询搜索（6 tools）
- **lint**: 验证检查（4 tools）
- **tools**: 内置工具服务（5 tools）
- **computation**: Attested Computation（3 tools）
- **git**: 版本控制集成（3 tools）

通过 `OKF_MCP_TOOLSETS` 环境变量配置启用的工具集，减少暴露面和启动开销。

### 2. Read-Only Mode Safety

支持 `OKF_MCP_READ_ONLY=true` 只读模式，禁用所有写操作（create/update/delete/save/commit/execute），适用于仅需查询的 Agent 场景，防止意外修改。

### 3. Path Sandboxing

通过 `OKF_MCP_ALLOWED_PATHS` 配置允许访问的路径前缀，所有文件操作前验证路径是否在允许范围内，防止路径穿越攻击。

### 4. Bundle State Management

- 单例 bundle 实例，通过 `sync.RWMutex` 保护并发访问
- 写操作获取写锁，读操作获取读锁
- 自动重载使用 `fsnotify` 监听文件变更，debounce 后原子切换 bundle 引用
- 支持 `okf_reload_bundle` 手动触发重载

### 5. Error Normalization

所有工具错误统一转换为结构化 MCP 错误格式，包含：
- 机器可读的错误码（`OKF_*`）
- 人类可读的错误消息
- 可选的详细信息
- 是否可重试标记

### 6. Dry Run Pattern

所有写操作工具支持 `dry_run` 参数，启用时仅验证和预览变更，不实际写入。这使得 Agent 可以在执行前预览影响，降低误操作风险。

## Data Flow

### Tool Call Flow

```
Client → tools/call (JSON-RPC)
       → Transport (parse JSON-RPC)
       → Protocol Router (dispatch to tool handler)
       → Tool Handler (validate params, acquire lock)
       → OKF Core (execute business logic)
       → Result Normalizer (format MCP response)
       → Transport (serialize JSON-RPC)
       → Client
```

### Resource Read Flow

```
Client → resources/read (URI: okf://concept/...)
       → URI Parser (extract bundle_path, concept_path)
       → Resource Handler (lookup concept, format content)
       → Content Response (text/markdown or application/json)
       → Client
```

### Auto-Reload Flow

```
fsnotify event → debounce (500ms)
               → reload bundle in background goroutine
               → atomic swap bundle pointer
               → emit notifications/resources/updated
               → emit notifications/okf/bundle_reloaded
```

## Dependency Strategy

### Primary Dependency

- `github.com/mark3labs/mcp-go` — 社区主流 Go MCP SDK，支持 stdio 和 HTTP 传输，工具/资源/提示词完整支持

### Fallback

如 mark3labs/mcp-go 不稳定，可降级为：
- 直接实现 JSON-RPC 2.0 over stdio（协议简单，约 500 行代码）
- HTTP 传输使用标准库 `net/http` + SSE

### No New Runtime Dependencies

- 不引入数据库（bundle 状态在内存）
- 不引入消息队列（通知是同步的）
- 不引入 ORM（直接使用 okf 现有类型）

## Performance Considerations

### Indexing

- Bundle 加载时构建内存索引（path → concept, type → concepts, tags → concepts）
- 查询优先使用索引，避免全量扫描
- 大知识库（>1000 concepts）考虑懒加载和分页

### Serialization

- 概念序列化缓存（修改时失效）
- 列表接口返回摘要（不含 body），减少传输量
- 大结果集支持分页和流式传输（future）

### Concurrency

- 读操作并发安全（RWMutex 读锁）
- 写操作串行化（RWMutex 写锁）
- 工具执行不阻塞 MCP 消息循环（每个调用在独立 goroutine）

## Security Model

### Threat Model

| Threat | Mitigation |
|--------|-----------|
| 路径穿越 | `OKF_MCP_ALLOWED_PATHS` 白名单 + `filepath.Clean` 验证 |
| 意外修改 | 只读模式 + dry_run + 写操作需确认（future） |
| 敏感信息泄露 | 日志脱敏 + 自定义字段白名单（future） |
| 拒绝服务 | 最大概念数限制 + 查询超时 + 结果分页 |
| 恶意 frontmatter | YAML 解析限制（递归深度、大小） |

### Trust Boundaries

- MCP Client 被视为不可信输入源
- 所有参数严格验证（类型、范围、路径）
- 错误消息不泄露内部路径和堆栈（除非 debug 模式）

## Migration Path

### Phase 1: Core (MVP)
- stdio 传输
- bundle + concepts + query + lint 工具集
- concept + bundle 资源
- 基础提示词模板

### Phase 2: Extended
- HTTP 传输
- tools + computation + git 工具集
- 全部资源类型
- 自动重载和通知
- 完成(Completion)支持

### Phase 3: Advanced
- 采样(Sampling)回调
- 多 bundle 支持
- 认证和权限
- 流式响应
- 插件系统

## Open Questions

1. **MCP Go SDK 选择**: mark3labs/mcp-go vs 官方 SDK（官方 Go SDK 尚未稳定发布）
2. **HTTP 传输版本**: Streamable HTTP（2025-03-26 spec）vs 旧版 HTTP+SSE（兼容性）
3. **工具命名空间**: 是否需要 `okf_` 前缀（避免与其他 MCP Server 工具名冲突）
4. **概念 ID**: v0.2 spec 未定义唯一 ID，是否需要生成（path hash）或仅用 path 标识
5. **执行器集成**: Attested Computation 的执行器如何配置和调用（是否需要独立的执行器子进程）
