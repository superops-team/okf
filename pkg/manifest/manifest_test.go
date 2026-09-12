package manifest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeKnowledgeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func goodFM(typ, title, extra string) string {
	return "---\ntype: " + typ + "\ntitle: " + title + "\n" + extra + "---\nbody " + title + "\n"
}

func TestManifestDefaults(t *testing.T) {
	root := t.TempDir()
	// Intentionally out of lexical order on disk to prove deterministic sort.
	writeKnowledgeFile(t, root, "concepts/zeta.md", goodFM("concept", "Zeta", ""))
	writeKnowledgeFile(t, root, "concepts/alpha.md", goodFM("source", "Alpha", ""))
	writeKnowledgeFile(t, root, "concepts/middle.md", goodFM("concept", "Middle", ""))

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	res, err := Build(context.Background(), root, filepath.Join(root, ".okf", "vector"), ManifestRequest{}, nil, now, false)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if res.Offset != 0 {
		t.Fatalf("offset = %d, want 0", res.Offset)
	}
	if res.Limit != defaultLimit {
		t.Fatalf("limit = %d, want default %d", res.Limit, defaultLimit)
	}
	if res.Total != 3 || len(res.Items) != 3 {
		t.Fatalf("total = %d, items = %d", res.Total, len(res.Items))
	}
	wantOrder := []string{"concepts/alpha.md", "concepts/middle.md", "concepts/zeta.md"}
	for i, item := range res.Items {
		if item.Path != wantOrder[i] {
			t.Fatalf("item %d path = %q, want %q (order must be path ASC)", i, item.Path, wantOrder[i])
		}
	}
	// Default index status is missing (no vector dir).
	if res.IndexStatus != IndexMissing {
		t.Fatalf("index status = %q, want missing", res.IndexStatus)
	}
}

func TestManifestPaginationValidation(t *testing.T) {
	root := t.TempDir()
	writeKnowledgeFile(t, root, "a.md", goodFM("concept", "A", ""))
	now := time.Now()

	// Negative offset.
	if _, err := Build(context.Background(), root, "", ManifestRequest{Offset: -1}, nil, now, false); err == nil {
		t.Fatal("negative offset should fail")
	} else if me, ok := err.(*Error); !ok || me.Code != "invalid_request" {
		t.Fatalf("err = %v, want invalid_request", err)
	}

	// Explicit limit out of range (too small / too large).
	for _, l := range []int{0, -5, 501, 1000} {
		l := l
		if _, err := Build(context.Background(), root, "", ManifestRequest{Limit: &l}, nil, now, false); err == nil {
			t.Fatalf("explicit limit %d should fail", l)
		}
	}

	// Explicit valid limit is honored; omitted limit defaults to 100.
	limit := 2
	res, err := Build(context.Background(), root, "", ManifestRequest{Limit: &limit}, nil, now, false)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if res.Limit != 2 {
		t.Fatalf("limit = %d, want 2", res.Limit)
	}

	// Presence semantics: nil pointer means omitted (→100), even if a zero value
	// is the Go default.
	omitted, err := Build(context.Background(), root, "", ManifestRequest{}, nil, now, false)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if omitted.Limit != defaultLimit {
		t.Fatalf("omitted limit resolved to %d, want default %d", omitted.Limit, defaultLimit)
	}
}

