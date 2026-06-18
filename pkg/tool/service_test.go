package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	okfgit "github.com/superops-team/okf/pkg/git"
	"github.com/superops-team/okf/pkg/okf"
)

func TestStatusNotInitializedReturnsVersionedEnvelope(t *testing.T) {
	repo := initToolTestRepo(t)
	wantRepo := canonicalPath(t, repo)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Status(context.Background(), StatusRequest{})

	if resp.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %q, want %q", resp.SchemaVersion, SchemaVersion)
	}
	if resp.Operation != OperationStatus {
		t.Fatalf("operation = %q, want %q", resp.Operation, OperationStatus)
	}
	if resp.OK {
		t.Fatal("status OK = true, want false for uninitialized knowledge base")
	}
	if resp.Mutating {
		t.Fatal("status mutating = true, want false")
	}
	if resp.Error == nil || resp.Error.Code != ErrKnowledgeNotInitialized {
		t.Fatalf("error = %#v, want code %q", resp.Error, ErrKnowledgeNotInitialized)
	}
	if resp.RepoRoot != wantRepo {
		t.Fatalf("repo root = %q, want %q", resp.RepoRoot, wantRepo)
	}
	if resp.KnowledgeDir != filepath.Join(wantRepo, ".okf", "knowledge") {
		t.Fatalf("knowledge dir = %q", resp.KnowledgeDir)
	}
	if resp.Freshness == nil || resp.Freshness.Head == "" {
		t.Fatalf("freshness = %#v, want current HEAD", resp.Freshness)
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("response did not marshal to valid JSON: %s", data)
	}
}

func TestStatusFreshnessOmitsHeadWhenRepositoryHasNoCommits(t *testing.T) {
	repo := t.TempDir()
	runToolGit(t, repo, "init")
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "alpha.md"), `---
type: concept
title: Alpha
---
Alpha content.
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Status(context.Background(), StatusRequest{})

	if !resp.OK {
		t.Fatalf("status OK = false, error = %#v", resp.Error)
	}
	if resp.Freshness == nil {
		t.Fatal("freshness = nil, want empty head freshness")
	}
	if strings.Contains(resp.Freshness.Head, "fatal:") || strings.Contains(resp.Freshness.Head, "HEAD") {
		t.Fatalf("freshness head = %q, want empty commit hash on unborn HEAD", resp.Freshness.Head)
	}
}

func TestToolEnvelopeMutatingFlagByOperation(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})

	initResp := svc.Init(context.Background(), InitRequest{})
	if !initResp.Mutating {
		t.Fatalf("init mutating = false, want true")
	}

	statusResp := svc.Status(context.Background(), StatusRequest{})
	if statusResp.Mutating {
		t.Fatalf("status mutating = true, want false")
	}

	queryResp := svc.Query(context.Background(), QueryRequest{Query: "README", Limit: 1})
	if queryResp.Mutating {
		t.Fatalf("query mutating = true, want false")
	}

	refreshResp := svc.Refresh(context.Background(), RefreshRequest{Mode: RefreshModeCacheOnly})
	if !refreshResp.Mutating {
		t.Fatalf("refresh mutating = false, want true")
	}
}

func TestInitCreatesKnowledgeBaseWithVersionedEnvelope(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})

	resp := svc.Init(context.Background(), InitRequest{})

	if !resp.OK {
		t.Fatalf("init OK = false, error = %#v", resp.Error)
	}
	if resp.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %q, want %q", resp.SchemaVersion, SchemaVersion)
	}
	if resp.Operation != OperationInit {
		t.Fatalf("operation = %q, want %q", resp.Operation, OperationInit)
	}
	if _, err := os.Stat(filepath.Join(repo, ".okf", "knowledge")); err != nil {
		t.Fatalf("knowledge dir was not created: %v", err)
	}
	result, ok := resp.Result.(InitResult)
	if !ok {
		t.Fatalf("result type = %T, want InitResult", resp.Result)
	}
	if result.ConceptCount == 0 || result.SavedCount == 0 {
		t.Fatalf("init counts = concepts:%d saved:%d, want positive", result.ConceptCount, result.SavedCount)
	}
}

func TestRefreshFullCreatesKnowledgeBaseWithVersionedEnvelope(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})

	resp := svc.Refresh(context.Background(), RefreshRequest{Mode: RefreshModeFull})

	if !resp.OK {
		t.Fatalf("refresh OK = false, error = %#v", resp.Error)
	}
	if resp.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %q, want %q", resp.SchemaVersion, SchemaVersion)
	}
	if resp.Operation != OperationRefresh {
		t.Fatalf("operation = %q, want %q", resp.Operation, OperationRefresh)
	}
	result, ok := resp.Result.(RefreshResult)
	if !ok {
		t.Fatalf("result type = %T, want RefreshResult", resp.Result)
	}
	if result.Mode != RefreshModeFull || result.GeneratedCount == 0 || result.SavedCount == 0 {
		t.Fatalf("refresh result = %#v, want full mode with positive counts", result)
	}
	if _, err := os.Stat(filepath.Join(repo, ".okf", "knowledge")); err != nil {
		t.Fatalf("knowledge dir was not created: %v", err)
	}
}

func TestInitAndStatusUseAbsoluteKnowledgeDir(t *testing.T) {
	repo := initToolTestRepo(t)
	knowledgeDir := filepath.Join(t.TempDir(), "knowledge")
	svc := NewService(Config{RepoPath: repo, KnowledgeDir: knowledgeDir})

	initResp := svc.Init(context.Background(), InitRequest{})
	if !initResp.OK {
		t.Fatalf("init OK = false, error = %#v", initResp.Error)
	}
	if _, err := os.Stat(knowledgeDir); err != nil {
		t.Fatalf("absolute knowledge dir was not created: %v", err)
	}

	statusResp := svc.Status(context.Background(), StatusRequest{})
	if !statusResp.OK {
		t.Fatalf("status OK = false, error = %#v", statusResp.Error)
	}
	if statusResp.KnowledgeDir != knowledgeDir {
		t.Fatalf("knowledge dir = %q, want %q", statusResp.KnowledgeDir, knowledgeDir)
	}
}

func TestStatusFreshnessUsesConfiguredKnowledgeDir(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo, KnowledgeDir: filepath.Join("custom", "knowledge")})
	initResp := svc.Init(context.Background(), InitRequest{})
	if !initResp.OK {
		t.Fatalf("init OK = false, error = %#v", initResp.Error)
	}

	statusResp := svc.Status(context.Background(), StatusRequest{})
	if !statusResp.OK {
		t.Fatalf("status OK = false, error = %#v", statusResp.Error)
	}
	if statusResp.Freshness == nil || statusResp.Freshness.LastIndexedCommit == "" {
		t.Fatalf("freshness = %#v, want last indexed commit from custom knowledge state", statusResp.Freshness)
	}
}

func TestRefreshIncrementalUpdatesChangedKnowledge(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	initResp := svc.Init(context.Background(), InitRequest{})
	if !initResp.OK {
		t.Fatalf("init OK = false, error = %#v", initResp.Error)
	}

	mustWriteToolFile(t, filepath.Join(repo, "handler.go"), "package main\nfunc HandleRequest() {}\n")
	runToolGit(t, repo, "add", "handler.go")
	runToolGit(t, repo, "commit", "-m", "add handler")

	resp := svc.Refresh(context.Background(), RefreshRequest{Mode: RefreshModeIncremental})

	if !resp.OK {
		t.Fatalf("refresh OK = false, error = %#v", resp.Error)
	}
	result, ok := resp.Result.(RefreshResult)
	if !ok {
		t.Fatalf("result type = %T, want RefreshResult", resp.Result)
	}
	if result.Mode != RefreshModeIncremental || result.UpdatedCount == 0 {
		t.Fatalf("refresh result = %#v, want incremental mode with updated count", result)
	}
	queryResp := svc.Query(context.Background(), QueryRequest{Query: "HandleRequest", Limit: 5})
	if !queryResp.OK {
		t.Fatalf("query OK = false, error = %#v", queryResp.Error)
	}
	queryResult, ok := queryResp.Result.(QueryResult)
	if !ok || len(queryResult.Results) == 0 {
		t.Fatalf("query result = %#v, want updated handler knowledge", queryResp.Result)
	}
}

func TestRefreshCacheOnlyDoesNotCreateKnowledgeBase(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})

	resp := svc.Refresh(context.Background(), RefreshRequest{Mode: RefreshModeCacheOnly})

	if !resp.OK {
		t.Fatalf("cache-only refresh OK = false, error = %#v", resp.Error)
	}
	if _, err := os.Stat(filepath.Join(repo, ".okf")); !os.IsNotExist(err) {
		t.Fatalf("cache-only refresh created .okf or returned unexpected error: %v", err)
	}
	result, ok := resp.Result.(RefreshResult)
	if !ok || result.Mode != RefreshModeCacheOnly {
		t.Fatalf("result = %#v, want cache-only RefreshResult", resp.Result)
	}
}

func TestRefreshCacheOnlyDoesNotReportRebuiltCacheWithoutCacheBackend(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	initResp := svc.Init(context.Background(), InitRequest{})
	if !initResp.OK {
		t.Fatalf("init OK = false, error = %#v", initResp.Error)
	}

	resp := svc.Refresh(context.Background(), RefreshRequest{Mode: RefreshModeCacheOnly})

	if !resp.OK {
		t.Fatalf("cache-only refresh OK = false, error = %#v", resp.Error)
	}
	result, ok := resp.Result.(RefreshResult)
	if !ok {
		t.Fatalf("result type = %T, want RefreshResult", resp.Result)
	}
	if result.RebuiltCache {
		t.Fatal("rebuilt_cache = true, want false because V1 has no persistent cache backend")
	}
}

func TestRefreshRejectsInvalidMode(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})

	resp := svc.Refresh(context.Background(), RefreshRequest{Mode: "auto"})

	if resp.OK {
		t.Fatal("refresh OK = true, want false for disabled auto mode")
	}
	if resp.Error == nil || resp.Error.Code != ErrInvalidRequest {
		t.Fatalf("error = %#v, want code %q", resp.Error, ErrInvalidRequest)
	}
}

func TestQueryAndContextAreReadOnlyWhenKnowledgeMissing(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})

	queryResp := svc.Query(context.Background(), QueryRequest{Query: "anything"})
	if queryResp.OK {
		t.Fatal("query OK = true, want false for missing knowledge base")
	}
	if queryResp.Error == nil || queryResp.Error.Code != ErrKnowledgeNotInitialized {
		t.Fatalf("query error = %#v, want code %q", queryResp.Error, ErrKnowledgeNotInitialized)
	}

	contextResp := svc.Context(context.Background(), ContextRequest{Query: "anything", BudgetTokens: 1000})
	if contextResp.OK {
		t.Fatal("context OK = true, want false for missing knowledge base")
	}
	if contextResp.Error == nil || contextResp.Error.Code != ErrKnowledgeNotInitialized {
		t.Fatalf("context error = %#v, want code %q", contextResp.Error, ErrKnowledgeNotInitialized)
	}

	if _, err := os.Stat(filepath.Join(repo, ".okf")); !os.IsNotExist(err) {
		t.Fatalf("read-only operations created .okf or returned unexpected error: %v", err)
	}
}

func TestStatusReadOnlyDoesNotCreateConfiguredKnowledgeDir(t *testing.T) {
	repo := initToolTestRepo(t)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	configuredDir := filepath.Join(t.TempDir(), "configured", "knowledge")
	if err := okf.SaveConfig(&okf.Config{KnowledgeDir: configuredDir}, configPath); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	t.Setenv("OKF_CONFIG_PATH", configPath)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Status(context.Background(), StatusRequest{})
	if resp.OK {
		t.Fatal("status OK = true, want false for missing configured knowledge base")
	}
	if resp.Error == nil || resp.Error.Code != ErrKnowledgeNotInitialized {
		t.Fatalf("status error = %#v, want code %q", resp.Error, ErrKnowledgeNotInitialized)
	}
	if resp.KnowledgeDir != configuredDir {
		t.Fatalf("knowledge dir = %q, want %q", resp.KnowledgeDir, configuredDir)
	}
	if _, err := os.Stat(configuredDir); !os.IsNotExist(err) {
		t.Fatalf("status created configured knowledge dir or returned unexpected stat error: %v", err)
	}
}

func TestStatusReportsResolvedPathMetadata(t *testing.T) {
	repo := initToolTestRepo(t)
	writeDir := filepath.Join(repo, ".okf", "knowledge")
	overlayDir := filepath.Join(t.TempDir(), "overlay", "knowledge")
	mustWriteToolFile(t, filepath.Join(writeDir, "concepts", "alpha.md"), `---
