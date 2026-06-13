# okf — Open Knowledge Format

> AI Agent 项目级知识库系统，支持从 Git 仓库自动生成知识、规范检查和自动化更新。

## 功能特性

- **📁 Open Knowledge Format** — 基于 Markdown + YAML Frontmatter 的开放知识格式
- **🔍 自动生成** — 扫描 Git 仓库代码自动生成项目知识库
- **⚡ 增量更新** — 基于 Git 提交的增量更新
- **🛠 Git Hook** — 一键安装，每次提交自动更新知识库
- **📋 Lint 检查** — 内置规范检查（13 条规则）
- **🔎 高级查询** — 支持按类型、标签、全文搜索
- **🏗 模块化架构** — 清晰的分层设计

## 项目结构

```
.
├── cmd/okf/          # CLI 入口程序
│   └── main.go      # 主入口
├── pkg/
│   ├── okf/         # 核心类型和 API
│   │   ├── types.go # Concept, KnowledgeBundle 类型定义
│   │   ├── api.go   # 加载/保存 bundle
│   │   ├── errors.go # 错误类型
│   │   ├── helpers.go # 辅助函数
│   │   └── meta/    # 版本信息
│   ├── parser/      # Markdown + YAML 解析器
│   │   └── parser.go
│   ├── query/       # 查询引擎
│   │   └── query.go
│   ├── lint/        # 规范检查
│   │   └── lint.go
│   └── git/         # Git 集成
│       ├── git.go       # Git 操作
│       └── generator.go # 知识库生成
├── go.mod
└── README.md
```

## 快速开始

```bash
# 安装 CLI
go build -o okf ./cmd/okf/

# 初始化知识库
cd /your/repo
./okf init

# 查看知识库
./okf show

# 搜索
./okf search -q "database"

# Lint 检查
./okf lint

# 安装 Git Hook（自动更新）
./okf hook -type post-commit
```

## 模块说明

| 模块 | 路径 | 功能 |
|------|------|------|
| **okf** | pkg/okf/ | 核心类型定义（Concept, KnowledgeBundle）和公共 API |
| **parser** | pkg/parser/ | Markdown + YAML frontmatter 解析和序列化 |
| **query** | pkg/query/ | 高级查询构建器和匹配引擎 |
| **lint** | pkg/lint/ | OKF 规范检查（13 条规则） |
| **git** | pkg/git/ | Git 仓库扫描、代码分析、知识库生成 |

## OKF 概念格式

```markdown
---
type: table
title: users
description: User accounts table
resource: bigquery.project.dataset.users
tags:
  - production
  - pii
timestamp: "2024-01-15T10:30:00Z"
---

## Users Table

存储所有用户账户信息。
```

## API 使用

```go
import (
    okf "github.com/superops-team/okf/pkg/okf"
    "github.com/superops-team/okf/pkg/git"
    "github.com/superops-team/okf/pkg/lint"
)

// 加载知识库
bundle, err := okf.LoadBundle(".okf/knowledge", nil)

// 搜索
results := bundle.Search("database")

// Lint 检查
result := lint.LintBundle(concepts, lint.DefaultConfig())

// 从 Git 生成
bundle, err := git.GenerateBundle(cfg, false)
```

## Lint 规则

| 代码 | 严重度 | 说明 |
|------|--------|------|
| OKF001 | ERROR | type 字段不能为空 |
| OKF002 | ERROR | title 字段不能为空 |
| OKF003 | WARNING | description 太短 |
| OKF004 | WARNING | type 应为小写 |
| OKF005 | WARNING | timestamp 格式不正确 |
| OKF006 | WARNING | 标签包含大写或空格 |
| OKF007 | WARNING | 内容体为空 |
| OKF009 | WARNING | 内容行过长 |
| OKF010 | WARNING | 重复标签 |

## License

Apache 2.0