func TestManifestFilters(t *testing.T) {
	root := t.TempDir()
	writeKnowledgeFile(t, root, "concepts/note.md", goodFM("note", "Note", "tags: [alpha, beta]\nstatus: draft\n"))
	writeKnowledgeFile(t, root, "concepts/source.md", goodFM("source", "Source", "tags: [beta]\n"))
	writeKnowledgeFile(t, root, "concepts/deprecated.md", goodFM("source", "Deprecated", "status: deprecated\n"))
	now := time.Now()

	// Types OR within dimension: note OR source.
	res, err := Build(context.Background(), root, "", ManifestRequest{Types: []string{"note", "source"}}, nil, now, false)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if res.Total != 3 {
		t.Fatalf("types OR total = %d, want 3", res.Total)
	}

	// Tags: beta matches both note and source (OR within tags list).
	res, _ = Build(context.Background(), root, "", ManifestRequest{Tags: []string{"alpha"}}, nil, now, false)
	if res.Total != 1 || res.Items[0].Path != "concepts/note.md" {
		t.Fatalf("tags alpha = %+v", res.Items)
	}

	// Statuses OR; missing status exposes as stable. The plain source.md has no
	// status → effective stable.
	res, _ = Build(context.Background(), root, "", ManifestRequest{Statuses: []string{"stable"}}, nil, now, false)
	if res.Total != 1 || res.Items[0].Path != "concepts/source.md" {
		t.Fatalf("status stable = %+v", res.Items)
	}
	// AND across dimensions: type=source AND status=stable.
	res, _ = Build(context.Background(), root, "", ManifestRequest{Types: []string{"source"}, Statuses: []string{"stable"}}, nil, now, false)
	if res.Total != 1 || res.Items[0].Path != "concepts/source.md" {
		t.Fatalf("type+status AND = %+v", res.Items)
	}
	// Effective status exposed on the item for a status-less file.
	if res.Items[0].Status != "stable" {
		t.Fatalf("effective status = %q, want stable", res.Items[0].Status)
	}
}

func TestManifestMetadata(t *testing.T) {
	root := t.TempDir()
	full := "---\n" +
		"type: source\n" +
		"title: Full\n" +
		"description: desc\n" +
		"tags: [a, b]\n" +
		"status: stable\n" +
		"stale_after: 2020-01-01\n" +
		"okf_id: okf_17a2c56db85c4889b4f8fe02ca9ac67e\n" +
		"generated:\n" +
		"  by: generator\n" +
		"  at: 2024-01-02T03:04:05Z\n" +
		"verified:\n" +
		"  - by: human:alice\n" +
		"    at: 2025-01-02T03:04:05Z\n" +
		"sources:\n" +
		"  - resource: refs/s1.md\n" +
		"  - resource: refs/s2.md\n" +
		"  - resource: refs/s3.md\n" +
		"  - resource: refs/s4.md\n" +
		"---\nbody content that must never surface\n"
	writeKnowledgeFile(t, root, "concepts/full.md", full)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	res, err := Build(context.Background(), root, "", ManifestRequest{}, nil, now, false)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("items = %d", len(res.Items))
	}
	item := res.Items[0]
	if item.OKFID != "okf_17a2c56db85c4889b4f8fe02ca9ac67e" || item.Ref != "okf://concept/okf_17a2c56db85c4889b4f8fe02ca9ac67e" {
		t.Fatalf("identity/ref = %q/%q", item.OKFID, item.Ref)
	}
	if item.IdentityState != "stable" {
		t.Fatalf("identity state = %q", item.IdentityState)
	}
	if item.Status != "stable" || item.TrustTier != "human-reviewed" {
		t.Fatalf("status/tier = %q/%q", item.Status, item.TrustTier)
	}
	if !item.Stale {
		t.Fatal("stale_after 2020 should be stale relative to 2026")
	}
	if item.GeneratedAt != "2024-01-02T03:04:05Z" {
		t.Fatalf("generated_at = %q", item.GeneratedAt)
	}
	if !strings.HasPrefix(item.VerifiedAt, "2025-01-02") {
		t.Fatalf("verified_at = %q", item.VerifiedAt)
	}
	if item.SourceCount != 4 {
		t.Fatalf("source_count = %d, want 4", item.SourceCount)
	}
	if len(item.SourceResources) != 3 {
		t.Fatalf("source_resources len = %d, want <=3", len(item.SourceResources))
	}
	if item.SourceResources[3-1] != "refs/s3.md" {
		t.Fatalf("source_resources = %v", item.SourceResources)
	}
	// Body must never be returned: no field can hold it.
	if strings.Contains(strings.ToLower(item.Description), "body content") {
		t.Fatal("body leaked into description")
	}
}