type: concept
title: Alpha Service
---
Alpha content.
`)
	mustWriteToolFile(t, filepath.Join(overlayDir, "concepts", "beta.md"), `---
type: concept
title: Beta Service
---
Beta content.
`)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := okf.SaveConfig(&okf.Config{KnowledgePaths: []string{overlayDir}}, configPath); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	t.Setenv("OKF_CONFIG_PATH", configPath)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Status(context.Background(), StatusRequest{})

	if !resp.OK {
		t.Fatalf("status OK = false, error = %#v", resp.Error)
	}
	result, ok := resp.Result.(StatusResult)
	if !ok {
		t.Fatalf("result type = %T, want StatusResult", resp.Result)
	}
	wantWriteDir := canonicalPath(t, writeDir)
	if result.KnowledgePath.WritePath != wantWriteDir || len(result.KnowledgePath.ReadPaths) != 2 {
		t.Fatalf("knowledge path metadata = %#v, want write dir and two read paths", result.KnowledgePath)
	}
	if result.KnowledgePath.ReadPaths[1].Path != overlayDir || result.KnowledgePath.ReadPaths[1].Rank != 1 {
		t.Fatalf("overlay read path = %#v, want rank 1 %s", result.KnowledgePath.ReadPaths[1], overlayDir)
	}
}

func TestQueryReturnsVersionedRankedResults(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "alpha.md"), `---
type: concept
title: Alpha Service
description: Handles AlphaSymbol routing
tags:
  - code
---
AlphaSymbol lives in internal/alpha/service.go and owns request routing.
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Query(context.Background(), QueryRequest{Query: "AlphaSymbol", Limit: 5})

	if !resp.OK {
		t.Fatalf("query OK = false, error = %#v", resp.Error)
	}
	if resp.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %q, want %q", resp.SchemaVersion, SchemaVersion)
	}
	result, ok := resp.Result.(QueryResult)
	if !ok {
		t.Fatalf("result type = %T, want QueryResult", resp.Result)
	}
	if len(result.Results) != 1 {
		t.Fatalf("result count = %d, want 1", len(result.Results))
	}
	got := result.Results[0]
	if got.Title != "Alpha Service" {
		t.Fatalf("title = %q, want Alpha Service", got.Title)
	}
	if got.Score <= 0 {
		t.Fatalf("score = %d, want positive", got.Score)
	}
	if got.Reason == "" {
		t.Fatal("reason is empty")
	}
}

func TestQueryFiltersAndReturnsSymbolNavigation(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "alpha.md"), `---
type: code_symbol
title: RouteAlphaSymbol
description: Exact symbol for alpha routing
resource: code://repo/internal/alpha/service.go
tags:
  - code
  - routing
source_path: internal/alpha/service.go
symbol_kind: function
qualified_name: alpha.RouteAlphaSymbol
start_line: 12
end_line: 14
---
RouteAlphaSymbol handles alpha routing.
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "beta.md"), `---
type: code_symbol
title: RouteBetaSymbol
description: Mentions RouteAlphaSymbol in docs only
resource: code://repo/internal/beta/service.go
tags:
  - code
source_path: internal/beta/service.go
symbol_kind: function
qualified_name: beta.RouteBetaSymbol
start_line: 20
end_line: 22
---
This documentation mentions RouteAlphaSymbol but is not the routing tag target.
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Query(context.Background(), QueryRequest{
		Query:      "RouteAlphaSymbol",
		Limit:      5,
		Type:       "code_symbol",
		Tag:        "routing",
		FilePath:   "internal/alpha/service.go",
		SymbolKind: "function",
	})

	if !resp.OK {
		t.Fatalf("query OK = false, error = %#v", resp.Error)
	}
	result, ok := resp.Result.(QueryResult)
	if !ok {
		t.Fatalf("result type = %T, want QueryResult", resp.Result)
	}
	if len(result.Results) != 1 {
		t.Fatalf("result count = %d, want 1", len(result.Results))
	}
	got := result.Results[0]
	if got.Title != "RouteAlphaSymbol" {
		t.Fatalf("title = %q, want RouteAlphaSymbol", got.Title)
	}
	if got.SymbolKind != "function" || got.QualifiedName != "alpha.RouteAlphaSymbol" {
		t.Fatalf("symbol metadata = kind:%q qualified:%q", got.SymbolKind, got.QualifiedName)
	}
	if got.SourcePath != "internal/alpha/service.go" || got.StartLine != 12 || got.EndLine != 14 {
		t.Fatalf("source navigation = %s:%d-%d, want internal/alpha/service.go:12-14", got.SourcePath, got.StartLine, got.EndLine)
	}
	if got.Location != "internal/alpha/service.go:12-14" {
		t.Fatalf("location = %q, want internal/alpha/service.go:12-14", got.Location)
	}
	if got.ConceptPath == "" {
		t.Fatal("concept_path is empty")
	}
	if !strings.Contains(got.Reason, "exact symbol match") {
		t.Fatalf("reason = %q, want exact symbol match", got.Reason)
	}
}

func TestQueryFiltersByLanguageAndQualifiedName(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "alpha.md"), `---
type: code_symbol
title: RouteAlphaSymbol
description: Exact symbol for alpha routing
resource: code://repo/internal/alpha/service.go
tags:
  - code
source_path: internal/alpha/service.go
language: go
symbol_kind: function
qualified_name: alpha.RouteAlphaSymbol
start_line: 12
end_line: 14
---
RouteAlphaSymbol handles alpha routing.
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "typescript.md"), `---
type: code_symbol
title: RouteAlphaSymbol
description: Same query but different language
resource: code://repo/internal/alpha/service.ts
tags:
  - code
source_path: internal/alpha/service.ts
language: typescript
symbol_kind: function
qualified_name: alpha.RouteAlphaSymbolTS
start_line: 8
end_line: 9
---
RouteAlphaSymbol handles TypeScript routing.
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Query(context.Background(), QueryRequest{
		Query:         "RouteAlphaSymbol",
		Limit:         5,
		Language:      "go",
		QualifiedName: "alpha.RouteAlphaSymbol",
	})

	if !resp.OK {
		t.Fatalf("query OK = false, error = %#v", resp.Error)
	}
	result, ok := resp.Result.(QueryResult)
	if !ok {
		t.Fatalf("result type = %T, want QueryResult", resp.Result)
	}
	if len(result.Results) != 1 {
		t.Fatalf("result count = %d, want 1", len(result.Results))
	}
	if got := result.Results[0]; got.SourcePath != "internal/alpha/service.go" || got.QualifiedName != "alpha.RouteAlphaSymbol" {
		t.Fatalf("result = %#v, want Go alpha.RouteAlphaSymbol", got)
	}
}

