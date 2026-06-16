package okf

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
)

// =============================================================================
// 合并结果
// =============================================================================

// MergeResult 合并操作的执行结果
type MergeResult struct {
	Strategy MergeStrategy
	Content  []byte
	Changed  bool
}

// =============================================================================
// Frontmatter 解析与序列化
// =============================================================================

// ParseFrontmatter 解析 markdown 字节切片，分离 frontmatter 与正文
// 返回 (frontmatter map, body string, hasFrontmatter bool)
// 简化策略：仅支持一层的 key: value 与 tags 列表，复杂 YAML 由
// 后续版本通过 yaml.v3 库支持。第一期只覆盖 OKF 必需字段。
func ParseFrontmatter(content []byte) (map[string]interface{}, string, bool) {
	fm := make(map[string]interface{})

	// 必须以 "---\n" 或 "---\r\n" 开头
	if !bytes.HasPrefix(content, []byte("---\n")) &&
		!bytes.HasPrefix(content, []byte("---\r\n")) {
		return nil, string(content), false
	}

	// 跳过起始 "---" 标记
	rest := content[4:]
	if bytes.HasPrefix(content, []byte("---\r\n")) {
		rest = content[5:]
	}

	// 查找结束 "---"
	endIdx := bytes.Index(rest, []byte("\n---"))
	if endIdx < 0 {
		return nil, string(content), false
	}

	fmBlock := rest[:endIdx]
	body := rest[endIdx+4:]
	// 移除 body 开头的换行
	body = bytes.TrimLeft(body, "\r\n")

	// 逐行解析
	scanner := bufio.NewScanner(bytes.NewReader(fmBlock))
	var currentList string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// 列表项
		if strings.HasPrefix(line, "- ") {
			value := strings.TrimPrefix(line, "- ")
			if currentList != "" {
				if list, ok := fm[currentList].([]string); ok {
					fm[currentList] = append(list, value)
				}
			}
			continue
		}

		// key: value
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"'")

		if value == "" {
			// 列表开始
			currentList = key
			fm[key] = []string{}
		} else {
			currentList = ""
			fm[key] = value
		}
	}

	return fm, string(body), true
}

// SerializeFrontmatter 将 frontmatter map 与 body 合并为完整 markdown
func SerializeFrontmatter(fm map[string]interface{}, body string) []byte {
	var buf bytes.Buffer
	buf.WriteString("---\n")

	// 保持稳定输出：按 key 排序
	keys := make([]string, 0, len(fm))
	for k := range fm {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := fm[k]
		switch val := v.(type) {
		case []string:
			if len(val) == 0 {
				buf.WriteString(k + ":\n")
			} else {
				buf.WriteString(k + ":\n")
				for _, item := range val {
					buf.WriteString("  - " + item + "\n")
				}
			}
		default:
			buf.WriteString(k + ": " + fmt.Sprint(val) + "\n")
		}
	}

	buf.WriteString("---\n")
	if body != "" {
		buf.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			buf.WriteString("\n")
		}
	}
	return buf.Bytes()
}

// =============================================================================
// 策略决策
// =============================================================================

// SmartImportOptions 智能导入选项
type SmartImportOptions struct {
	// ForceStrategy 强制使用的合并策略（优先级最高）
	ForceStrategy MergeStrategy

	// PatchFields patch 模式要更新的字段
	PatchFields []string

	// DetectOnly 仅检测变更，不执行
	DetectOnly bool

	// HashOnly 仅计算 hash（不执行导入）
	HashOnly bool
}

// DefaultSmartImportOptions 返回默认选项
func DefaultSmartImportOptions() *SmartImportOptions {
	return &SmartImportOptions{}
}

// DecideStrategy 决策使用哪种合并策略
// 优先级：ForceStrategy > 已记录 Strategy > 默认 skip
func DecideStrategy(meta *FileMetadata, opts *SmartImportOptions) MergeStrategy {
	if opts != nil && opts.ForceStrategy != "" {
		return opts.ForceStrategy
	}
	if meta != nil && meta.Strategy != "" {
		return meta.Strategy
	}
	return StrategySkip
}

// resolvePatchFields 解析最终的 patch 字段列表
// 优先级：opts.PatchFields > meta.PatchFields > 默认值
func resolvePatchFields(opts *SmartImportOptions, meta *FileMetadata) []string {
	if opts != nil && len(opts.PatchFields) > 0 {
		return opts.PatchFields
	}
	if meta != nil && len(meta.PatchFields) > 0 {
		return meta.PatchFields
	}
	return DefaultPatchFields
}

// =============================================================================
// 合并策略实现
// =============================================================================

// DoSkip 跳过策略：目标存在则跳过，否则导入
func DoSkip(source, target string, meta *FileMetadata) (*MergeResult, error) {
	if _, err := os.Stat(target); err == nil {
		// 目标已存在 → 跳过
		return &MergeResult{Strategy: StrategySkip, Changed: false}, nil
	}

	// 目标不存在 → 导入
	content, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("read source: %w", err)
	}
	if err := os.MkdirAll(parentDir(target), 0755); err != nil {
		return nil, fmt.Errorf("create target dir: %w", err)
	}
	if err := AtomicWriteFile(target, content, 0644); err != nil {
		return nil, fmt.Errorf("write target: %w", err)
	}
	return &MergeResult{Strategy: StrategySkip, Content: content, Changed: true}, nil
}

