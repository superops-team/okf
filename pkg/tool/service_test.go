package tool

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

func hasContextOmission(omissions []ContextOmission, reason, sourcePath string) bool {
	for _, omission := range omissions {
		if omission.Reason == reason && omission.SourcePath == sourcePath {
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

func mustWriteToolFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
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
