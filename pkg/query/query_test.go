package query

import (
	"fmt"
	"testing"
)

var (
	benchmarkConceptsSink      []*Concept
	benchmarkSearchResultsSink []SearchResult
)

func TestQueryMatchesCombinedCriteria(t *testing.T) {
	t.Parallel()

	bundle := &KnowledgeBundle{Concepts: []*Concept{
		{Type: "api", Title: "Users API", Description: "Handles user accounts", Resource: "api/users", Tags: []string{"go", "production"}, Content: "GET /users"},
		{Type: "table", Title: "Users Table", Description: "Stores user accounts", Resource: "db/users", Tags: []string{"sql"}, Content: "CREATE TABLE users"},
		{Type: "api", Title: "Orders API", Description: "Handles orders", Resource: "api/orders", Tags: []string{"go"}, Content: "GET /orders"},
	}}

	q := New().
		WithType("api").
		WithTags("go", "production").
		WithResource("users").
		WithText("accounts").
		WithTitleRegex(`Users`).
		WithDescriptionRegex(`user`).
		WithContentRegex(`/users`).
		Build()

	results := q.Execute(bundle)
	if len(results) != 1 || results[0].Title != "Users API" {
		t.Fatalf("results = %#v, want only Users API", results)
	}
}

func TestSearchAndFilters(t *testing.T) {
	t.Parallel()

	bundle := &KnowledgeBundle{Concepts: []*Concept{
		{Type: "api", Title: "Users API", Tags: []string{"go"}, Content: "REST endpoint"},
		{Type: "metric", Title: "Latency", Tags: []string{"slo"}, Content: "p95 latency"},
	}}

	if got := Search(bundle, "rest"); len(got) != 1 || got[0].Title != "Users API" {
		t.Fatalf("Search returned %#v", got)
	}
	if got := FilterByType(bundle, "metric"); len(got) != 1 || got[0].Title != "Latency" {
		t.Fatalf("FilterByType returned %#v", got)
	}
	if got := FilterByTag(bundle, "go"); len(got) != 1 || got[0].Title != "Users API" {
		t.Fatalf("FilterByTag returned %#v", got)
	}
}

func TestSearchWithMatchesReturnsSymbolKindAndLocation(t *testing.T) {
	t.Parallel()

	bundle := &KnowledgeBundle{Concepts: []*Concept{
		{
			Type:     "component",
			Title:    "server.go",
			Resource: "server.go",
			Content:  "## File: `server.go`\n\n### Symbols\n\n- `function` `StartServer` (exported) at `server.go:12-18`\n- `method` `App.Stop` (exported) at `server.go:20-22`\n",
		},
		{
			Type:     "component",
			Title:    "client.go",
			Resource: "client.go",
			Content:  "## File: `client.go`\n\n### Symbols\n\n- `function` `Connect` (exported) at `client.go:5-8`\n",
		},
	}}

	results := SearchWithMatches(bundle, "StartServer")
	if len(results) != 1 {
		t.Fatalf("SearchWithMatches returned %d results, want 1: %#v", len(results), results)
	}
	if results[0].Concept.Title != "server.go" {
		t.Fatalf("matched concept = %q, want server.go", results[0].Concept.Title)
	}
	if len(results[0].SymbolMatches) != 1 {
		t.Fatalf("symbol matches = %#v, want one symbol match", results[0].SymbolMatches)
	}
	symbol := results[0].SymbolMatches[0]
	if symbol.Kind != "function" || symbol.Name != "StartServer" || symbol.Location != "server.go:12-18" {
		t.Fatalf("symbol match = %#v, want function StartServer at server.go:12-18", symbol)
	}
}

