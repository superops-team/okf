package okf

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// =============================================================================
// Watch Daemon (Phase 4.2)
// =============================================================================

// WatchEvent 记录一次检测到的源文件变更
type WatchEvent struct {
	// SourcePath 绝对路径
	SourcePath string

	// Rule 触发变更规则的索引（指向 cfg.Rules 中的规则）
	RuleIndex int

	// Op 原始 fsnotify 操作（调试用）
	Op fsnotify.Op
}

// WatchDaemon 监听并自动同步知识库
//
// 数据流：
//
//	fsnotify events → filter (rule pattern match) → debouncer → processor
//	                                                               │
//	                                                               ▼
//	                                                    SmartImporter.ImportFile
//	                                                    MetadataIndex.Save
type WatchDaemon struct {
	cfg       *WatchConfig
	idx       *MetadataIndex
	importer  *SmartImporter
	watcher   *fsnotify.Watcher
	mu        sync.Mutex
	logger    *log.Logger
	debounce  map[string]*time.Timer // sourcePath → timer
	eventChan chan WatchEvent
	stopCh  chan struct{}
	wg        sync.WaitGroup
	running   bool
}

// NewWatchDaemon 基于 WatchConfig 创建 WatchDaemon
//
// 使用：
//
//	d, err := NewWatchDaemon(cfg)
//	...
//	err = d.Run(ctx)  // 阻塞直到 ctx 取消或错误
func NewWatchDaemon(cfg *WatchConfig) (*WatchDaemon, error) {
	if cfg == nil {
		return nil, fmt.Errorf("watch config cannot be nil")
	}

	// 确保知识库目录存在
	if err := os.MkdirAll(cfg.KnowledgeDir, 0755); err != nil {
		return nil, fmt.Errorf("create knowledge dir: %w", err)
	}

	// 加载 / 初始化 metadata 索引
	metaPath := KnowledgeMetadataPath(cfg.KnowledgeDir)
	idx := NewMetadataIndex()
	if err := idx.Load(metaPath); err != nil {
		return nil, fmt.Errorf("load metadata index: %w", err)
	}

	return &WatchDaemon{
		cfg:       cfg,
		idx:       idx,
		importer:  NewSmartImporter(idx, cfg.KnowledgeDir),
		logger:    log.New(os.Stderr, "[okf-watch] ", log.LstdFlags),
		debounce:  make(map[string]*time.Timer),
		eventChan: make(chan WatchEvent, 64),
		stopCh:   make(chan struct{}),
	}, nil
}

// SetLogger 替换默认日志
func (d *WatchDaemon) SetLogger(l *log.Logger) {
	if l != nil {
		d.logger = l
	}
}

// Run 启动监听循环。阻塞直到 ctx 取消。
//
// 初始化：为每个 rule 的每个 source 目录添加 watcher（递归）
// 事件：接收 fsnotify 事件 → 过滤 → debounce → 处理
func (d *WatchDaemon) Run(ctx context.Context) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return fmt.Errorf("watch daemon already running")
	}
	d.running = true
	d.mu.Unlock()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create fsnotify watcher: %w", err)
	}
	d.watcher = watcher

	// 为每个 source 目录添加 watcher
	for _, rule := range d.cfg.Rules {
		r := rule // capture loop variable
		for _, src := range r.Sources {
			info, err := os.Stat(src)
			if err != nil {
				d.logger.Printf("WARN: cannot access source %q for rule %q: %v", src, r.Name, err)
				continue
			}
			if info.IsDir() {
				// 递归添加所有子目录
				err = filepath.Walk(src, func(p string, fi os.FileInfo, werr error) error {
					if werr != nil {
						return werr
					}
					if fi.IsDir() {
						if werr := watcher.Add(p); werr != nil {
							d.logger.Printf("WARN: cannot watch dir %q: %v", p, werr)
						}
					}
					return nil
				})
				if err != nil {
					d.logger.Printf("WARN: walk %q: %v", src, err)
				}
			} else {
				// 单个文件
				if err := watcher.Add(src); err != nil {
					d.logger.Printf("WARN: cannot watch file %q: %v", src, err)
				}
			}
		}
	}

	d.wg.Add(2)
	go d.dispatchLoop(ctx)
	go d.processLoop(ctx)

	d.logger.Printf("watch started: %d rules, knowledge dir: %s", len(d.cfg.Rules), d.cfg.KnowledgeDir)

	// 等待 ctx 取消
	<-ctx.Done()
	d.logger.Printf("shutting down...")

	// 关闭 stopCh（通知所有 pending 的 debounce 定时器退出）
	close(d.stopCh)

	// 清理所有 debounce 定时器
	d.mu.Lock()
	for _, t := range d.debounce {
		t.Stop()
	}
	d.debounce = make(map[string]*time.Timer)
	d.mu.Unlock()

	// 关闭 watcher
	if err := watcher.Close(); err != nil {
		d.logger.Printf("warn on close: %v", err)
	}

	d.wg.Wait()
	d.mu.Lock()
	d.running = false
	d.mu.Unlock()
	return nil
}