func TestQueryDeterministicTieBreakUsesSourcePathLineAndConceptPath(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "zeta.md"), `---
type: code_symbol
title: SharedSymbol
description: SharedSymbol
resource: code://repo/internal/zeta/service.go
tags:
  - code
source_path: internal/zeta/service.go
symbol_kind: function
qualified_name: zeta.SharedSymbol
start_line: 30
end_line: 31
---
SharedSymbol
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "alpha.md"), `---
type: code_symbol
title: SharedSymbol
description: SharedSymbol
resource: code://repo/internal/alpha/service.go
tags:
  - code
source_path: internal/alpha/service.go
symbol_kind: function
qualified_name: alpha.SharedSymbol
start_line: 40
end_line: 41
---
SharedSymbol
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "alpha_early.md"), `---
type: code_symbol
title: SharedSymbol
description: SharedSymbol
resource: code://repo/internal/alpha/service.go
tags:
  - code
source_path: internal/alpha/service.go
symbol_kind: function
qualified_name: alpha.SharedSymbol
start_line: 10
end_line: 11
---
SharedSymbol
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Query(context.Background(), QueryRequest{Query: "SharedSymbol", Limit: 3, Type: "code_symbol"})

	if !resp.OK {
		t.Fatalf("query OK = false, error = %#v", resp.Error)
	}
	result, ok := resp.Result.(QueryResult)
	if !ok {
		t.Fatalf("result type = %T, want QueryResult", resp.Result)
	}
	if len(result.Results) != 3 {
		t.Fatalf("result count = %d, want 3", len(result.Results))
	}
	want := []struct {
		path string
		line int
	}{
		{path: "internal/alpha/service.go", line: 10},
		{path: "internal/alpha/service.go", line: 40},
		{path: "internal/zeta/service.go", line: 30},
	}
	for i, wantHit := range want {
		got := result.Results[i]
		if got.SourcePath != wantHit.path || got.StartLine != wantHit.line {
			t.Fatalf("result[%d] = %s:%d, want %s:%d", i, got.SourcePath, got.StartLine, wantHit.path, wantHit.line)
		}
	}
}

func TestQuerySinglePathOrderingPreservesLegacyTieBreakWithoutExactness(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "zeta.md"), `---
type: concept
title: Zeta Owner
description: LegacyOrderToken shared description
resource: internal/zeta/service.go
source_path: internal/zeta/service.go
start_line: 30
end_line: 31
---
LegacyOrderToken shared content.
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "alpha-late.md"), `---
type: concept
title: Alpha Late Owner
description: LegacyOrderToken shared description
resource: internal/alpha/service.go
source_path: internal/alpha/service.go
start_line: 40
end_line: 41
---
LegacyOrderToken shared content.
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "alpha-early.md"), `---
type: concept
title: Alpha Early Owner
description: LegacyOrderToken shared description
resource: internal/alpha/service.go
source_path: internal/alpha/service.go
start_line: 10
end_line: 11
---
LegacyOrderToken shared content.
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Query(context.Background(), QueryRequest{Query: "LegacyOrderToken", Limit: 3, Type: "concept"})

	if !resp.OK {
		t.Fatalf("query OK = false, error = %#v", resp.Error)
	}
	result := resp.Result.(QueryResult)
	if len(result.Results) != 3 {
		t.Fatalf("result count = %d, want 3", len(result.Results))
	}
	want := []struct {
		path string
		line int
	}{
		{path: "internal/alpha/service.go", line: 10},
		{path: "internal/alpha/service.go", line: 40},
		{path: "internal/zeta/service.go", line: 30},
	}
	for i, wantHit := range want {
		got := result.Results[i]
		if got.SourcePath != wantHit.path || got.StartLine != wantHit.line {
			t.Fatalf("result[%d] = %s:%d, want legacy single-path order %s:%d", i, got.SourcePath, got.StartLine, wantHit.path, wantHit.line)
		}
		if strings.Contains(got.Reason, "exact") || strings.Contains(got.Reason, "title match") || strings.Contains(got.Reason, "qualified symbol match") {
			t.Fatalf("result[%d] reason = %q, want no structured exactness signal", i, got.Reason)
		}
	}
}

func TestQueryReturnsRelationOrientedResults(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "relations", "imports.md"), `---
type: code_relation
title: alpha imports beta
description: alpha service imports beta package
resource: okf://relations/imports
tags:
  - code
  - relations
source_path: internal/alpha/service.go
relation_kind: file_import
relation_source: internal/alpha/service.go
relation_target: internal/beta/service.go
start_line: 27
end_line: 27
provenance: okf-go-ast
---
internal/alpha/service.go imports internal/beta/service.go
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "relations", "ownership.md"), `---
type: code_relation
title: alpha owns RouteAlphaSymbol
description: alpha file owns symbol RouteAlphaSymbol
resource: okf://relations/ownership
tags:
  - code
  - relations
source_path: internal/alpha/service.go
relation_kind: file_owns_symbol
relation_source: internal/alpha/service.go
relation_target: alpha.RouteAlphaSymbol
start_line: 12
end_line: 12
provenance: okf-go-ast
---
internal/alpha/service.go owns alpha.RouteAlphaSymbol
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Query(context.Background(), QueryRequest{
		Query:        "internal/beta/service.go",
		Type:         "code_relation",
		RelationKind: "file_import",
		FilePath:     "internal/alpha/service.go",
	})

	if !resp.OK {
		t.Fatalf("query OK = false, error = %#v", resp.Error)
	}
	result, ok := resp.Result.(QueryResult)
	if !ok {
		t.Fatalf("result type = %T, want QueryResult", resp.Result)
	}
	if len(result.Results) != 1 {
		t.Fatalf("result count = %d, want 1", len(result.Results))
	}
	got := result.Results[0]
	if got.RelationKind != "file_import" {
		t.Fatalf("relation_kind = %q, want file_import", got.RelationKind)
	}
	if got.RelationSource != "internal/alpha/service.go" || got.RelationTarget != "internal/beta/service.go" {
		t.Fatalf("relation = %q -> %q, want alpha -> beta", got.RelationSource, got.RelationTarget)
	}
	if got.SourcePath != "internal/alpha/service.go" || got.StartLine != 27 || got.EndLine != 27 {
		t.Fatalf("source navigation = %s:%d-%d, want internal/alpha/service.go:27-27", got.SourcePath, got.StartLine, got.EndLine)
	}
	if got.Provenance != "okf-go-ast" {
		t.Fatalf("provenance = %q, want okf-go-ast", got.Provenance)
	}
	if !strings.Contains(got.Reason, "relation target match") {
		t.Fatalf("reason = %q, want relation target match", got.Reason)
	}
}

func TestQueryRelationSourceAndTargetFiltersAreAdditive(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "relations", "alpha-beta.md"), `---
type: code_relation
title: alpha imports beta
description: alpha imports beta
resource: okf://relations/alpha-beta
source_path: internal/alpha/service.go
relation_kind: file_import
relation_source: internal/alpha/service.go
relation_target: internal/beta/service.go
start_line: 10
end_line: 10
---
alpha imports beta
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "relations", "alpha-gamma.md"), `---
type: code_relation
title: alpha imports gamma
description: alpha imports gamma
resource: okf://relations/alpha-gamma
source_path: internal/alpha/service.go
relation_kind: file_import
relation_source: internal/alpha/service.go
relation_target: internal/gamma/service.go
start_line: 11
end_line: 11
---
alpha imports gamma
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "relations", "delta-beta.md"), `---
type: code_relation
title: delta imports beta
description: delta imports beta
resource: okf://relations/delta-beta
source_path: internal/delta/service.go
relation_kind: file_import
relation_source: internal/delta/service.go
relation_target: internal/beta/service.go
start_line: 12
end_line: 12
---
delta imports beta
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Query(context.Background(), QueryRequest{
		Query:          "imports",
		Type:           "code_relation",
		RelationKind:   "file_import",
		RelationSource: "internal/alpha/service.go",
		RelationTarget: "internal/beta/service.go",
		Limit:          5,
	})

	if !resp.OK {
		t.Fatalf("query OK = false, error = %#v", resp.Error)
	}
	result := resp.Result.(QueryResult)
	data, err := json.MarshalIndent(result.Results, "", "  ")
	if err != nil {
		t.Fatalf("marshal results: %v", err)
	}
	got := string(data)
	want := `[
  {
    "title": "alpha imports beta",
    "type": "code_relation",
    "resource": "okf://relations/alpha-beta",
    "file_path": "internal/alpha/service.go",
    "source_path": "internal/alpha/service.go",
    "location": "internal/alpha/service.go:10",
    "concept_path": "relations/alpha-beta.md",
    "start_line": 10,
    "end_line": 10,
    "relation_kind": "file_import",
    "relation_source": "internal/alpha/service.go",
    "relation_target": "internal/beta/service.go",
    "score": 80,
    "reason": "title match, description match, content match",
    "provenance": "okf.code",
    "knowledge_path": "` + canonicalPath(t, filepath.Join(repo, ".okf", "knowledge")) + `",
    "knowledge_path_source": "repo_local",
    "source_rank": 0
  }
]`
	if got != want {
		t.Fatalf("relation source/target golden mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestQueryStructuredFiltersRemainExactAfterSharedQueryFiltering(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "alpha.md"), `---
