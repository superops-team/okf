package okf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// =============================================================================
// Watch Configuration (Phase 4.1)
// =============================================================================

// WatchRule 定义单条监听规则
//
// 示例 .watch.yaml：
//
//	version: 1
//	knowledgeDir: ./.kb
//	rules:
//	  - name: docs
//	    sources:
//	      - ./docs
//	    strategy: merge
//	    patterns:
//	      - "**/*.md"
//	    debounce: 500ms
//	    patchFields:
//	      - title
//	      - tags
type WatchRule struct {
	// Name 规则名称（用于日志输出与错误定位）
	Name string `yaml:"name"`

	// Sources 监听的源目录/文件绝对或相对路径列表
	// 相对路径以 .watch.yaml 所在目录为根
	Sources []string `yaml:"sources"`

	// Strategy 变更检测命中后使用的合并策略
	// skip | overwrite | merge | patch
	Strategy MergeStrategy `yaml:"strategy"`

	// Patterns 文件匹配模式（支持 glob "**/*.md"）
	// 空列表默认 ["**/*.md"]
	Patterns []string `yaml:"patterns,omitempty"`

	// Debounce 事件防抖时间（短时间内的多个写入事件合并为一次处理）
	// 默认 300ms
	Debounce time.Duration `yaml:"debounce,omitempty"`

	// PatchFields 当 Strategy=patch 时更新的字段
	PatchFields []string `yaml:"patchFields,omitempty"`
}

// WatchConfig .watch.yaml 的顶层结构
type WatchConfig struct {
	// Version 配置文件格式版本
	Version int `yaml:"version"`

	// KnowledgeDir 知识库目录（相对或绝对路径）
	KnowledgeDir string `yaml:"knowledgeDir"`

	// Debounce 全局默认防抖时间
	Debounce time.Duration `yaml:"debounce,omitempty"`

	// Rules 监听规则列表
	Rules []WatchRule `yaml:"rules"`
}

// LoadWatchConfig 从磁盘读取并解析 .watch.yaml
// path 可以是文件或目录（目录时自动追加 DefaultWatchConfigFilename）
func LoadWatchConfig(path string) (*WatchConfig, error) {
	if path == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat watch config path: %w", err)
	}

	// 如果是目录，尝试读取目录下的 DefaultWatchConfigFilename
	var cfgPath string
	if info.IsDir() {
		cfgPath = filepath.Join(path, DefaultWatchConfigFilename)
	} else {
		cfgPath = path
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("read watch config: %w", err)
	}

	var cfg WatchConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse watch config: %w", err)
	}

	// 规范化
	baseDir := filepath.Dir(cfgPath)
	if err := cfg.normalize(baseDir); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// normalize 规范化配置：设置默认值，解析相对路径，校验必选字段
func (c *WatchConfig) normalize(baseDir string) error {
	if c.Version == 0 {
		c.Version = 1
	}
	if c.Version != 1 {
		return fmt.Errorf("unsupported watch config version: %d (supported: 1)", c.Version)
	}

	if c.KnowledgeDir == "" {
		c.KnowledgeDir = filepath.Join(baseDir, ".okf", "knowledge")
	} else if !filepath.IsAbs(c.KnowledgeDir) {
		c.KnowledgeDir = filepath.Join(baseDir, c.KnowledgeDir)
	}

	if c.Debounce == 0 {
		c.Debounce = 300 * time.Millisecond
	}

	if len(c.Rules) == 0 {
		return fmt.Errorf("at least one rule is required in watch config")
	}

	for i := range c.Rules {
		r := &c.Rules[i]
		if r.Name == "" {
			r.Name = fmt.Sprintf("rule-%d", i+1)
		}
		if len(r.Sources) == 0 {
			return fmt.Errorf("rule %q: sources cannot be empty", r.Name)
		}
		// 解析相对路径为绝对路径
		for j := range r.Sources {
			if !filepath.IsAbs(r.Sources[j]) {
				r.Sources[j] = filepath.Join(baseDir, r.Sources[j])
			}
		}
		// 策略默认为 merge
		if r.Strategy == "" {
			r.Strategy = StrategyMerge
		}
		switch r.Strategy {
		case StrategySkip, StrategyOverwrite, StrategyMerge, StrategyPatch:
			// ok
		default:
			return fmt.Errorf("rule %q: invalid strategy %q", r.Name, r.Strategy)
		}
		// 默认为 **/*.md
		if len(r.Patterns) == 0 {
			r.Patterns = []string{"**/*.md"}
		}
		// 规则级 debounce
		if r.Debounce == 0 {
			r.Debounce = c.Debounce
		}
	}

	return nil
}

