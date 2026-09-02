package vectorindex

import (
	"math"
	"math/rand"
	"testing"
)

// 确定性回归：coder/hnsw 默认以 time.Now().UnixNano() 播种（graph.go:defaultRand），
// 若封装层不显式固定 Rng，则同一份数据两次建索引会得到不同图结构，
// 检索序列在多次 rebuild 之间漂移，评测数字不可复现。
// 本组测试固定该行为，防止回归。

// buildIdx 以固定顺序插入同一批向量，返回索引。
func buildIdx(t *testing.T, dims, n int) *HNSW {
	t.Helper()
	idx := NewHNSW(dims)
	// 用固定种子生成可复现的测试向量（与索引内部 Rng 无关）
	r := rand.New(rand.NewSource(7))
	for i := 0; i < n; i++ {
		vec := make([]float32, dims)
		for j := range vec {
			vec[j] = float32(r.NormFloat64())
		}
		idx.Add(keyOf(i), vec)
	}
	return idx
}

func keyOf(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "k0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{digits[i%10]}, b...)
		i /= 10
	}
	return "k" + string(b)
}

// queryVec 生成确定的查询向量。
func queryVec(dims int) []float32 {
	r := rand.New(rand.NewSource(99))
	v := make([]float32, dims)
	for i := range v {
		v[i] = float32(r.NormFloat64())
	}
	return v
}

func TestSearchIsDeterministicAcrossRebuilds(t *testing.T) {
	const dims, n, k = 16, 60, 10
	q := queryVec(dims)

	first := buildIdx(t, dims, n).Search(q, k)
	if len(first) != k {
		t.Fatalf("首次检索返回 %d 条，期望 %d", len(first), k)
	}

	// 重复多轮：每轮都是全新的图（等价于 okf vector rebuild）
	for round := 0; round < 5; round++ {
		got := buildIdx(t, dims, n).Search(q, k)
		if len(got) != len(first) {
			t.Fatalf("第 %d 轮返回 %d 条，首轮 %d 条", round, len(got), len(first))
		}
		for i := range got {
			if got[i].Key != first[i].Key {
				t.Fatalf("第 %d 轮 rank %d = %q，首轮为 %q（索引构建非确定性）",
					round, i, got[i].Key, first[i].Key)
			}
			if math.Abs(float64(got[i].Score-first[i].Score)) > 1e-9 {
				t.Fatalf("第 %d 轮 rank %d 分数 %v，首轮 %v（分数非确定性）",
					round, i, got[i].Score, first[i].Score)
			}
		}
	}
}

// 同一个索引实例内多次检索也必须稳定（排除 Search 自身的不确定性）。
func TestSearchIsStableOnSameIndex(t *testing.T) {
	const dims, n, k = 16, 40, 8
	idx := buildIdx(t, dims, n)
	q := queryVec(dims)

	first := idx.Search(q, k)
	for round := 0; round < 3; round++ {
		got := idx.Search(q, k)
		for i := range got {
			if got[i].Key != first[i].Key {
				t.Fatalf("同实例第 %d 轮 rank %d = %q，首轮 %q", round, i, got[i].Key, first[i].Key)
			}
		}
	}
}

// Save/Load 往返后检索序列必须与原索引一致（持久化不引入漂移）。
func TestSearchDeterministicAfterReload(t *testing.T) {
	const dims, n, k = 16, 50, 10
	idx := buildIdx(t, dims, n)
	q := queryVec(dims)
	want := idx.Search(q, k)

	dir := t.TempDir()
	if err := idx.Save(dir, Meta{Model: "test"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reloaded := NewHNSW(dims)
	if _, err := reloaded.Load(dir); err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := reloaded.Search(q, k)
	if len(got) != len(want) {
		t.Fatalf("重载后返回 %d 条，期望 %d", len(got), len(want))
	}
	for i := range got {
		if got[i].Key != want[i].Key {
			t.Fatalf("重载后 rank %d = %q，期望 %q", i, got[i].Key, want[i].Key)
		}
	}
}

// 超过 searchExactMaxNodes 的索引走 oversample 路径，同样必须稳定。
func TestSearchDeterministicOnLargeIndex(t *testing.T) {
	const dims, n, k = 32, searchExactMaxNodes + 500, 10
	q := queryVec(dims)

	first := buildIdx(t, dims, n).Search(q, k)
	if len(first) != k {
		t.Fatalf("返回 %d 条，期望 %d", len(first), k)
	}
	// 同一实例重复检索必须稳定（大索引重建成本高，此处验证检索侧确定性）
	idx := buildIdx(t, dims, n)
	base := idx.Search(q, k)
	for round := 0; round < 3; round++ {
		got := idx.Search(q, k)
		for i := range got {
			if got[i].Key != base[i].Key {
				t.Fatalf("大索引第 %d 轮 rank %d = %q，首轮 %q", round, i, got[i].Key, base[i].Key)
			}
		}
	}
}