type: code_symbol
title: RouteAlphaSymbol
description: RouteAlphaSymbol exact match
resource: code://repo/internal/alpha/service.go
source_path: internal/alpha/service.go
language: go
symbol_kind: function
qualified_name: alpha.RouteAlphaSymbol
start_line: 2
end_line: 2
---
RouteAlphaSymbol exact match.
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "alpha-suffix.md"), `---
type: code_symbol
title: RouteAlphaSymbolSuffix
description: RouteAlphaSymbol suffix should not pass exact filters
resource: code://repo/internal/alpha/service.go.bak
source_path: internal/alpha/service.go.bak
language: go
symbol_kind: function
qualified_name: alpha.RouteAlphaSymbolSuffix
start_line: 2
end_line: 2
---
RouteAlphaSymbol suffix match.
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Query(context.Background(), QueryRequest{
		Query:         "RouteAlphaSymbol",
		FilePath:      "internal/alpha/service.go",
		QualifiedName: "alpha.RouteAlphaSymbol",
		Limit:         10,
	})

	if !resp.OK {
		t.Fatalf("query OK = false, error = %#v", resp.Error)
	}
	result := resp.Result.(QueryResult)
	if len(result.Results) != 1 {
		t.Fatalf("results = %#v, want only exact structured filter match", result.Results)
	}
	if result.Results[0].QualifiedName != "alpha.RouteAlphaSymbol" || result.Results[0].SourcePath != "internal/alpha/service.go" {
		t.Fatalf("result = %#v, want exact alpha symbol", result.Results[0])
	}
}

func TestQueryRelationSourceAndTargetFiltersRemainExact(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "relations", "alpha-beta.md"), `---
type: code_relation
title: alpha imports beta
description: alpha imports beta
resource: okf://relations/alpha-beta
source_path: internal/alpha/service.go
relation_kind: file_import
relation_source: internal/alpha/service.go
relation_target: internal/beta/service.go
start_line: 10
end_line: 10
---
alpha imports beta
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "relations", "alpha-beta-bak.md"), `---
type: code_relation
title: alpha imports beta backup
description: alpha imports beta backup
resource: okf://relations/alpha-beta-bak
source_path: internal/alpha/service.go
relation_kind: file_import
relation_source: internal/alpha/service.go.bak
relation_target: internal/beta/service.go.bak
start_line: 11
end_line: 11
---
alpha imports beta backup
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Query(context.Background(), QueryRequest{
		Query:          "imports beta",
		Type:           "code_relation",
		RelationKind:   "file_import",
		RelationSource: "internal/alpha/service.go",
		RelationTarget: "internal/beta/service.go",
		Limit:          10,
	})

	if !resp.OK {
		t.Fatalf("query OK = false, error = %#v", resp.Error)
	}
	result := resp.Result.(QueryResult)
	if len(result.Results) != 1 {
		t.Fatalf("results = %#v, want only exact relation source/target match", result.Results)
	}
	if result.Results[0].RelationSource != "internal/alpha/service.go" || result.Results[0].RelationTarget != "internal/beta/service.go" {
		t.Fatalf("result = %#v, want exact relation endpoints", result.Results[0])
	}
}

func TestQueryUsesOverlayReadPathsAndReportsSourceMetadata(t *testing.T) {
	repo := initToolTestRepo(t)
	writeDir := filepath.Join(repo, ".okf", "knowledge")
	overlayDir := filepath.Join(t.TempDir(), "overlay", "knowledge")
	mustWriteToolFile(t, filepath.Join(writeDir, "concepts", "alpha.md"), `---
type: concept
title: Alpha Service
description: Local write target concept
---
Alpha content.
`)
	mustWriteToolFile(t, filepath.Join(overlayDir, "concepts", "beta.md"), `---
type: concept
title: Beta Service
description: Overlay-only concept
---
BetaOverlayToken lives only in overlay knowledge.
`)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := okf.SaveConfig(&okf.Config{KnowledgePaths: []string{overlayDir}}, configPath); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	t.Setenv("OKF_CONFIG_PATH", configPath)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Query(context.Background(), QueryRequest{Query: "BetaOverlayToken", Limit: 5, IncludeTrace: true})

	if !resp.OK {
		t.Fatalf("query OK = false, error = %#v", resp.Error)
	}
	result, ok := resp.Result.(QueryResult)
	if !ok {
		t.Fatalf("result type = %T, want QueryResult", resp.Result)
	}
	if len(result.Results) != 1 {
		t.Fatalf("result count = %d, want 1", len(result.Results))
	}
	hit := result.Results[0]
	if hit.Title != "Beta Service" || hit.KnowledgePath != overlayDir || hit.KnowledgePathSource != "overlay" || hit.SourceRank != 1 {
		t.Fatalf("hit = %#v, want overlay source metadata", hit)
	}
	if len(result.Trace) == 0 || !traceRefsContain(result.Trace, overlayDir) {
		t.Fatalf("trace = %#v, want overlay path reference", result.Trace)
	}
}

func TestQueryDeduplicatesGeneratedOverlayConceptsBySourceRank(t *testing.T) {
	repo := initToolTestRepo(t)
	writeDir := filepath.Join(repo, ".okf", "knowledge")
	overlayDir := filepath.Join(t.TempDir(), "overlay", "knowledge")
	generated := `---
type: code_symbol
title: SharedSymbol
description: SharedSymbol generated concept
resource: code://repo/internal/shared.go
source_path: internal/shared.go
symbol_kind: function
qualified_name: shared.SharedSymbol
generated: true
generator: okf.git
---
SharedSymbol generated concept.
`
	mustWriteToolFile(t, filepath.Join(writeDir, "code", "shared.md"), generated)
	mustWriteToolFile(t, filepath.Join(overlayDir, "code", "shared.md"), generated)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := okf.SaveConfig(&okf.Config{KnowledgePaths: []string{overlayDir}}, configPath); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	t.Setenv("OKF_CONFIG_PATH", configPath)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Query(context.Background(), QueryRequest{Query: "SharedSymbol", Limit: 5, IncludeTrace: true})

	if !resp.OK {
		t.Fatalf("query OK = false, error = %#v", resp.Error)
	}
	result := resp.Result.(QueryResult)
	if len(result.Results) != 1 {
		t.Fatalf("result count = %d, want duplicate generated concept collapsed to 1", len(result.Results))
	}
	hit := result.Results[0]
	wantWriteDir := canonicalPath(t, writeDir)
	if hit.KnowledgePath != wantWriteDir || hit.SourceRank != 0 {
		t.Fatalf("hit = %#v, want write path source rank 0 as primary", hit)
	}
	if len(hit.DuplicateSources) != 1 || hit.DuplicateSources[0].Path != overlayDir {
		t.Fatalf("duplicate sources = %#v, want overlay duplicate retained", hit.DuplicateSources)
	}
}

func TestQueryTraceIncludesOverlayMergeDecision(t *testing.T) {
	repo := initToolTestRepo(t)
	writeDir := filepath.Join(repo, ".okf", "knowledge")
	overlayDir := filepath.Join(t.TempDir(), "overlay", "knowledge")
	generated := `---
type: code_symbol
title: SharedSymbol
description: SharedSymbol generated concept
resource: code://repo/internal/shared.go
source_path: internal/shared.go
symbol_kind: function
qualified_name: shared.SharedSymbol
generated: true
generator: okf.git
---
SharedSymbol generated concept.
`
	mustWriteToolFile(t, filepath.Join(writeDir, "code", "shared.md"), generated)
	mustWriteToolFile(t, filepath.Join(overlayDir, "code", "shared.md"), generated)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := okf.SaveConfig(&okf.Config{KnowledgePaths: []string{overlayDir}}, configPath); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	t.Setenv("OKF_CONFIG_PATH", configPath)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Query(context.Background(), QueryRequest{Query: "SharedSymbol", Limit: 5, IncludeTrace: true})

	if !resp.OK {
		t.Fatalf("query OK = false, error = %#v", resp.Error)
	}
	result := resp.Result.(QueryResult)
	step, ok := traceStepByType(result.Trace, "overlay_merge")
	if !ok {
		t.Fatalf("trace = %#v, want overlay_merge step", result.Trace)
	}
	if step.Counts["collapsed_duplicates"] != 1 {
		t.Fatalf("overlay_merge counts = %#v, want collapsed_duplicates=1", step.Counts)
	}
	if !traceStepRefsContain(step, canonicalPath(t, writeDir)) || !traceStepRefsContain(step, overlayDir) {
		t.Fatalf("overlay_merge step = %#v, want refs for write and overlay paths", step)
	}
}

func TestToolFixtureWithGeneratedKnowledgeAndUserOverlay(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, "cmd", "server", "main.go"), `package main

import "fmt"

func StartServer() { fmt.Println("ok") }
`)
	runToolGit(t, repo, "add", ".")
	runToolGit(t, repo, "commit", "-m", "add server source")

	writeDir := filepath.Join(repo, ".okf", "knowledge")
	bundle, err := okfgit.GenerateBundle(&okfgit.Config{
		RepoPath:      repo,
		KnowledgeDir:  writeDir,
		IncludeFiles:  []string{"*.go"},
		ExcludeDirs:   []string{".git", ".okf"},
		MaxFileSizeKB: 100,
	}, true)
	if err != nil {
		t.Fatalf("GenerateBundle() error = %v", err)
	}
	if _, err := okfgit.SaveKnowledgeBase(bundle, &okfgit.Config{RepoPath: repo, KnowledgeDir: writeDir}); err != nil {
		t.Fatalf("SaveKnowledgeBase() error = %v", err)
	}

	overlayDir := filepath.Join(t.TempDir(), "overlay", "knowledge")
	mustWriteToolFile(t, filepath.Join(overlayDir, "concepts", "deployment-note.md"), `---
type: concept
title: Deployment Overlay Note
description: User-authored overlay note for OverlayDeployToken
resource: okf://notes/deployment-overlay
tags:
  - overlay
