# 性能分析与边界条件

## 1. 性能目标

### 1.1 单次导入性能

| 操作 | 100 个文件 | 1000 个文件 | 5000 个文件 |
|------|-----------|-------------|-------------|
| FAST PATH (mtime+size) | < 50ms | < 200ms | < 1s |
| 完整 hash 检测 | < 200ms | < 2s | < 10s |
| 完整导入 (含写文件) | < 500ms | < 5s | < 25s |

**假设**: 平均文件大小 5KB，SSD 存储。

### 1.2 Watch daemon 性能

| 指标 | 目标 |
|------|------|
| 事件响应延迟 | < 1s |
| 内存占用 | < 50MB (1000 files) |
| CPU idle | < 1% (无事件时) |
| 事件去抖命中率 | > 90% (编辑器保存场景) |

## 2. 性能优化策略

### 2.1 FAST PATH 优化

```go
// FAST PATH: 99% of cases when files are unchanged
// O(1) 系统调用: os.Stat() → mtime + size 比较
func detectFast(meta FileMetadata, sourcePath string) DetectResult {
    info, err := os.Stat(sourcePath)
    if err != nil { return DetectSourceMissing }

    if meta.LastModified == info.ModTime() &&
       meta.FileSize == info.Size() {
        return DetectNoChange  // 无变更 → FAST PATH 命中
    }
    return DetectContentChanged  // 需要 hash 比较 (SLOW PATH)
}
```

**设计意图**:
- mtime + size 足以检测绝大多数真实变更
- FAST PATH 命中率 > 95% (典型场景)
- 零文件 IO，仅一次系统调用

### 2.2 并行检测

```go
// 检测阶段可以并行执行 (IO 密集型)
func (d *detector) DetectAll(files []string) map[string]DetectResult {
    var wg sync.WaitGroup
    results := make(map[string]DetectResult, len(files))
    mu := sync.Mutex{}

    // 限制并发度，避免文件描述符耗尽
    semaphore := make(chan struct{}, 32)

    for _, f := range files {
        wg.Add(1)
        go func(file string) {
            defer wg.Done()
            semaphore <- struct{}{}
            defer func() { <-semaphore }()

            result, _ := d.DetectOne(file)
            mu.Lock()
            results[file] = result
            mu.Unlock()
        }(f)
    }

    wg.Wait()
    return results
}
```

### 2.3 内存优化

- **元数据索引内存占用**
  ```
  1000 files × (200 bytes path + 64 bytes hash + 32 bytes timestamps)
  ≈ 300KB per index
  ```

- **Content 不缓存**
  不缓存文件内容，避免内存膨胀。每次检测时按需读取。

### 2.4 Hash 计算优化

```go
// 流式 hash: 不一次性加载整个文件到内存
func sha256stream(path string) (string, error) {
    f, err := os.Open(path)
    if err != nil { return "", err }
    defer f.Close()

    h := sha256.New()
    buf := make([]byte, 64*1024)  // 64KB buffer

    for {
        n, err := f.Read(buf)
        if n > 0 { h.Write(buf[:n]) }
        if err == io.EOF { break }
        if err != nil { return "", err }
    }
    return hex.EncodeToString(h.Sum(nil)), nil
}
```

## 3. 边界条件处理

### 3.1 文件系统边界

| 场景 | 处理方式 |
|------|---------|
| 源文件被删除 | 从索引中标记为 missing，或使用 `--prune` 清理 |
| 目标路径只读 | 清晰错误提示 + skip 策略 |
| 文件被锁定 | 重试 3 次 (间隔 100ms) |
| 临时文件 (`swp`, `tmp`, `.DS_Store`) | 扩展名过滤 |
| 符号链接 | 跟随 symlink (可选安全开关禁用) |

### 3.2 元数据边界

| 场景 | 处理方式 |
|------|---------|
| `.metadata.json` 不存在 | 初始化空索引 |
| `.metadata.json` 损坏 | 忽略损坏文件，从内容重建 |
| 元数据版本旧 | 自动升级迁移 |
| `.metadata.json.tmp` 残留 | 启动时清理 |

### 3.3 并发安全边界

| 场景 | 处理方式 |
|------|---------|
| 多个 okf 进程并发写入 | 文件锁 (fcntl on Linux, LockFileEx on Windows) |
| Watch daemon 与手动命令同时运行 | pidfile 检测 + 警告 |
| 同一文件多次触发事件 | Debouncer 去抖 + 任务队列去重 |