func TestBuildIndexPopulatesStructuredLookupMaps(t *testing.T) {
	t.Parallel()

	bundle := &KnowledgeBundle{Concepts: []*Concept{
		{
			Type:     "component",
			Title:    "Server",
			Resource: "cmd/server.go",
			Tags:     []string{"go", "cli"},
			Content:  "### Symbols\n\n- `function` `StartServer` (exported) at `cmd/server.go:12-18`\n",
		},
		{
			Type:     "doc",
			Title:    "README",
			Resource: "README.md",
			Tags:     []string{"docs"},
			Content:  "Usage guide",
		},
	}}

	bundle.BuildIndex()

	if bundle.index == nil {
		t.Fatal("BuildIndex did not attach an index to the bundle")
	}
	if got := bundle.index.byType["component"]; len(got) != 1 || got[0].Title != "Server" {
		t.Fatalf("type index = %#v, want Server component", got)
	}
	if got := bundle.index.byTag["go"]; len(got) != 1 || got[0].Title != "Server" {
		t.Fatalf("tag index = %#v, want Server tagged go", got)
	}
	if got := bundle.index.byResource["README.md"]; len(got) != 1 || got[0].Title != "README" {
		t.Fatalf("resource index = %#v, want README", got)
	}
	if got := bundle.index.byTitle["server"]; len(got) != 1 || got[0].Title != "Server" {
		t.Fatalf("title index = %#v, want case-folded Server", got)
	}
	if got := bundle.index.symbolsByName["startserver"]; len(got) != 1 || got[0].match.Location != "cmd/server.go:12-18" {
		t.Fatalf("symbol index = %#v, want StartServer location", got)
	}
}

func TestIndexedSearchPreservesFreeTextSemantics(t *testing.T) {
	t.Parallel()

	bundle := &KnowledgeBundle{Concepts: []*Concept{
		{Type: "component", Title: "Server", Description: "HTTP gateway", Resource: "server.go", Tags: []string{"go"}, Content: "Listens for requests"},
		{Type: "component", Title: "Client", Description: "SDK", Resource: "client.go", Tags: []string{"go"}, Content: "Calls the gateway"},
		{Type: "doc", Title: "Guide", Description: "Usage", Resource: "README.md", Tags: []string{"docs"}, Content: "Run okf init"},
	}}
	bundle.BuildIndex()

	for _, tc := range []struct {
		name string
		text string
		want []string
	}{
		{name: "title substring", text: "serv", want: []string{"Server"}},
		{name: "description substring", text: "gate", want: []string{"Server", "Client"}},
		{name: "content substring", text: "OKF", want: []string{"Guide"}},
		{name: "empty text returns all", text: "", want: []string{"Server", "Client", "Guide"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Search(bundle, tc.text)
			assertConceptTitles(t, got, tc.want)
		})
	}
}

func TestIndexedSearchSeesConceptsAppendedAfterInitialQuery(t *testing.T) {
	t.Parallel()

	bundle := &KnowledgeBundle{Concepts: []*Concept{
		{Type: "component", Title: "Server", Tags: []string{"go"}, Content: "server content"},
	}}
	if got := FilterByType(bundle, "component"); len(got) != 1 {
		t.Fatalf("initial FilterByType returned %d concepts, want 1", len(got))
	}

	bundle.Concepts = append(bundle.Concepts, &Concept{Type: "component", Title: "Client", Tags: []string{"go"}, Content: "client content"})

	if got := FilterByType(bundle, "component"); len(got) != 2 {
		t.Fatalf("FilterByType after append returned %d concepts, want 2", len(got))
	}
	if got := Search(bundle, "client"); len(got) != 1 || got[0].Title != "Client" {
		t.Fatalf("Search after append returned %#v, want Client", got)
	}
}

func TestIndexedSearchSeesConceptFieldMutationsAfterInitialQuery(t *testing.T) {
	t.Parallel()

	concept := &Concept{Type: "component", Title: "Server", Tags: []string{"old"}, Resource: "server.go", Content: "server content"}
	bundle := &KnowledgeBundle{Concepts: []*Concept{concept}}
	if got := Search(bundle, "server"); len(got) != 1 {
		t.Fatalf("initial Search returned %d concepts, want 1", len(got))
	}

	concept.Title = "Client"
	concept.Tags = []string{"new"}
	concept.Resource = "client.go"
	concept.Content = "### Symbols\n\n- `function` `Connect` (exported) at `client.go:3-4`\n"

	if got := Search(bundle, "client"); len(got) != 1 || got[0].Title != "Client" {
		t.Fatalf("Search after title mutation returned %#v, want Client", got)
	}
	if got := FilterByTag(bundle, "new"); len(got) != 1 || got[0].Title != "Client" {
		t.Fatalf("FilterByTag after tag mutation returned %#v, want Client", got)
	}
	if got := SearchWithMatches(bundle, "Connect"); len(got) != 1 || len(got[0].SymbolMatches) != 1 {
		t.Fatalf("SearchWithMatches after content mutation returned %#v, want Connect symbol", got)
	}
}