---
OverlayDeployToken belongs to the user overlay knowledge path.
`)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := okf.SaveConfig(&okf.Config{KnowledgePaths: []string{overlayDir}}, configPath); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	t.Setenv("OKF_CONFIG_PATH", configPath)

	svc := NewService(Config{RepoPath: repo})
	generatedResp := svc.Query(context.Background(), QueryRequest{Query: "cmd/server/main.go", Type: "code_file", Limit: 5})
	if !generatedResp.OK {
		t.Fatalf("generated query OK = false, error = %#v", generatedResp.Error)
	}
	generated := generatedResp.Result.(QueryResult)
	if len(generated.Results) == 0 {
		t.Fatal("generated results empty, want generated code knowledge")
	}
	generatedHit := generated.Results[0]
	if !generatedHit.Generated || generatedHit.Generator != "okf.git" || generatedHit.KnowledgePath != canonicalPath(t, writeDir) || generatedHit.SourceRank != 0 {
		t.Fatalf("generated hit = %#v, want generated okf.git provenance from write path", generatedHit)
	}

	overlayResp := svc.Query(context.Background(), QueryRequest{Query: "OverlayDeployToken", Limit: 5})
	if !overlayResp.OK {
		t.Fatalf("overlay query OK = false, error = %#v", overlayResp.Error)
	}
	overlay := overlayResp.Result.(QueryResult)
	if len(overlay.Results) != 1 {
		t.Fatalf("overlay result count = %d, want 1", len(overlay.Results))
	}
	overlayHit := overlay.Results[0]
	if overlayHit.Title != "Deployment Overlay Note" || overlayHit.Generated || overlayHit.Generator != "" || overlayHit.KnowledgePath != overlayDir || overlayHit.SourceRank != 1 {
		t.Fatalf("overlay hit = %#v, want user-authored overlay source metadata", overlayHit)
	}
}

func TestQueryTraceIncludesFreshnessWarningWhenKnowledgeIsStale(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	initResp := svc.Init(context.Background(), InitRequest{})
	if !initResp.OK {
		t.Fatalf("init OK = false, error = %#v", initResp.Error)
	}
	mustWriteToolFile(t, filepath.Join(repo, "stale.go"), "package main\nfunc StaleSymbol() {}\n")
	runToolGit(t, repo, "add", "stale.go")
	runToolGit(t, repo, "commit", "-m", "make head stale")

	resp := svc.Query(context.Background(), QueryRequest{Query: "README", Limit: 1, IncludeTrace: true})

	if !resp.OK {
		t.Fatalf("query OK = false, error = %#v", resp.Error)
	}
	if len(resp.Warnings) == 0 || !strings.Contains(resp.Warnings[0], "stale") {
		t.Fatalf("warnings = %#v, want stale warning", resp.Warnings)
	}
	result := resp.Result.(QueryResult)
	step, ok := traceStepByType(result.Trace, "path_resolution")
	if !ok {
		t.Fatalf("trace = %#v, want path_resolution step", result.Trace)
	}
	if len(step.Warnings) == 0 || !containsString(step.Warnings, "knowledge is stale relative to HEAD") {
		t.Fatalf("path_resolution warnings = %#v, want stale freshness warning", step.Warnings)
	}
}

func TestQueryWarnsForMissingOptionalOverlayPath(t *testing.T) {
	repo := initToolTestRepo(t)
	writeDir := filepath.Join(repo, ".okf", "knowledge")
	missingOverlay := filepath.Join(t.TempDir(), "missing", "knowledge")
	mustWriteToolFile(t, filepath.Join(writeDir, "concepts", "alpha.md"), `---
type: concept
title: Alpha Service
description: AlphaToken local concept
---
AlphaToken content.
`)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := okf.SaveConfig(&okf.Config{KnowledgePaths: []string{missingOverlay}}, configPath); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	t.Setenv("OKF_CONFIG_PATH", configPath)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Query(context.Background(), QueryRequest{Query: "AlphaToken", Limit: 5, IncludeTrace: true})

	if !resp.OK {
		t.Fatalf("query OK = false, error = %#v", resp.Error)
	}
	if len(resp.Warnings) == 0 || !strings.Contains(resp.Warnings[0], missingOverlay) {
		t.Fatalf("warnings = %#v, want missing optional overlay warning", resp.Warnings)
	}
	result := resp.Result.(QueryResult)
	if len(result.Results) != 1 || result.Results[0].Title != "Alpha Service" {
		t.Fatalf("results = %#v, want local write path result despite missing overlay", result.Results)
	}
	if len(result.Trace) == 0 || len(result.Trace[0].Warnings) == 0 {
		t.Fatalf("trace = %#v, want path_resolution warning", result.Trace)
	}
}

func TestQueryReturnsInvalidRequestForNonDirectoryOverlayPath(t *testing.T) {
	repo := initToolTestRepo(t)
	writeDir := filepath.Join(repo, ".okf", "knowledge")
	mustWriteToolFile(t, filepath.Join(writeDir, "concepts", "alpha.md"), `---