// dispatchLoop 接收 fsnotify 事件，过滤并发送到 eventChan
func (d *WatchDaemon) dispatchLoop(ctx context.Context) {
	defer d.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-d.watcher.Events:
			if !ok {
				return
			}
			d.handleFsEvent(event)
		case err, ok := <-d.watcher.Errors:
			if !ok {
				return
			}
			d.logger.Printf("fsnotify error: %v", err)
		}
	}
}

// handleFsEvent 处理一次 fsnotify 事件：过滤 + 按源 debounce
func (d *WatchDaemon) handleFsEvent(event fsnotify.Event) {
	// 目录：忽略事件，但为新建目录添加递归监听
	if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
		if event.Op&fsnotify.Create != 0 {
			_ = d.watcher.Add(event.Name)
			_ = filepath.Walk(event.Name, func(p string, fi os.FileInfo, werr error) error {
				if werr != nil {
					return nil
				}
				if fi.IsDir() {
					_ = d.watcher.Add(p)
				}
				return nil
			})
		}
		return
	}

	// 仅处理 Create / Write / Rename 事件
	if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) == 0 {
		return
	}

	eventSlash := filepath.ToSlash(event.Name)

	// 检查每个规则
	for ruleIdx, rule := range d.cfg.Rules {
		matched := false
		for _, src := range rule.Sources {
			srcSlash := filepath.ToSlash(src)
			if eventSlash == srcSlash || strings.HasPrefix(eventSlash, srcSlash+"/") {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		if !rule.MatchFilename(event.Name) {
			continue
		}

		// debounce：同一文件在 rule.Debounce 时间内的多个事件合并
		d.mu.Lock()
		if t, exists := d.debounce[event.Name]; exists {
			t.Stop()
		}
		debounceDur := rule.Debounce
		// AfterFunc 的闭包捕获 event.Name, event.Op, ruleIdx, d
		d.debounce[event.Name] = time.AfterFunc(debounceDur, func() {
			select {
			case d.eventChan <- WatchEvent{SourcePath: event.Name, RuleIndex: ruleIdx, Op: event.Op}:
			case <-d.stopCh:
				return
			}
			d.mu.Lock()
			delete(d.debounce, event.Name)
			d.mu.Unlock()
		})
		d.mu.Unlock()
	}
}

// processLoop 从 eventChan 读取 debounce 后的事件并导入
func (d *WatchDaemon) processLoop(ctx context.Context) {
	defer d.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-d.eventChan:
			if !ok {
				return
			}
			d.processEvent(evt)
		}
	}
}

// processEvent 处理单个文件事件：
// - 变更检测 → 导入/合并 → 持久化
func (d *WatchDaemon) processEvent(evt WatchEvent) {
	if evt.RuleIndex < 0 || evt.RuleIndex >= len(d.cfg.Rules) {
		return
	}
	rule := &d.cfg.Rules[evt.RuleIndex]

	// 构建 SmartImportOptions
	opts := &SmartImportOptions{
		ForceStrategy: rule.Strategy,
		PatchFields:   rule.PatchFields,
	}

	// 计算目标相对路径
	target := targetFromSource(evt.SourcePath, rule, d.cfg.KnowledgeDir)

	// 调用 SmartImporter
	result, err := d.importer.ImportFile(evt.SourcePath, target, opts)
	if err != nil {
		d.logger.Printf("ERROR import %q (rule=%s): %v", evt.SourcePath, rule.Name, err)
		return
	}

	// 仅在有实际变更时保存元数据
	if result != nil && result.Changed {
		metaPath := KnowledgeMetadataPath(d.cfg.KnowledgeDir)
		if serr := d.idx.Save(metaPath); serr != nil {
			d.logger.Printf("ERROR save metadata: %v", serr)
		}
		d.logger.Printf("imported [%s] %s -> %s", result.Strategy, evt.SourcePath, target)
	}
}

// targetFromSource 从源文件计算相对目标路径
// 例如: 源目录 /src，源文件 /src/docs/a.md → target = docs/a.md
// 若源文件与源目录相同（单文件规则）→ target = 文件名
func targetFromSource(source string, rule *WatchRule, _ string) string {
	srcSlash := filepath.ToSlash(source)
	for _, src := range rule.Sources {
		ruleSrcSlash := filepath.ToSlash(src)
		// 精确匹配：source == rule source（单文件规则）
		if srcSlash == ruleSrcSlash {
			return filepath.Base(source)
		}
		// 带边界的前缀匹配：避免 /tmp/docs 错误匹配 /tmp/docsfile
		if strings.HasPrefix(srcSlash, ruleSrcSlash+"/") {
			rel, err := filepath.Rel(src, source)
			if err == nil && !isOutsideRel(rel) {
				return rel
			}
		}
		// 兜底：直接使用 filepath.Rel
		rel, err := filepath.Rel(src, source)
		if err == nil && !isOutsideRel(rel) {
			return rel
		}
	}
	// fallback: 仅文件名
	return filepath.Base(source)
}

// isOutsideRel 判断 rel 是否指向 base 外部
func isOutsideRel(rel string) bool {
	return rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
		strings.HasPrefix(rel, "../")
}