### 3.4 资源使用边界

| 资源 | 限制 |
|------|------|
| 单文件大小 | < 10MB |
| 知识库总文件 | < 10000 |
| 单次操作总大小 | < 100MB |
| 并行度 | < 32 (检测阶段) |

## 4. 错误恢复策略

### 4.1 导入失败恢复

```
文件写入失败 (磁盘满 / 权限错):
  Step 1: 不更新元数据索引
  Step 2: 记录到 failed 列表
  Step 3: 下次导入自动重试
  Step 4: 不回滚已成功的文件

元数据写入失败:
  Step 1: 保留 .tmp 文件 (下次启动可恢复)
  Step 2: 下次启动检测到 .tmp 时，尝试恢复
```

### 4.2 Watch daemon 崩溃恢复

```
Daemon 崩溃自动恢复:
  Step 1: 检测 pidfile 是否存在
  Step 2: 检查 pid 是否有效 (kill -0 pid)
  Step 3: 若无效，清理 pidfile 并正常启动
  Step 4: 在启动时，对所有规则路径执行一次完整扫描
```

## 5. 向后兼容策略

### 5.1 无元数据的知识库

```
用户: okf watch --dir ./knowledge
知识库不含 .metadata.json:
  Step 1: 执行 rebuild 扫描所有文件
  Step 2: 对每个文件计算 hash，记录到新的索引
  Step 3: 启动 watch
```

### 5.2 旧版本元数据文件

```
.version = "0.9" (当前 1.0):
  Step 1: 检测版本
  Step 2: 执行迁移逻辑
  Step 3: 写入新版元数据文件
```

## 6. 测试覆盖策略

### 6.1 单元测试

| 模块 | 测试重点 | 覆盖率目标 |
|------|---------|-----------|
| 元数据读写 | 损坏文件恢复、版本升级 | 95% |
| 变更检测 | FAST PATH 命中、SLOW PATH 精确 | 90% |
| 合并策略 | 4 种策略 + 边界 | 95% |
| Watch daemon | 事件去抖、信号处理 | 85% |

### 6.2 集成测试

| 场景 | 测试目标 |
|------|---------|
| 1000 文件导入 | 内存 < 50MB, 时间 < 5s |
| Watch daemon 运行 1h | 无内存泄漏 |
| 并行执行命令 | 数据一致性 |
| 磁盘满 / 权限错 | 优雅降级 |

### 6.3 E2E 测试

```bash
# 场景: 真实开发工作流
$ mkdir -p /tmp/docs /tmp/kb
$ cat > /tmp/docs/api.md << 'EOF'
---
type: api
title: Test API
tags:
  - test
---

API Content
EOF

$ okf add /tmp/docs --dir /tmp/kb --strategy=overwrite
# 验证: api.md 被导入，metadata.json 被创建

$ echo "edit API" >> /tmp/docs/api.md
$ okf add /tmp/docs --dir /tmp/kb --strategy=overwrite
# 验证: api.md 被更新，hash 变化

$ okf watch --dir /tmp/kb &
$ echo "new content" > /tmp/docs/new.md
# 验证: new.md 被自动导入
```

## 7. 潜在风险与缓解

### 7.1 文件系统事件丢失

**风险**: fsnotify 在某些场景下可能丢失事件 (Linux inotify watch 数量限制)

**缓解**:
- 注册 watch 失败时记录警告
- 定期 (每 5 min) 执行一次全量扫描作为补偿
- 提供 `okf watch --full-scan` 手动全量扫描命令

### 7.2 Hash 碰撞

**风险**: SHA256 理论上存在碰撞可能

**缓解**:
- 使用标准库 `crypto/sha256` (经过严格审计)
- 对关键操作，双重验证: hash + mtime/size
- 如果需要更高安全，可切换到 SHA3-256

### 7.3 并发写入元数据

**风险**: 多个 okf 命令/daemon 同时写入 `.metadata.json`

**缓解**:
- 文件锁 (`flock` on Unix, `LockFileEx` on Windows)
- 写入时使用 RWMutex (进程内)
- 每次写入前校验当前版本是否匹配

### 7.4 编辑器临时文件

**风险**: Vim/VSCode 保存时产生 `.swp`, `4913` 测试文件等

**缓解**:
- 基于扩展名白名单过滤: `.md` only
- Debouncer 窗口: 500ms 内的多次事件合并
- 检查文件完整性: size > 0 and not a lockfile pattern