type: concept
title: Alpha Service
---
AlphaToken content.
`)
	nonDirectoryOverlay := filepath.Join(t.TempDir(), "overlay.md")
	mustWriteToolFile(t, nonDirectoryOverlay, "not a directory")
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := okf.SaveConfig(&okf.Config{KnowledgePaths: []string{nonDirectoryOverlay}}, configPath); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	t.Setenv("OKF_CONFIG_PATH", configPath)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Query(context.Background(), QueryRequest{Query: "AlphaToken", Limit: 5})

	if resp.OK {
		t.Fatal("query OK = true, want invalid request for non-directory overlay")
	}
	if resp.Error == nil || resp.Error.Code != ErrInvalidRequest {
		t.Fatalf("error = %#v, want code %q", resp.Error, ErrInvalidRequest)
	}
	if !strings.Contains(resp.Error.Message, nonDirectoryOverlay) {
		t.Fatalf("error message = %q, want overlay path", resp.Error.Message)
	}
}

func TestQueryTraceIsDefaultOffAndDeterministicWhenEnabled(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "alpha.md"), `---
type: code_symbol
title: RouteAlphaSymbol
description: Exact symbol for alpha routing
resource: code://repo/internal/alpha/service.go
source_path: internal/alpha/service.go
language: go
symbol_kind: function
qualified_name: alpha.RouteAlphaSymbol
start_line: 12
end_line: 14
---
RouteAlphaSymbol handles alpha routing.
`)

	svc := NewService(Config{RepoPath: repo})
	defaultResp := svc.Query(context.Background(), QueryRequest{Query: "RouteAlphaSymbol", Limit: 5})
	if !defaultResp.OK {
		t.Fatalf("query OK = false, error = %#v", defaultResp.Error)
	}
	defaultResult := defaultResp.Result.(QueryResult)
	if defaultResult.Trace != nil {
		t.Fatalf("trace = %#v, want omitted by default", defaultResult.Trace)
	}

	traceResp := svc.Query(context.Background(), QueryRequest{Query: "RouteAlphaSymbol", Limit: 5, IncludeTrace: true})
	if !traceResp.OK {
		t.Fatalf("query trace OK = false, error = %#v", traceResp.Error)
	}
	traceResult := traceResp.Result.(QueryResult)
	if len(traceResult.Trace) < 4 {
		t.Fatalf("trace len = %d, want compact deterministic steps", len(traceResult.Trace))
	}
	if traceResult.Trace[0].Type != "path_resolution" {
		t.Fatalf("first trace step = %#v, want path_resolution", traceResult.Trace[0])
	}
	if traceResult.Trace[1].Type != "bundle_load" {
		t.Fatalf("second trace step = %#v, want bundle_load", traceResult.Trace[1])
	}
	if traceResult.Trace[len(traceResult.Trace)-1].Type != "ranking" {
		t.Fatalf("last trace step = %#v, want ranking", traceResult.Trace[len(traceResult.Trace)-1])
	}
}

func TestQueryTraceSchemaMatchesGoldenContract(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "golden.md"), `---
type: code_symbol
title: GoldenTraceSymbol
description: Stable trace fixture
resource: code://repo/internal/trace/service.go
source_path: internal/trace/service.go
language: go
symbol_kind: function
qualified_name: trace.GoldenTraceSymbol
start_line: 7
end_line: 9
---
Stable trace fixture body.
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Query(context.Background(), QueryRequest{Query: "GoldenTraceSymbol", Limit: 1, IncludeTrace: true})

	if !resp.OK {
		t.Fatalf("query OK = false, error = %#v", resp.Error)
	}
	result := resp.Result.(QueryResult)
	trace := normalizeTraceForGolden(result.Trace, repo)
	data, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}
	got := string(data)
	want := `[
  {
    "type": "path_resolution",
    "message": "resolved knowledge paths",
    "refs": [
      "$REPO/.okf/knowledge"
    ],
    "counts": {
      "read_paths": 1
    }
  },
  {
    "type": "bundle_load",
    "message": "loaded OKF knowledge bundle",
    "counts": {
      "concepts": 1
    }
  },
  {
    "type": "overlay_merge",
    "message": "no overlay merges applied",
    "counts": {
      "collapsed_duplicates": 0
    }
  },
  {
    "type": "filter_application",
    "message": "applied structured query filters",
    "counts": {
      "active_filters": 0
    }
  },
  {
    "type": "candidate_scoring",
    "message": "scored matching candidates",
    "counts": {
      "matches": 1
    },
    "score_delta": 200
  },
  {
    "type": "ranking",
    "message": "ordered candidates by deterministic tie-breaks",
    "refs": [
      "internal/trace/service.go:7-9",
      "internal/trace/service.go",
      "code/golden.md",
      "$REPO/.okf/knowledge"
    ],
    "counts": {
      "returned": 1
    }
  }
]`
	if got != want {
		t.Fatalf("trace golden mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestContextTraceSchemaMatchesGoldenContract(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, "internal", "trace", "context.go"), `package trace

func GoldenContextSymbol() string {
	return "golden"
}
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "context-golden.md"), `---
type: code_symbol
title: GoldenContextSymbol
description: Stable context trace fixture
resource: code://repo/internal/trace/context.go
source_path: internal/trace/context.go
language: go
symbol_kind: function
qualified_name: trace.GoldenContextSymbol
start_line: 3
end_line: 5
---
Stable context trace fixture body.
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Context(context.Background(), ContextRequest{Query: "GoldenContextSymbol", BudgetTokens: 200, IncludeTrace: true})

	if !resp.OK {
		t.Fatalf("context OK = false, error = %#v", resp.Error)
	}
	result := resp.Result.(ContextResult)
	trace := normalizeTraceForGolden(result.Trace, repo)
	data, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}
	got := string(data)
	want := `[
  {
    "type": "path_resolution",
    "message": "resolved knowledge paths",
    "refs": [
      "$REPO/.okf/knowledge"
    ],
    "counts": {
      "read_paths": 1
    }
  },
  {
    "type": "primary_hits",
    "message": "selected primary query hits",
    "refs": [
      "internal/trace/context.go:3-5",
      "internal/trace/context.go",
      "code/context-golden.md",
      "$REPO/.okf/knowledge"
    ],
    "counts": {
      "hits": 1
    }
  },
  {
    "type": "overlay_merge",
    "message": "no overlay merges applied",
    "counts": {
      "collapsed_duplicates": 0
    }
  },
  {
    "type": "relation_expansion",
    "message": "relation expansion disabled",
    "counts": {
      "neighbors": 0
    }
  },
  {
    "type": "range_merge",
    "message": "merged overlapping or adjacent same-file ranges",
    "counts": {
      "merges": 0
    }
  },
  {
    "type": "omission",
    "message": "omitted context candidate",
    "refs": [
      "internal/trace/context.go"
    ],
    "counts": {
      "count": 1
    },
    "omission_reason": "snippet_truncated"
  },
  {
    "type": "snippet_extraction",
    "message": "extracted source snippet",
    "refs": [
      "internal/trace/context.go:2-5",
      "code/context-golden.md",
      "$REPO/.okf/knowledge"
    ],
    "counts": {
      "tokens": 14
    }
  },
  {
    "type": "budget_packing",
    "message": "packed context items under token budget",
    "counts": {
      "budget_tokens": 200,
      "items": 1,
      "omitted": 1,
      "used_tokens": 14
    }
  }
]`
	if got != want {
		t.Fatalf("context trace golden mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestContextReturnsSnippetAndTokenBudget(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, "internal", "alpha", "service.go"), `package alpha

func RouteAlphaSymbol() string {
	return "AlphaSymbol routes requests"
}
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "alpha.md"), `---
type: concept
title: Alpha Service
description: Handles AlphaSymbol routing
resource: internal/alpha/service.go
file_path: internal/alpha/service.go
tags:
  - code
---
AlphaSymbol lives in internal/alpha/service.go and owns request routing.
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Context(context.Background(), ContextRequest{Query: "AlphaSymbol", BudgetTokens: 12})

	if !resp.OK {
		t.Fatalf("context OK = false, error = %#v", resp.Error)
	}
	if resp.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %q, want %q", resp.SchemaVersion, SchemaVersion)
	}
	if resp.Operation != OperationContext {
		t.Fatalf("operation = %q, want %q", resp.Operation, OperationContext)
	}
	result, ok := resp.Result.(ContextResult)
	if !ok {
		t.Fatalf("result type = %T, want ContextResult", resp.Result)
	}
	if result.Query != "AlphaSymbol" {
		t.Fatalf("query = %q, want AlphaSymbol", result.Query)
	}
	if result.BudgetTokens != 12 {
		t.Fatalf("budget_tokens = %d, want 12", result.BudgetTokens)
	}
	if result.UsedTokens > result.BudgetTokens {
		t.Fatalf("used_tokens = %d exceeds budget %d", result.UsedTokens, result.BudgetTokens)
	}
	if len(result.Items) != 1 {
		t.Fatalf("context item count = %d, want 1", len(result.Items))
	}
	item := result.Items[0]
	if item.Title != "Alpha Service" || item.SourcePath != "internal/alpha/service.go" {
		t.Fatalf("item = %#v, want Alpha Service from internal/alpha/service.go", item)
	}
	if item.StartLine != 3 || item.EndLine != 3 {
		t.Fatalf("item lines = %d-%d, want 3-3", item.StartLine, item.EndLine)
	}
	if item.Location != "internal/alpha/service.go:3" {
		t.Fatalf("item location = %q, want internal/alpha/service.go:3", item.Location)
	}
	if item.TokenEstimate > result.BudgetTokens || item.TokenEstimate <= 0 {
		t.Fatalf("item token estimate = %d, want within budget 1..%d", item.TokenEstimate, result.BudgetTokens)
	}
	if item.Snippet == "" || !strings.Contains(item.Snippet, "AlphaSymbol") {
		t.Fatalf("snippet = %q, want AlphaSymbol", item.Snippet)
	}
	if result.Omitted <= 0 {
		t.Fatalf("omitted = %d, want positive because low budget truncates source context", result.Omitted)
	}
	if len(result.Omissions) == 0 || result.Omissions[0].Reason != "snippet_truncated" {
		t.Fatalf("omissions = %#v, want snippet_truncated reason", result.Omissions)
	}
}

func TestContextReportsBudgetExceededOmissionReason(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, "internal", "alpha", "service.go"), "package alpha\nfunc AlphaSymbol() string { return \"alpha\" }\n")
	mustWriteToolFile(t, filepath.Join(repo, "internal", "beta", "service.go"), "package beta\nfunc AlphaSymbolBeta() string { return \"beta AlphaSymbol\" }\n")
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "alpha.md"), `---
type: concept
title: Alpha A
description: AlphaSymbol first result
resource: internal/alpha/service.go
source_path: internal/alpha/service.go
---
AlphaSymbol first result.
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "beta.md"), `---
type: concept
title: Alpha B
description: AlphaSymbol second result
resource: internal/beta/service.go
source_path: internal/beta/service.go
---
AlphaSymbol second result.
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Context(context.Background(), ContextRequest{Query: "AlphaSymbol", BudgetTokens: 8})

	if !resp.OK {
		t.Fatalf("context OK = false, error = %#v", resp.Error)
	}
	result, ok := resp.Result.(ContextResult)
	if !ok {
		t.Fatalf("result type = %T, want ContextResult", resp.Result)
	}
	if len(result.Items) != 1 {
		t.Fatalf("context item count = %d, want 1 item within budget", len(result.Items))
	}
	if result.Omitted == 0 {
		t.Fatal("omitted = 0, want positive")
	}
	if !hasContextOmission(result.Omissions, "budget_exceeded", "internal/beta/service.go") {
		t.Fatalf("omissions = %#v, want budget_exceeded for internal/beta/service.go", result.Omissions)
	}
}

func TestContextWarnsWhenSourceFileMissing(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "missing.md"), `---
type: concept
title: Missing Service
description: Handles MissingSymbol routing
resource: internal/missing/service.go
file_path: internal/missing/service.go
start_line: 9
end_line: 11
---
MissingSymbol was indexed from a file that no longer exists.
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Context(context.Background(), ContextRequest{Query: "MissingSymbol", BudgetTokens: 100})

	if !resp.OK {
		t.Fatalf("context OK = false, error = %#v", resp.Error)
	}
	result, ok := resp.Result.(ContextResult)
	if !ok {
		t.Fatalf("result type = %T, want ContextResult", resp.Result)
	}
	if len(result.Items) != 1 {
		t.Fatalf("context item count = %d, want 1 metadata-only item for missing source", len(result.Items))
	}
	item := result.Items[0]
	if item.Title != "Missing Service" || item.SourcePath != "internal/missing/service.go" {
		t.Fatalf("item = %#v, want Missing Service metadata for internal/missing/service.go", item)
	}
	if item.StartLine != 9 || item.EndLine != 11 {
		t.Fatalf("item lines = %d-%d, want 9-11", item.StartLine, item.EndLine)
	}
	if item.Location != "internal/missing/service.go:9-11" {
		t.Fatalf("item location = %q, want internal/missing/service.go:9-11", item.Location)
	}
	if item.Snippet != "" || item.TokenEstimate != 0 {
		t.Fatalf("snippet/token = %q/%d, want empty snippet and zero tokens for missing source", item.Snippet, item.TokenEstimate)
	}
	if !hasContextOmission(result.Omissions, "source_missing", "internal/missing/service.go") {
		t.Fatalf("omissions = %#v, want source_missing for internal/missing/service.go", result.Omissions)
	}
	if len(resp.Warnings) == 0 {
		t.Fatal("warnings empty, want missing source warning")
	}
	if !strings.Contains(resp.Warnings[0], "internal/missing/service.go") {
		t.Fatalf("warning = %q, want missing source path", resp.Warnings[0])
	}
}