func TestIndexedSearchSeesCustomFieldMutationsAfterInitialQuery(t *testing.T) {
	t.Parallel()

	concept := &Concept{
		Type:         "code_symbol",
		Title:        "RouteAlphaSymbol",
		CustomFields: map[string]interface{}{"qualified_name": "alpha.OldSymbol"},
		Content:      "RouteAlphaSymbol content",
	}
	bundle := &KnowledgeBundle{Concepts: []*Concept{concept}}
	if got := New().WithCodeQualifiedName("alpha.OldSymbol").Build().Execute(bundle); len(got) != 1 {
		t.Fatalf("initial custom field query returned %d concepts, want 1", len(got))
	}

	concept.CustomFields["source_path"] = "internal/alpha/service.go"
	concept.CustomFields["language"] = "go"
	concept.CustomFields["symbol_kind"] = "function"
	concept.CustomFields["qualified_name"] = "alpha.RouteAlphaSymbol"
	concept.CustomFields["relation_kind"] = "file_import"
	concept.CustomFields["relation_source"] = "internal/alpha/service.go"
	concept.CustomFields["relation_target"] = "internal/beta/service.go"

	q := New().
		WithCodeFilePath("internal/alpha/service.go").
		WithCodeLanguage("go").
		WithCodeSymbolKind("function").
		WithCodeQualifiedName("alpha.RouteAlphaSymbol").
		WithCodeRelationKind("file_import").
		WithCodeRelationSource("internal/alpha/service.go").
		WithCodeRelationTarget("internal/beta/service.go").
		Build()
	got := q.Execute(bundle)
	assertConceptTitles(t, got, []string{"RouteAlphaSymbol"})
}

func TestQueryCodeDimensionFilters(t *testing.T) {
	t.Parallel()

	bundle := &KnowledgeBundle{Concepts: []*Concept{
		{
			Type:     "code_file",
			Title:    "server.go",
			Resource: "code://repo/cmd/server.go",
			Tags:     []string{"code", "generated", "go"},
			Content:  "## Source\n\n- Path: `cmd/server.go`\n- Language: `go`\n\n### Symbols\n\n- `function` `main.StartServer` (exported) at `cmd/server.go:10-20`\n",
		},
		{
			Type:     "code_file",
			Title:    "client.ts",
			Resource: "code://repo/web/client.ts",
			Tags:     []string{"code", "generated", "typescript"},
			Content:  "## Source\n\n- Path: `web/client.ts`\n- Language: `typescript`\n\n### Symbols\n\n- `component` `ClientView` (exported) at `web/client.ts:3-15`\n",
		},
		{
			Type:     "code_relation_index",
			Title:    "Code Relation Index",
			Resource: "okf://code/relations",
			Tags:     []string{"code", "relations", "generated"},
			Content:  "## Code Relation Index\n\n| Kind | Source | Target | Location | Provenance |\n| --- | --- | --- | --- | --- |\n| calls | `code:repo:cmd/server.go#symbol:function:main.StartServer@L10-L20` | `code:repo:cmd/log.go#symbol:function:main.Log@L2-L4` | `cmd/server.go:12` | codegraph |\n",
		},
	}}

	tests := []struct {
		name string
		q    *Query
		want []string
	}{
		{name: "language", q: New().WithCodeLanguage("go").Build(), want: []string{"server.go"}},
		{name: "file path", q: New().WithCodeFilePath("web/client.ts").Build(), want: []string{"client.ts"}},
		{name: "symbol kind", q: New().WithCodeSymbolKind("component").Build(), want: []string{"client.ts"}},
		{name: "qualified name", q: New().WithCodeQualifiedName("main.StartServer").Build(), want: []string{"server.go", "Code Relation Index"}},
		{name: "relation kind", q: New().WithCodeRelationKind("calls").Build(), want: []string{"Code Relation Index"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.q.Execute(bundle)
			assertConceptTitles(t, got, tt.want)
		})
	}
}

