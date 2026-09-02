// Package vectorindex 提供概念向量的近似近邻索引（HNSW）与持久化。
//
// 基于 github.com/coder/hnsw（CC0-1.0，纯 Go 零 CGO），默认余弦距离；
// 读多写少：Search 走 RLock 快照，Add/Remove/Save 走写锁。
package vectorindex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand"
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

// CurrentIndexFormatVersion 是当前索引格式版本。
//
// 版本 2 起索引以 chunk 为单位，key 形如 "<概念指纹>#<块序号>"；
// 版本 1（或缺失该字段）为概念级 key，二者不兼容：
// 用旧索引检索会因 key 无法回溯到概念而静默返回空结果，
// 故加载时必须显式拒绝并提示 rebuild。
const CurrentIndexFormatVersion = 2

// Meta 描述索引元信息，用于加载时版本/维度一致性校验。
type Meta struct {
	Dims       int    `json:"dims"`
	Model      string `json:"model"`
	OkfVersion string `json:"okf_version"`
	Count      int    `json:"count"`
	// IndexFormatVersion 是索引结构版本，见 CurrentIndexFormatVersion。
	IndexFormatVersion int `json:"index_format_version"`
	// Concepts 是索引覆盖的概念数（chunk 的父概念去重计数），用于 status 展示。
	Concepts int `json:"concepts,omitempty"`
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

// indexRngSeed 是 HNSW 层级生成的固定随机种子。
//
// coder/hnsw 默认以 rand.NewSource(time.Now().UnixNano()) 播种（graph.go:defaultRand），
// 会导致同一份数据每次建索引得到不同图结构：检索序列在多次 rebuild 之间漂移，
// 检索结果与评测指标都不可复现。库文档明确允许固定该值以获得可复现性
// （"Rng ... may be set to a deterministic value for reproducibility"）。
const indexRngSeed = 42

// 检索候选放大参数。
//
// HNSW 是近似检索，且 coder/hnsw 的入口点选择依赖 map 遍历顺序，
// 召回池偏小时尾部结果会在多次运行间漂移（实测：k=10 取 40 候选仍漂移）。
// 因此封装层多取候选后精确重排：
//   - 索引规模 ≤ searchExactMaxNodes 时全量召回，等价精确检索；
//   - 超过该规模时取 k*Factor 且不低于 Floor，兼顾稳定性与性能。
const (
	searchOversampleFactor = 8
	searchOversampleFloor  = 128
	searchExactMaxNodes    = 2048
)

// NewHNSW 创建维度为 dims 的空索引。M=16、EfSearch=64（384 维语义向量的精度/性能平衡）。
// Rng 固定种子以保证索引构建与检索确定可复现。
func NewHNSW(dims int) *HNSW {
	g := hnsw.NewGraph[string]()
	g.M = 16
	g.EfSearch = 64
	g.Rng = rand.New(rand.NewSource(indexRngSeed))
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
//
// 确定性保证：coder/hnsw 的 layer.entry() 以 map 随机遍历选取搜索入口
// （graph.go:198，注释自称 consistent 但实际依赖 Go map 顺序），
// 导致同图多次检索的尾部近似结果可能漂移。为使结果可复现，封装层：
//  1. 向底层多取候选（oversample），扩大召回池以覆盖漂移范围；
//  2. 对候选用完整余弦相似度精算后重排；
//  3. 同分时按 key 升序 tie-break，消除 map 顺序影响。
func (h *HNSW) Search(vec []float32, k int) []Match {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.g.Len() == 0 || k <= 0 {
		return nil
	}
	// 候选池规模：小索引全量召回（等价精确检索），大索引按倍数放大。
	// 实测：召回池不足会使尾部结果在多次 rebuild 间漂移（分数互异，属召回缺失而非排序不稳）。
	total := h.g.Len()
	want := total
	if total > searchExactMaxNodes {
		want = k*searchOversampleFactor + len(h.removed)
		if want < searchOversampleFloor {
			want = searchOversampleFloor
		}
		if want > total {
			want = total
		}
	}
	nodes := h.g.Search(vec, want)
	out := make([]Match, 0, len(nodes))
	for _, n := range nodes {
		if _, ok := h.removed[n.Key]; ok {
			continue
		}
		out = append(out, Match{Key: n.Key, Score: 1 - hnsw.CosineDistance(vec, n.Value)})
	}
	// 精确重排：分数降序；同分按 key 升序，保证顺序不依赖 map 遍历。
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Key < out[j].Key
	})
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
	meta.IndexFormatVersion = CurrentIndexFormatVersion

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
	// 索引格式版本校验先于维度校验：格式不兼容时 key 语义已变，
	// 即使维度相同也不能使用（会静默返回空结果）。
	if meta.IndexFormatVersion != CurrentIndexFormatVersion {
		return meta, fmt.Errorf(
			"索引格式版本 %d 与当前版本 %d 不兼容（分块级索引），请执行 okf vector rebuild",
			meta.IndexFormatVersion, CurrentIndexFormatVersion)
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