func TestContextOmitsUnsafeSourcePathWithWarningAndTrace(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "unsafe.md"), `---
type: concept
title: Unsafe Service
description: UnsafeSymbol source path escapes repo
source_path: ../outside/service.go
start_line: 1
end_line: 1
---
UnsafeSymbol content.
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Context(context.Background(), ContextRequest{Query: "UnsafeSymbol", BudgetTokens: 100, IncludeTrace: true})

	if !resp.OK {
		t.Fatalf("context OK = false, error = %#v", resp.Error)
	}
	result := resp.Result.(ContextResult)
	if len(result.Items) != 0 {
		t.Fatalf("items = %#v, want unsafe source omitted", result.Items)
	}
	if !hasContextOmission(result.Omissions, "source_path_escapes_repo", "../outside/service.go") {
		t.Fatalf("omissions = %#v, want source_path_escapes_repo", result.Omissions)
	}
	if len(resp.Warnings) == 0 || !strings.Contains(resp.Warnings[0], "../outside/service.go") {
		t.Fatalf("warnings = %#v, want unsafe path warning", resp.Warnings)
	}
	if !traceHasOmission(result.Trace, "source_path_escapes_repo") {
		t.Fatalf("trace = %#v, want source_path_escapes_repo omission", result.Trace)
	}
}

func TestContextOmitsSymlinkSourcePathEscapingRepo(t *testing.T) {
	repo := initToolTestRepo(t)
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.go")
	mustWriteToolFile(t, outsideFile, "package secret\nfunc SecretSymbol() string { return \"leak\" }\n")
	linkPath := filepath.Join(repo, "linked", "secret.go")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		t.Fatalf("mkdir symlink dir: %v", err)
	}
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "linked-secret.md"), `---
type: concept
title: Linked Secret
description: SecretSymbol source path is a symlink escaping repo
source_path: linked/secret.go
start_line: 1
end_line: 2
---
SecretSymbol content.
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Context(context.Background(), ContextRequest{Query: "SecretSymbol", BudgetTokens: 100, IncludeTrace: true})

	if !resp.OK {
		t.Fatalf("context OK = false, error = %#v", resp.Error)
	}
	result := resp.Result.(ContextResult)
	if len(result.Items) != 0 {
		t.Fatalf("items = %#v, want symlink escape omitted without snippet", result.Items)
	}
	if !hasContextOmission(result.Omissions, "source_path_escapes_repo", "linked/secret.go") {
		t.Fatalf("omissions = %#v, want source_path_escapes_repo for symlink", result.Omissions)
	}
	if !traceHasOmission(result.Trace, "source_path_escapes_repo") {
		t.Fatalf("trace = %#v, want source_path_escapes_repo omission", result.Trace)
	}
}

func TestContextPlannerPrefersSymbolRangeOverKeywordSearch(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, "internal", "alpha", "service.go"), `package alpha

// AlphaSymbol appears in an introductory comment that should not drive symbol context.

func RouteAlphaSymbol() string {
	return "route alpha"
}
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "route-alpha.md"), `---
type: code_symbol
title: RouteAlphaSymbol
description: Handles AlphaSymbol routing
resource: code://repo/internal/alpha/service.go
source_path: internal/alpha/service.go
symbol_kind: function
qualified_name: alpha.RouteAlphaSymbol
start_line: 5
end_line: 7
---
RouteAlphaSymbol handles AlphaSymbol routing.
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Context(context.Background(), ContextRequest{Query: "AlphaSymbol", BudgetTokens: 100})

	if !resp.OK {
		t.Fatalf("context OK = false, error = %#v", resp.Error)
	}
	result, ok := resp.Result.(ContextResult)
	if !ok {
		t.Fatalf("result type = %T, want ContextResult", resp.Result)
	}
	if len(result.Items) != 1 {
		t.Fatalf("context item count = %d, want 1", len(result.Items))
	}
	item := result.Items[0]
	if item.StartLine != 4 || item.EndLine != 7 {
		t.Fatalf("item lines = %d-%d, want symbol range with leading context 4-7", item.StartLine, item.EndLine)
	}
	if !strings.Contains(item.Snippet, "func RouteAlphaSymbol") || strings.Contains(item.Snippet, "introductory comment") {
		t.Fatalf("snippet = %q, want symbol body not earlier keyword comment", item.Snippet)
	}
}

func TestContextPlannerMergesAdjacentSameFileRanges(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, "internal", "alpha", "service.go"), `package alpha

func AlphaSymbolA() string {
	return "a"
}
func AlphaSymbolB() string {
	return "b"
}
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "alpha-a.md"), `---
type: code_symbol
title: AlphaSymbolA
description: AlphaSymbol adjacent function A
resource: code://repo/internal/alpha/service.go
source_path: internal/alpha/service.go
symbol_kind: function
qualified_name: alpha.AlphaSymbolA
start_line: 3
end_line: 5
---
AlphaSymbol adjacent function A.
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "alpha-b.md"), `---
type: code_symbol
title: AlphaSymbolB
description: AlphaSymbol adjacent function B
resource: code://repo/internal/alpha/service.go
source_path: internal/alpha/service.go
symbol_kind: function
qualified_name: alpha.AlphaSymbolB
start_line: 6
end_line: 8
---
AlphaSymbol adjacent function B.
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Context(context.Background(), ContextRequest{Query: "AlphaSymbol", BudgetTokens: 200})

	if !resp.OK {
		t.Fatalf("context OK = false, error = %#v", resp.Error)
	}
	result, ok := resp.Result.(ContextResult)
	if !ok {
		t.Fatalf("result type = %T, want ContextResult", resp.Result)
	}
	if len(result.Items) != 1 {
		t.Fatalf("context item count = %d, want adjacent ranges merged into 1 item", len(result.Items))
	}
	item := result.Items[0]
	if item.StartLine != 2 || item.EndLine != 8 {
		t.Fatalf("item lines = %d-%d, want merged range with leading context 2-8", item.StartLine, item.EndLine)
	}
	if !strings.Contains(item.Snippet, "AlphaSymbolA") || !strings.Contains(item.Snippet, "AlphaSymbolB") {
		t.Fatalf("snippet = %q, want both contributing symbols", item.Snippet)
	}
	if !strings.Contains(item.Reason, "merged 2 hits") {
		t.Fatalf("reason = %q, want merged hit references", item.Reason)
	}
}

func TestContextTraceIdentifiesPrimaryHitsAndRangeMergeRefs(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, "internal", "alpha", "service.go"), `package alpha