func TestCodeMetadataFiltersUseFrontmatterCustomFields(t *testing.T) {
	t.Parallel()

	bundle := &KnowledgeBundle{Concepts: []*Concept{
		{
			Type:     "code_symbol",
			Title:    "RouteAlphaSymbol",
			Resource: "code://repo/internal/alpha/service.go",
			CustomFields: map[string]interface{}{
				"source_path":     "internal/alpha/service.go",
				"language":        "go",
				"symbol_kind":     "function",
				"qualified_name":  "alpha.RouteAlphaSymbol",
				"relation_kind":   "file_import",
				"relation_source": "internal/alpha/service.go",
				"relation_target": "internal/beta/service.go",
			},
			Content: "RouteAlphaSymbol generated concept with structured frontmatter.",
		},
		{
			Type:     "code_symbol",
			Title:    "RouteAlphaSymbolBeta",
			Resource: "code://repo/internal/beta/service.go",
			CustomFields: map[string]interface{}{
				"source_path":     "internal/beta/service.go",
				"language":        "go",
				"symbol_kind":     "function",
				"qualified_name":  "beta.RouteAlphaSymbolBeta",
				"relation_kind":   "file_import",
				"relation_source": "internal/beta/service.go",
				"relation_target": "internal/gamma/service.go",
			},
			Content: "RouteAlphaSymbol generated concept with different structured frontmatter.",
		},
	}}

	q := New().
		WithCodeLanguage("go").
		WithCodeFilePath("internal/alpha/service.go").
		WithCodeSymbolKind("function").
		WithCodeQualifiedName("alpha.RouteAlphaSymbol").
		WithCodeRelationKind("file_import").
		WithCodeRelationSource("internal/alpha/service.go").
		WithCodeRelationTarget("internal/beta/service.go").
		Build()

	got := q.Execute(bundle)
	assertConceptTitles(t, got, []string{"RouteAlphaSymbol"})
}

func assertConceptTitles(t *testing.T, got []*Concept, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d concepts %#v, want titles %v", len(got), got, want)
	}
	for i := range want {
		if got[i].Title != want[i] {
			t.Fatalf("got titles %#v, want %v", got, want)
		}
	}
}

func BenchmarkQueryIndexedPath(b *testing.B) {
	bundle := benchmarkQueryBundle(1000)
	bundle.BuildIndex()
	q := New().WithType("component").WithTags("go", "indexed").WithResource("file_0999.go").Build()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkConceptsSink = q.Execute(bundle)
	}
}

func BenchmarkQueryFreeTextPath(b *testing.B) {
	bundle := benchmarkQueryBundle(1000)
	bundle.BuildIndex()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkConceptsSink = Search(bundle, "rare phrase 0999")
	}
}

func BenchmarkSearchWithSymbolMatches(b *testing.B) {
	bundle := benchmarkQueryBundle(1000)
	bundle.BuildIndex()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkSearchResultsSink = SearchWithMatches(bundle, "Symbol0999")
	}
}

func benchmarkQueryBundle(count int) *KnowledgeBundle {
	concepts := make([]*Concept, 0, count)
	for i := 0; i < count; i++ {
		concepts = append(concepts, &Concept{
			Type:        "component",
			Title:       fmt.Sprintf("file_%04d.go", i),
			Description: fmt.Sprintf("Generated component %04d", i),
			Resource:    fmt.Sprintf("src/file_%04d.go", i),
			Tags:        []string{"go", "indexed"},
			Content:     fmt.Sprintf("Package code with rare phrase %04d\n\n### Symbols\n\n- `function` `Symbol%04d` (exported) at `src/file_%04d.go:10-12`\n", i, i, i),
		})
	}
	return &KnowledgeBundle{Concepts: concepts}
}
