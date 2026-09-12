package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestCLISearchGroupedOutput (S34): the classic CLI search entry projects the
// same lexical hits through query.Project and prints equivalent group keys and
// counts. It also anchors S27: without -group-by the legacy text output is
// untouched.
func TestCLISearchGroupedOutput(t *testing.T) {
	bin := buildOKF(t)
	repo := initCLIRepo(t)

	kb := filepath.Join(repo, ".okf", "knowledge", "concepts")
	mustWriteCLIFile(t, filepath.Join(kb, "one.md"), `---
type: concept
title: Alpha Doc One
description: ParityToken first half of doc1
source_path: src/doc1.md
---
ParityToken body alpha one.
`)
	mustWriteCLIFile(t, filepath.Join(kb, "two.md"), `---
type: concept
title: Alpha Doc Two
description: ParityToken second half of doc1
source_path: src/doc1.md
---
ParityToken body alpha two.
`)
	mustWriteCLIFile(t, filepath.Join(kb, "three.md"), `---
type: concept
title: Beta Doc
description: ParityToken lives in doc2
source_path: src/doc2.md
---
ParityToken body beta.
`)

	// Ungrouped (S27): classic text output, no projection banner.
	ungrouped := runOKF(t, bin, "search", "-path", repo, "-q", "ParityToken")
	if strings.Contains(ungrouped, "Projected into") {
		t.Fatalf("ungrouped search should not print projection banner:\n%s", ungrouped)
	}
	if !strings.Contains(ungrouped, "Alpha Doc One") {
		t.Fatalf("ungrouped search missing expected result:\n%s", ungrouped)
	}

	// Grouped (S34): source projection merges doc1's two concepts.
	grouped := runOKF(t, bin, "search", "-path", repo, "-q", "ParityToken",
		"-group-by", "source", "-include-group-members")
	if !strings.Contains(grouped, "Projected into 2 groups (by source)") {
		t.Fatalf("grouped search missing projection header:\n%s", grouped)
	}
	if !strings.Contains(grouped, "key=src:src/doc1.md") || !strings.Contains(grouped, "hits=2") {
		t.Fatalf("grouped search missing doc1 merged group:\n%s", grouped)
	}
	if !strings.Contains(grouped, "key=src:src/doc2.md") || !strings.Contains(grouped, "hits=1") {
		t.Fatalf("grouped search missing doc2 group:\n%s", grouped)
	}
	// Members are printed when requested.
	if !strings.Contains(grouped, "rank=1") {
		t.Fatalf("grouped search missing member ranks:\n%s", grouped)
	}
}

// TestCLISearchInvalidGroupBy: an unknown group_by value must print an error
// instead of silently falling back (S33 at the CLI layer).
func TestCLISearchInvalidGroupBy(t *testing.T) {
	bin := buildOKF(t)
	repo := initCLIRepo(t)
	kb := filepath.Join(repo, ".okf", "knowledge", "concepts")
	mustWriteCLIFile(t, filepath.Join(kb, "one.md"), "---\ntype: concept\ntitle: Any\n---\nbody token.\n")
	out := runOKF(t, bin, "search", "-path", repo, "-q", "token", "-group-by", "document")
	if !strings.Contains(out, "invalid_group_by") {
		t.Fatalf("expected invalid_group_by error, got:\n%s", out)
	}
}
