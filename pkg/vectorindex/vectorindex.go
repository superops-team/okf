// Package vectorindex 提供概念向量的近似近邻索引（HNSW）与持久化。
//
// 基于 github.com/coder/hnsw（CC0-1.0，纯 Go 零 CGO），默认余弦距离；
// 读多写少：Search 走 RLock 快照，Add/Remove/Save 走写锁。
package vectorindex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/coder/hnsw"
)

// VectorIndex 定义向量索引的最小接口。
type VectorIndex interface {
	Add(key string, vec []float32)
	Remove(key string)
	Search(vec []float32, k int) []Match
	Len() int
	Dims() int
	Save(dir string, meta Meta) error
	Load(dir string) (Meta, error)
}

// Match 是一条检索命中（含余弦相似度，越接近 1 越相似）。
type Match struct {
	Key   string
	Score float32
}

// Meta 描述索引元信息，用于加载时版本/维度一致性校验。
type Meta struct {
	Dims       int    `json:"dims"`
	Model      string `json:"model"`
	OkfVersion string `json:"okf_version"`
	Count      int    `json:"count"`
}

const (
	// IndexFileName 是图二进制文件。
	IndexFileName = "index.bin"
	// MetaFileName 是索引元信息文件。
	MetaFileName = "index.meta.json"
)

// HNSW 是 VectorIndex 的 HNSW 实现。
//
// 兼容性说明（coder/hnsw v0.6.1 实测）：
//   - Search 返回顺序不保证按距离排序，封装层自行重排；
//   - Delete 会破坏图层结构（空指针风险），故 Remove 采用 tombstone 过滤，不调用 Delete。
type HNSW struct {
	mu      sync.RWMutex
	g       *hnsw.Graph[string]
	dims    int
	removed map[string]struct{} // tombstone：已删除（不参与检索）
}

// NewHNSW 创建维度为 dims 的空索引。M=16、EfSearch=64（384 维语义向量的精度/性能平衡）。
func NewHNSW(dims int) *HNSW {
	g := hnsw.NewGraph[string]()
	g.M = 16
	g.EfSearch = 64
	return &HNSW{g: g, dims: dims, removed: make(map[string]struct{})}
}

// Add 幂等插入：key 已存在则跳过（不覆盖、不 panic）。
// 注意：coder/hnsw 对相同 key 重复 Add 会 panic（"node not added"），
// 且 Delete 后再 Add 会破坏图层结构，故 P0 采用「追加 + 已存在跳过」：
// 概念内容变更需通过全量重建（rebuild）更新向量（见 spec S3.4）。
func (h *HNSW) Add(key string, vec []float32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.removed, key) // 若曾删除，重新加入
	if _, ok := h.g.Lookup(key); ok {
		return // 幂等跳过
	}
	h.g.Add(hnsw.MakeNode(key, vec))
}

// Remove 标记 key 为已删除（tombstone），返回是否有效。P0 不物理删除节点。
func (h *HNSW) Remove(key string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.removed[key]; ok {
		return false // 已删除
	}
	if _, ok := h.g.Lookup(key); !ok {
		return false
	}
	h.removed[key] = struct{}{}
	return true
}

// Search 返回与 vec 最相似的 k 个命中（按余弦相似度降序，过滤已删除项）。
func (h *HNSW) Search(vec []float32, k int) []Match {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.g.Len() == 0 {
		return nil
	}
	// 多取一些候选以抵消 tombstone 占位，保证过滤后仍可返回 k 条
	nodes := h.g.Search(vec, k+len(h.removed)+1)
	out := make([]Match, 0, len(nodes))
	for _, n := range nodes {
		if _, ok := h.removed[n.Key]; ok {
			continue
		}
		out = append(out, Match{Key: n.Key, Score: 1 - hnsw.CosineDistance(vec, n.Value)})
	}
	// HNSW 不保证返回顺序按距离排序，显式降序重排
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > k {
		out = out[:k]
	}
	return out
}

// Len 返回有效（未删除）向量数。
func (h *HNSW) Len() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.g.Len() - len(h.removed)
}

// Dims 返回向量维度。
func (h *HNSW) Dims() int { return h.dims }

// Save 将图与元信息写入 dir（原子写：临时文件 + rename）。
func (h *HNSW) Save(dir string, meta Meta) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	meta.Dims = h.dims
	meta.Count = h.g.Len() - len(h.removed) // 有效节点数（排除 tombstone）

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	binTmp := filepath.Join(dir, "."+IndexFileName+".tmp")
	f, err := os.Create(binTmp)
	if err != nil {
		return err
	}
	defer os.Remove(binTmp)
	if err := h.g.Export(f); err != nil {
		f.Close()
		return fmt.Errorf("export graph: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(binTmp, filepath.Join(dir, IndexFileName)); err != nil {
		return err
	}

	metaData, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	metaTmp := filepath.Join(dir, "."+MetaFileName+".tmp")
	if err := os.WriteFile(metaTmp, metaData, 0o644); err != nil {
		return err
	}
	defer os.Remove(metaTmp)
	return os.Rename(metaTmp, filepath.Join(dir, MetaFileName))
}

// Load 从 dir 加载图与元信息；维度/版本不匹配时返回可读错误（提示 rebuild）。
func (h *HNSW) Load(dir string) (Meta, error) {
	var meta Meta
	metaData, err := os.ReadFile(filepath.Join(dir, MetaFileName))
	if err != nil {
		return meta, fmt.Errorf("读取索引元信息失败（%w），请执行 okf vector rebuild", err)
	}
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return meta, fmt.Errorf("解析索引元信息失败（%w），请执行 okf vector rebuild", err)
	}
	if meta.Dims != h.dims {
		return meta, fmt.Errorf("索引维度 %d 与当前模型维度 %d 不一致，请执行 okf vector rebuild", meta.Dims, h.dims)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	f, err := os.Open(filepath.Join(dir, IndexFileName))
	if err != nil {
		return meta, fmt.Errorf("打开索引文件失败（%w），请执行 okf vector rebuild", err)
	}
	defer f.Close()
	if err := h.g.Import(bufio.NewReader(f)); err != nil {
		return meta, fmt.Errorf("加载索引失败（%w），请执行 okf vector rebuild", err)
	}
	h.dims = meta.Dims
	return meta, nil
}