// MatchFilename 检查文件名是否匹配规则的任一 patterns
// filename 可以是绝对或相对路径，匹配时使用路径相对于 common 前缀
func (r *WatchRule) MatchFilename(filename string) bool {
	// 尝试每个 source 作为前缀，获取相对路径
	// 也允许直接匹配文件名的 basename
	for _, pattern := range r.Patterns {
		pattern = filepath.ToSlash(pattern)
		// 尝试：basename 匹配
		if matchGlobStar(pattern, filepath.ToSlash(filepath.Base(filename))) {
			return true
		}
		// 尝试：相对路径匹配
		for _, src := range r.Sources {
			rel, err := filepath.Rel(src, filename)
			if err == nil && rel != "" && !strings.HasPrefix(rel, "..") {
				if matchGlobStar(pattern, filepath.ToSlash(rel)) {
					return true
				}
			}
		}
		// 最后尝试：绝对路径直接匹配
		if matchGlobStar(pattern, filepath.ToSlash(filename)) {
			return true
		}
	}
	return false
}

// matchGlobStar 支持 "**" 跨目录 glob 匹配
// 语义：** 匹配任意层级目录（包括零层级）
// 其他通配符（* ? [abc]）只在单个目录段内匹配
// 示例：
//   **/*.md    → a.md, docs/a.md, x/y/a.md
//   *.md       → a.md, docs.md（但不匹配 docs/a.md）
//   docs/**    → docs/a.md, docs/sub/a.md（docs 必须是第一个 segment）
//   docs/**/*.md → docs/a.md, docs/sub/a.md
//   a?.md      → a1.md, ab.md, ax.md（a 后跟单个字符 + .md）
func matchGlobStar(pattern, path string) bool {
	pattern = filepath.ToSlash(pattern)
	path = filepath.ToSlash(path)

	// 将 pattern 按 "**" 切分，保留 "**" 作为独立 token
	// 例如 "docs/**/*.md" → ["docs", "**", "*.md"]
	tokens := splitGlobPattern(pattern)

	// 将 path 按 "/" 切分
	segments := splitPathSegments(path)

	return matchTokens(tokens, segments, 0, 0)
}

// splitGlobPattern 将 "a/**/b/*.md" 切分为 ["a", "**", "b", "*.md"]
func splitGlobPattern(p string) []string {
	parts := make([]string, 0)
	segStart := 0
	for i := 0; i < len(p); {
		if i+1 < len(p) && p[i] == '*' && p[i+1] == '*' {
			// "**" 前的 segment
			if i > segStart {
				parts = append(parts, p[segStart:i])
			}
			// "**" 前后是 "/" 或位于开头/结尾 → 作为独立 token
			prevIsBoundary := (i == 0) || p[i-1] == '/'
			nextIsBoundary := (i+2 == len(p)) || p[i+2] == '/'
			if prevIsBoundary && nextIsBoundary {
				parts = append(parts, "**")
				i += 2
				// 跳过紧随的 "/"
				if i < len(p) && p[i] == '/' {
					i++
				}
				segStart = i
				continue
			}
			// 不是独立 "**"（如 **.md 或 a**b）→ 按字面量继续
			i += 2
			continue
		}
		if p[i] == '/' {
			if i > segStart {
				parts = append(parts, p[segStart:i])
			}
			i++
			segStart = i
			continue
		}
		i++
	}
	if segStart < len(p) {
		parts = append(parts, p[segStart:])
	}
	return parts
}

// splitPathSegments 将 "a/b/c.md" 切分为 ["a", "b", "c.md"]
func splitPathSegments(path string) []string {
	if path == "" {
		return []string{""}
	}
	// 去掉首尾斜杠
	path = strings.Trim(path, "/")
	if path == "" {
		return []string{}
	}
	return strings.Split(path, "/")
}

// matchTokens 递归匹配 token 与路径段
// tokens[i] == "**" 匹配 0..N 个路径段
// 否则用 filepath.Match(tokens[i], segments[j]) 匹配单个段
func matchTokens(tokens, segments []string, ti, si int) bool {
	// 两个序列都耗尽 → 成功
	if ti == len(tokens) && si == len(segments) {
		return true
	}

	// tokens 耗尽，路径还有剩余 → 失败（除非最后一个 token 是 "**"）
	if ti == len(tokens) {
		return false
	}

	tok := tokens[ti]

	// "**" token：匹配 0 个或多个路径段
	if tok == "**" {
		// 尝试跳过 0..N 个段
		for skip := 0; si+skip <= len(segments); skip++ {
			if matchTokens(tokens, segments, ti+1, si+skip) {
				return true
			}
		}
		return false
	}

	// 普通 token：必须匹配当前段
	if si >= len(segments) {
		return false
	}
	ok, err := filepath.Match(tok, segments[si])
	if err == nil && ok {
		return matchTokens(tokens, segments, ti+1, si+1)
	}
	return false
}