func TestManifestTokenEstimate(t *testing.T) {
	root := t.TempDir()
	// N is the total file size; estimated_tokens = ceil(N/4).
	for _, n := range []int64{0, 1, 4, 5, 4096, 4097} {
		content := "---\ntype: concept\ntitle: T\n---\n" + strings.Repeat("x", int(n))
		writeKnowledgeFile(t, root, "c.md", content)
		res, err := Build(context.Background(), root, "", ManifestRequest{}, nil, time.Now(), false)
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		got := res.Items[0].EstimatedTokens
		want := (int64(len(content)) + 3) / 4
		if got != want {
			t.Fatalf("size=%d tokens = %d, want ceil=%d", len(content), got, want)
		}
		if res.Items[0].EstimateKind != "file_bytes_div4" {
			t.Fatalf("estimate_kind = %q", res.Items[0].EstimateKind)
		}
		os.Remove(filepath.Join(root, "c.md"))
	}
}

func TestManifestIndexObservation(t *testing.T) {
	cases := []struct {
		name string
		mk   func(dir string)
		want IndexStatus
	}{
		{"missing", func(string) {}, IndexMissing},
		{"ready", func(dir string) {
			os.WriteFile(filepath.Join(dir, "index.meta.json"), []byte(`{"index_format_version":3,"count":1}`), 0o644)
			os.WriteFile(filepath.Join(dir, "index.bin"), []byte{0}, 0o644)
		}, IndexReady},
		{"incompatible", func(dir string) {
			os.WriteFile(filepath.Join(dir, "index.meta.json"), []byte(`{"index_format_version":2,"count":1}`), 0o644)
			os.WriteFile(filepath.Join(dir, "index.bin"), []byte{0}, 0o644)
		}, IndexIncompatible},
		{"unreadable", func(dir string) {
			os.WriteFile(filepath.Join(dir, "index.meta.json"), []byte("not json"), 0o644)
		}, IndexUnreadable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.mk(dir)
			if got := ObserveIndexStatus(dir, false); got != tc.want {
				t.Fatalf("ObserveIndexStatus = %q, want %q", got, tc.want)
			}
		})
	}
	// stale upgrades a ready index only when the caller supplies freshness.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.meta.json"), []byte(`{"index_format_version":3}`), 0o644)
	os.WriteFile(filepath.Join(dir, "index.bin"), []byte{0}, 0o644)
	if got := ObserveIndexStatus(dir, true); got != IndexStale {
		t.Fatalf("stale = %q, want stale", got)
	}

	// Build must observe status without requiring any embedder/index runtime:
	// a knowledge root with no vector dir builds cleanly.
	root := t.TempDir()
	writeKnowledgeFile(t, root, "a.md", goodFM("concept", "A", ""))
	res, err := Build(context.Background(), root, filepath.Join(root, "nope", "vector"), ManifestRequest{}, nil, time.Now(), false)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if res.IndexStatus != IndexMissing {
		t.Fatalf("index status = %q, want missing", res.IndexStatus)
	}
}

func TestManifestDuplicateIDFails(t *testing.T) {
	root := t.TempDir()
	writeKnowledgeFile(t, root, "concepts/one.md", goodFM("concept", "One", "okf_id: okf_17a2c56db85c4889b4f8fe02ca9ac67e\n"))
	// The second file reuses the same id.
	writeKnowledgeFile(t, root, "concepts/two.md", goodFM("concept", "Two", "okf_id: okf_17a2c56db85c4889b4f8fe02ca9ac67e\n"))

	_, err := Build(context.Background(), root, "", ManifestRequest{}, nil, time.Now(), false)
	if err == nil {
		t.Fatal("duplicate id should fail")
	}
	var me *Error
	if !asManifestError(err, &me) || me.Code != "duplicate_concept_id" {
		t.Fatalf("err = %v, want duplicate_concept_id", err)
	}
	if len(me.Paths) < 2 {
		t.Fatalf("error paths = %v, want both files", me.Paths)
	}
}

func asManifestError(err error, target **Error) bool {
	for err != nil {
		if e, ok := err.(*Error); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