func AlphaSymbolA() string {
	return "a"
}
func AlphaSymbolB() string {
	return "b"
}
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "alpha-a.md"), `---
type: code_symbol
title: AlphaSymbolA
description: AlphaSymbol adjacent function A
resource: code://repo/internal/alpha/service.go
source_path: internal/alpha/service.go
symbol_kind: function
qualified_name: alpha.AlphaSymbolA
start_line: 3
end_line: 5
---
AlphaSymbol adjacent function A.
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "alpha-b.md"), `---
type: code_symbol
title: AlphaSymbolB
description: AlphaSymbol adjacent function B
resource: code://repo/internal/alpha/service.go
source_path: internal/alpha/service.go
symbol_kind: function
qualified_name: alpha.AlphaSymbolB
start_line: 6
end_line: 8
---
AlphaSymbol adjacent function B.
`)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Context(context.Background(), ContextRequest{Query: "AlphaSymbol", BudgetTokens: 200, IncludeTrace: true})

	if !resp.OK {
		t.Fatalf("context OK = false, error = %#v", resp.Error)
	}
	result := resp.Result.(ContextResult)
	primaryStep, ok := traceStepByType(result.Trace, "primary_hits")
	if !ok {
		t.Fatalf("trace = %#v, want primary_hits step", result.Trace)
	}
	if !traceStepRefsContain(primaryStep, "internal/alpha/service.go:3-5") || !traceStepRefsContain(primaryStep, "internal/alpha/service.go:6-8") {
		t.Fatalf("primary_hits step = %#v, want refs for both selected ranges", primaryStep)
	}
	mergeStep, ok := traceStepByType(result.Trace, "range_merge")
	if !ok {
		t.Fatalf("trace = %#v, want range_merge step", result.Trace)
	}
	if mergeStep.Counts["merges"] != 1 || !traceStepRefsContain(mergeStep, "internal/alpha/service.go:3-8") {
		t.Fatalf("range_merge step = %#v, want one merged source range ref", mergeStep)
	}
	for _, step := range result.Trace {
		if strings.Contains(strings.Join(step.Refs, "\n"), "return \"a\"") || strings.Contains(step.Message, "return \"a\"") {
			t.Fatalf("trace step leaks source content: %#v", step)
		}
	}
}

func TestContextRelationExpansionIsExplicitAndLimited(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, "internal", "alpha", "service.go"), "package alpha\nfunc AlphaSymbol() {}\n")
	mustWriteToolFile(t, filepath.Join(repo, "internal", "beta", "service.go"), "package beta\nfunc BetaNeighbor() {}\n")
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "alpha.md"), `---
type: code_symbol
title: AlphaSymbol
description: AlphaSymbol primary
resource: code://repo/internal/alpha/service.go
source_path: internal/alpha/service.go
symbol_kind: function
qualified_name: alpha.AlphaSymbol
start_line: 2
end_line: 2
---
AlphaSymbol primary.
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "beta.md"), `---
type: code_symbol
title: BetaNeighbor
description: Beta neighbor reached through relation only
resource: code://repo/internal/beta/service.go
source_path: internal/beta/service.go
symbol_kind: function
qualified_name: beta.BetaNeighbor
start_line: 2
end_line: 2
---
Beta neighbor reached through relation only.
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "alpha-imports-beta.md"), `---
type: code_relation
title: Alpha imports Beta
description: Alpha to Beta relation
resource: code://repo/internal/alpha/service.go
source_path: internal/alpha/service.go
relation_kind: file_import
relation_source: internal/alpha/service.go
relation_target: internal/beta/service.go
start_line: 2
end_line: 2
---
Alpha imports Beta.
`)

	svc := NewService(Config{RepoPath: repo})
	defaultResp := svc.Context(context.Background(), ContextRequest{Query: "AlphaSymbol", BudgetTokens: 200})
	if !defaultResp.OK {
		t.Fatalf("default context OK = false, error = %#v", defaultResp.Error)
	}
	defaultResult := defaultResp.Result.(ContextResult)
	if hasContextItem(defaultResult.Items, "internal/beta/service.go") {
		t.Fatalf("default context items = %#v, want relation neighbor omitted unless explicitly enabled", defaultResult.Items)
	}

	relationResp := svc.Context(context.Background(), ContextRequest{Query: "AlphaSymbol", BudgetTokens: 200, IncludeRelations: true})
	if !relationResp.OK {
		t.Fatalf("relation context OK = false, error = %#v", relationResp.Error)
	}
	relationResult := relationResp.Result.(ContextResult)
	item, ok := contextItemByPath(relationResult.Items, "internal/beta/service.go")
	if !ok {
		t.Fatalf("relation context items = %#v, want beta neighbor", relationResult.Items)
	}
	if !strings.Contains(item.Reason, "relation file_import from internal/alpha/service.go") {
		t.Fatalf("neighbor reason = %q, want relation explanation", item.Reason)
	}
}

func TestContextTraceIsDefaultOffAndCompactWhenEnabled(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, "internal", "alpha", "service.go"), "package alpha\nfunc AlphaSymbol() {}\n")
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "alpha.md"), `---
type: code_symbol
title: AlphaSymbol
description: AlphaSymbol primary
resource: code://repo/internal/alpha/service.go
source_path: internal/alpha/service.go
symbol_kind: function
qualified_name: alpha.AlphaSymbol
start_line: 2
end_line: 2
---
AlphaSymbol primary.
`)

	svc := NewService(Config{RepoPath: repo})
	defaultResp := svc.Context(context.Background(), ContextRequest{Query: "AlphaSymbol", BudgetTokens: 100})
	if !defaultResp.OK {
		t.Fatalf("default context OK = false, error = %#v", defaultResp.Error)
	}
	defaultResult := defaultResp.Result.(ContextResult)
	if defaultResult.Trace != nil {
		t.Fatalf("trace = %#v, want omitted by default", defaultResult.Trace)
	}

	traceResp := svc.Context(context.Background(), ContextRequest{Query: "AlphaSymbol", BudgetTokens: 100, IncludeTrace: true})
	if !traceResp.OK {
		t.Fatalf("trace context OK = false, error = %#v", traceResp.Error)
	}
	traceResult := traceResp.Result.(ContextResult)
	if len(traceResult.Trace) == 0 {
		t.Fatal("trace empty, want compact planner steps")
	}
	if traceResult.Trace[0].Type != "path_resolution" || traceResult.Trace[0].Counts["read_paths"] == 0 {
		t.Fatalf("first trace step = %#v, want path_resolution with read path count", traceResult.Trace[0])
	}
	for _, step := range traceResult.Trace {
		if strings.Contains(strings.Join(step.Refs, "\n"), "func AlphaSymbol") || strings.Contains(step.Message, "func AlphaSymbol") {
			t.Fatalf("trace step leaks source content: %#v", step)
		}
	}
}

func TestExtractSnippetKeepsQueryWhenBudgetTruncatesLongLine(t *testing.T) {
	snippet, startLine, endLine, omitted := extractSnippet(
		"package alpha\n"+strings.Repeat("prefix ", 20)+"AlphaSymbol keeps routing context\n",
		"AlphaSymbol",
		4,
	)

	if !strings.Contains(snippet, "AlphaSymbol") {
		t.Fatalf("snippet = %q, want to preserve query after truncation", snippet)
	}
	if estimateTokens(snippet) > 4 {
		t.Fatalf("snippet token estimate = %d, want <= 4", estimateTokens(snippet))
	}
	if startLine != 2 || endLine != 2 {
		t.Fatalf("lines = %d-%d, want 2-2", startLine, endLine)
	}
	if omitted != 1 {
		t.Fatalf("omitted = %d, want 1 non-empty line outside snippet", omitted)
	}
}

func BenchmarkToolQueryAndContextNamedFixture(b *testing.B) {
	repo := initToolBenchmarkRepo(b)
	for i := 0; i < 20; i++ {
		mustWriteBenchmarkFile(b, filepath.Join(repo, ".okf", "knowledge", "code", fmt.Sprintf("symbol-%02d.md", i)), strings.ReplaceAll(`---
type: code_symbol
title: BenchSymbolNN
description: BenchQueryToken benchmark symbol NN
resource: code://repo/internal/bench/service.go
source_path: internal/bench/service.go
language: go
symbol_kind: function
qualified_name: bench.BenchSymbolNN
start_line: 3
end_line: 5
generated: true
generator: okf.git
---
BenchQueryToken benchmark generated concept NN.
`, "NN", fmt.Sprintf("%02d", i)))
	}
	overlayDir := filepath.Join(b.TempDir(), "overlay", "knowledge")
	mustWriteBenchmarkFile(b, filepath.Join(overlayDir, "concepts", "bench-overlay.md"), `---
type: concept
title: Bench Overlay Note
description: BenchOverlayToken user overlay note
---
BenchOverlayToken validates overlay reads in the benchmark fixture.
`)
	configPath := filepath.Join(b.TempDir(), "config.yaml")
	if err := okf.SaveConfig(&okf.Config{KnowledgePaths: []string{overlayDir}}, configPath); err != nil {
		b.Fatalf("SaveConfig() error = %v", err)
	}
	b.Setenv("OKF_CONFIG_PATH", configPath)
	svc := NewService(Config{RepoPath: repo})

	b.Run("query/bench-fixture", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			resp := svc.Query(context.Background(), QueryRequest{Query: "BenchQueryToken", Limit: 5, IncludeTrace: true})
			if !resp.OK {
				b.Fatalf("query OK = false, error = %#v", resp.Error)
			}
		}
	})
	b.Run("context/bench-fixture", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			resp := svc.Context(context.Background(), ContextRequest{Query: "BenchQueryToken", BudgetTokens: 200, IncludeTrace: true})
			if !resp.OK {
				b.Fatalf("context OK = false, error = %#v", resp.Error)
			}
		}
	})
}

func hasContextOmission(omissions []ContextOmission, reason, sourcePath string) bool {
	for _, omission := range omissions {
		if omission.Reason == reason && omission.SourcePath == sourcePath {
			return true
		}
	}
	return false
}

func hasContextItem(items []ContextItem, sourcePath string) bool {
	_, ok := contextItemByPath(items, sourcePath)
	return ok
}

func contextItemByPath(items []ContextItem, sourcePath string) (ContextItem, bool) {
	for _, item := range items {
		if item.SourcePath == sourcePath {
			return item, true
		}
	}
	return ContextItem{}, false
}

func traceRefsContain(trace []TraceStep, want string) bool {
	for _, step := range trace {
		for _, ref := range step.Refs {
			if ref == want {
				return true
			}
		}
	}
	return false
}

func traceHasOmission(trace []TraceStep, reason string) bool {
	for _, step := range trace {
		if step.OmissionReason == reason {
			return true
		}
	}
	return false
}

func traceStepByType(trace []TraceStep, stepType string) (TraceStep, bool) {
	for _, step := range trace {
		if step.Type == stepType {
			return step, true
		}
	}
	return TraceStep{}, false
}

func traceStepRefsContain(step TraceStep, want string) bool {
	for _, ref := range step.Refs {
		if ref == want {
			return true
		}
	}
	return false
}

func normalizeTraceForGolden(trace []TraceStep, repo string) []TraceStep {
	normalized := make([]TraceStep, 0, len(trace))
	repo = filepath.Clean(repo)
	if canonical, err := filepath.EvalSymlinks(repo); err == nil {
		repo = filepath.Clean(canonical)
	}
	for _, step := range trace {
		step.Refs = append([]string{}, step.Refs...)
		for i, ref := range step.Refs {
			cleanRef := filepath.Clean(ref)
			if filepath.IsAbs(cleanRef) {
				if canonical, err := filepath.EvalSymlinks(cleanRef); err == nil {
					cleanRef = filepath.Clean(canonical)
				}
			}
			step.Refs[i] = strings.ReplaceAll(cleanRef, repo, "$REPO")
		}
		normalized = append(normalized, step)
	}
	return normalized
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func initToolTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runToolGit(t, dir, "init")
	runToolGit(t, dir, "config", "user.name", "Test User")
	runToolGit(t, dir, "config", "user.email", "test@example.com")
	mustWriteToolFile(t, filepath.Join(dir, "README.md"), "# test repo\n")
	runToolGit(t, dir, "add", "README.md")
	runToolGit(t, dir, "commit", "-m", "initial")
	return dir
}

func runToolGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func initToolBenchmarkRepo(b *testing.B) string {
	b.Helper()
	dir := b.TempDir()
	runToolGitB(b, dir, "init")
	runToolGitB(b, dir, "config", "user.name", "Benchmark User")
	runToolGitB(b, dir, "config", "user.email", "benchmark@example.com")
	mustWriteBenchmarkFile(b, filepath.Join(dir, "README.md"), "# benchmark repo\n")
	mustWriteBenchmarkFile(b, filepath.Join(dir, "internal", "bench", "service.go"), `package bench

func BenchSymbol00() string { return "bench" }
func BenchSymbol01() string { return "bench" }
`)
	runToolGitB(b, dir, "add", ".")
	runToolGitB(b, dir, "commit", "-m", "initial benchmark fixture")
	return dir
}

func runToolGitB(b *testing.B, dir string, args ...string) {
	b.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		b.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func mustWriteToolFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustWriteBenchmarkFile(b *testing.B, path, content string) {
	b.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		b.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		b.Fatalf("write %s: %v", path, err)
	}
}

func canonicalPath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("eval symlinks for %s: %v", path, err)
	}
	return canonical
}