// DoOverwrite 覆盖策略：用源文件覆盖目标
func DoOverwrite(source, target string, meta *FileMetadata) (*MergeResult, error) {
	content, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("read source: %w", err)
	}
	if err := os.MkdirAll(parentDir(target), 0755); err != nil {
		return nil, fmt.Errorf("create target dir: %w", err)
	}
	if err := AtomicWriteFile(target, content, 0644); err != nil {
		return nil, fmt.Errorf("write target: %w", err)
	}
	return &MergeResult{Strategy: StrategyOverwrite, Content: content, Changed: true}, nil
}

// DoMerge 合并策略：
//   - frontmatter 合并：保留目标已有字段值，tags 取并集
//   - 正文保留：目标文件的 body 保持不变
func DoMerge(source, target string, meta *FileMetadata) (*MergeResult, error) {
	srcData, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("read source: %w", err)
	}
	tgtData, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("read target: %w", err)
	}

	srcFM, _, srcHasFM := ParseFrontmatter(srcData)
	tgtFM, tgtBody, tgtHasFM := ParseFrontmatter(tgtData)

	// 合并 frontmatter
	mergedFM := make(map[string]interface{})

	// 先写入目标已有字段（保留）
	if tgtHasFM {
		for k, v := range tgtFM {
			mergedFM[k] = v
		}
	}

	// 处理源新增字段 + tags 合并
	if srcHasFM {
		for k, v := range srcFM {
			if k == "tags" {
				mergedFM[k] = mergeTags(mergedFM["tags"], v)
				continue
			}
			// 其他字段：若目标已有则保留目标值
			if _, exists := mergedFM[k]; !exists {
				mergedFM[k] = v
			}
		}
	}

	// 若两边都没有 frontmatter，直接保留 target 内容
	if !srcHasFM && !tgtHasFM {
		// 内容已相同则 Changed=false
		if bytes.Equal(srcData, tgtData) {
			return &MergeResult{Strategy: StrategyMerge, Changed: false}, nil
		}
		if err := AtomicWriteFile(target, tgtData, 0644); err != nil {
			return nil, err
		}
		return &MergeResult{Strategy: StrategyMerge, Content: tgtData, Changed: true}, nil
	}

	merged := SerializeFrontmatter(mergedFM, tgtBody)

	if bytes.Equal(merged, tgtData) {
		return &MergeResult{Strategy: StrategyMerge, Content: merged, Changed: false}, nil
	}

	if err := AtomicWriteFile(target, merged, 0644); err != nil {
		return nil, fmt.Errorf("write merged: %w", err)
	}
	return &MergeResult{Strategy: StrategyMerge, Content: merged, Changed: true}, nil
}

// DoPatch patch 策略：仅更新指定 frontmatter 字段
func DoPatch(source, target string, meta *FileMetadata) (*MergeResult, error) {
	opts := DefaultSmartImportOptions()
	return DoPatchWithOpts(source, target, meta, opts)
}

// DoPatchWithOpts 带选项的 patch 策略
func DoPatchWithOpts(source, target string, meta *FileMetadata, opts *SmartImportOptions) (*MergeResult, error) {
	fields := resolvePatchFields(opts, meta)

	srcData, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("read source: %w", err)
	}
	tgtData, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("read target: %w", err)
	}

	srcFM, _, srcHasFM := ParseFrontmatter(srcData)
	tgtFM, tgtBody, tgtHasFM := ParseFrontmatter(tgtData)

	// 起始以目标的 frontmatter 为基础
	resultFM := make(map[string]interface{})
	if tgtHasFM {
		for k, v := range tgtFM {
			resultFM[k] = v
		}
	}

	// 覆盖指定字段
	if srcHasFM {
		for _, field := range fields {
			if v, ok := srcFM[field]; ok {
				resultFM[field] = v
			}
		}
	}

	// 没有任何 frontmatter
	if !srcHasFM && !tgtHasFM {
		return &MergeResult{Strategy: StrategyPatch, Changed: false}, nil
	}

	patched := SerializeFrontmatter(resultFM, tgtBody)

	if bytes.Equal(patched, tgtData) {
		return &MergeResult{Strategy: StrategyPatch, Content: patched, Changed: false}, nil
	}

	if err := AtomicWriteFile(target, patched, 0644); err != nil {
		return nil, fmt.Errorf("write patched: %w", err)
	}
	return &MergeResult{Strategy: StrategyPatch, Content: patched, Changed: true}, nil
}

// =============================================================================
// 内部辅助
// =============================================================================

// mergeTags 合并两个 tag 集合并去重
func mergeTags(existing, incoming interface{}) []string {
	tagSet := make(map[string]struct{})
	addTags := func(v interface{}) {
		switch list := v.(type) {
		case []string:
			for _, t := range list {
				tagSet[t] = struct{}{}
			}
		case string:
			if list != "" {
				tagSet[list] = struct{}{}
			}
		}
	}
	addTags(existing)
	addTags(incoming)
	result := make([]string, 0, len(tagSet))
	for t := range tagSet {
		result = append(result, t)
	}
	sort.Strings(result)
	return result
}

// parentDir 返回 path 的父目录
func parentDir(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		idx = strings.LastIndex(path, "\\")
	}
	if idx < 0 {
		return "."
	}
	return path[:idx]
}
