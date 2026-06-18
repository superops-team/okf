// Package tool exposes OKF repository knowledge operations through a stable,
// agent-facing service API.
package tool

import (
	stdctx "context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/superops-team/okf/pkg/git"
	"github.com/superops-team/okf/pkg/okf"
	querypkg "github.com/superops-team/okf/pkg/query"
)

const SchemaVersion = "okf.tool.v1"

const (
	OperationInit    = "init"
	OperationRefresh = "refresh"
	OperationStatus  = "status"
	OperationQuery   = "query"
	OperationContext = "context"
)

const (
	ErrKnowledgeNotInitialized = "knowledge_not_initialized"
	ErrNotGitRepository        = "not_git_repository"
	ErrInvalidRequest          = "invalid_request"
	ErrInvalidQuery            = "invalid_query"
)

const (
	RefreshModeIncremental = "incremental"
	RefreshModeFull        = "full"
	RefreshModeCacheOnly   = "cache-only"
)

const (
	defaultContextBudgetTokens = 4000
	maxContextCandidates       = 50
	maxContextSourceBytes      = 256 * 1024
	contextNeighborLimit       = 3
	contextSurroundingLines    = 1
)

// Config controls repository knowledge service behavior.
type Config struct {
	RepoPath     string
	KnowledgeDir string
}

// Service serves OKF tool operations for one repository.
type Service struct {
	cfg Config
}

// NewService constructs a Service with OKF defaults.
func NewService(cfg Config) *Service {
	return &Service{cfg: cfg}
}

// ToolEnvelope is the stable top-level response contract for agent tools.
type ToolEnvelope struct {
	SchemaVersion string      `json:"schema_version"`
	Operation     string      `json:"operation"`
	OK            bool        `json:"ok"`
	Mutating      bool        `json:"mutating"`
	RepoRoot      string      `json:"repo_root,omitempty"`
	KnowledgeDir  string      `json:"knowledge_dir,omitempty"`
	Freshness     *Freshness  `json:"freshness,omitempty"`
	Warnings      []string    `json:"warnings"`
	Result        interface{} `json:"result,omitempty"`
	Error         *ToolError  `json:"error,omitempty"`
}

// ToolError describes stable machine-readable failures.
type ToolError struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

// Freshness reports whether loaded knowledge matches the current repository HEAD.
type Freshness struct {
	Head              string `json:"head,omitempty"`
	LastIndexedCommit string `json:"last_indexed_commit,omitempty"`
	Stale             bool   `json:"stale"`
	ChangedFiles      int    `json:"changed_files,omitempty"`
}

type StatusRequest struct{}

type InitRequest struct{}

type RefreshRequest struct {
	Mode string `json:"mode"`
}

type QueryRequest struct {
	Query          string `json:"query"`
	Limit          int    `json:"limit,omitempty"`
	Type           string `json:"type,omitempty"`
	Tag            string `json:"tag,omitempty"`
	FilePath       string `json:"file_path,omitempty"`
	Language       string `json:"language,omitempty"`
	SymbolKind     string `json:"symbol_kind,omitempty"`
	QualifiedName  string `json:"qualified_name,omitempty"`
	RelationKind   string `json:"relation_kind,omitempty"`
	RelationSource string `json:"relation_source,omitempty"`
	RelationTarget string `json:"relation_target,omitempty"`
	IncludeTrace   bool   `json:"include_trace,omitempty"`
}

type ContextRequest struct {
	Query            string `json:"query"`
	BudgetTokens     int    `json:"budget_tokens,omitempty"`
	IncludeRelations bool   `json:"include_relations,omitempty"`
	IncludeTrace     bool   `json:"include_trace,omitempty"`
}

type StatusResult struct {
	Ready         bool                `json:"ready"`
	ConceptCount  int                 `json:"concept_count"`
	UniqueTypes   int                 `json:"unique_types"`
	UniqueTags    int                 `json:"unique_tags"`
	KnowledgePath KnowledgePathStatus `json:"knowledge_path,omitempty"`
}

type InitResult struct {
	ConceptCount int `json:"concept_count"`
	SavedCount   int `json:"saved_count"`
}

type RefreshResult struct {
	Mode           string `json:"mode"`
	GeneratedCount int    `json:"generated_count,omitempty"`
	SavedCount     int    `json:"saved_count,omitempty"`
	UpdatedCount   int    `json:"updated_count,omitempty"`
	RebuiltCache   bool   `json:"rebuilt_cache"`
}

type QueryResult struct {
	Query   string      `json:"query"`
	Results []QueryHit  `json:"results"`
	Trace   []TraceStep `json:"trace,omitempty"`
}

type ContextResult struct {
	Query        string            `json:"query"`
	BudgetTokens int               `json:"budget_tokens"`
	UsedTokens   int               `json:"used_tokens"`
	Omitted      int               `json:"omitted"`
	Omissions    []ContextOmission `json:"omissions"`
	Items        []ContextItem     `json:"items"`
	Trace        []TraceStep       `json:"trace,omitempty"`
}

type TraceStep struct {
	Type           string         `json:"type"`
	Message        string         `json:"message"`
	Refs           []string       `json:"refs,omitempty"`
	Counts         map[string]int `json:"counts,omitempty"`
	ScoreDelta     int            `json:"score_delta,omitempty"`
	OmissionReason string         `json:"omission_reason,omitempty"`
	Warnings       []string       `json:"warnings,omitempty"`
}

type ContextOmission struct {
	Reason     string `json:"reason"`
	Title      string `json:"title,omitempty"`
	SourcePath string `json:"source_path,omitempty"`
	Count      int    `json:"count,omitempty"`
}

type ContextItem struct {
	Title         string `json:"title"`
	Type          string `json:"type"`
	SourcePath    string `json:"source_path"`
	Location      string `json:"location,omitempty"`
	StartLine     int    `json:"start_line"`
	EndLine       int    `json:"end_line"`
	Snippet       string `json:"snippet"`
	TokenEstimate int    `json:"token_estimate"`
	Score         int    `json:"score"`
	Reason        string `json:"reason"`
	Provenance    string `json:"provenance"`
}

type KnowledgePathStatus struct {
	WritePath   string                    `json:"write_path"`
	WriteSource string                    `json:"write_source,omitempty"`
	ReadPaths   []KnowledgePathResolution `json:"read_paths,omitempty"`
}

type KnowledgePathResolution struct {
	Path   string `json:"path"`
	Source string `json:"source,omitempty"`
	Rank   int    `json:"rank"`
}

type overlayMergeDecision struct {
	PrimaryPath string
	MergedPaths []string
	Count       int
}

type bundleLoadMetadata struct {
	Warnings      []string
	OverlayMerges []overlayMergeDecision
}

type QueryHit struct {
	Title               string                    `json:"title"`
	Type                string                    `json:"type"`
	Resource            string                    `json:"resource,omitempty"`
	FilePath            string                    `json:"file_path,omitempty"`
	SourcePath          string                    `json:"source_path,omitempty"`
	Location            string                    `json:"location,omitempty"`
	ConceptPath         string                    `json:"concept_path,omitempty"`
	StartLine           int                       `json:"start_line,omitempty"`
	EndLine             int                       `json:"end_line,omitempty"`
	SymbolKind          string                    `json:"symbol_kind,omitempty"`
	QualifiedName       string                    `json:"qualified_name,omitempty"`
	RelationKind        string                    `json:"relation_kind,omitempty"`
	RelationSource      string                    `json:"relation_source,omitempty"`
	RelationTarget      string                    `json:"relation_target,omitempty"`
	Score               int                       `json:"score"`
	Reason              string                    `json:"reason"`
	Provenance          string                    `json:"provenance"`
	Generated           bool                      `json:"generated,omitempty"`
	Generator           string                    `json:"generator,omitempty"`
	KnowledgePath       string                    `json:"knowledge_path,omitempty"`
	KnowledgePathSource string                    `json:"knowledge_path_source,omitempty"`
	SourceRank          int                       `json:"source_rank"`
	DuplicateSources    []KnowledgePathResolution `json:"duplicate_sources,omitempty"`
	exactness           int
	sourceRank          int
	typeRank            int
}

// Status reports repository knowledge readiness without mutating files.
func (s *Service) Status(_ stdctx.Context, _ StatusRequest) ToolEnvelope {
	resolved, err := s.resolve()
	if err != nil {
		return failure(OperationStatus, "", "", nil, err)
	}

	freshness := readFreshness(resolved)
	if !okf.Exists(resolved.knowledgeDir) {
		return failure(
			OperationStatus,
			resolved.repoRoot,
			resolved.knowledgeDir,
			freshness,
			knowledgeNotInitialized(resolved.repoRoot),
		)
	}

	bundle, loadMeta, err := loadKnowledgeBundle(resolved)
	if err != nil {
		return failure(OperationStatus, resolved.repoRoot, resolved.knowledgeDir, freshness, err)
	}
	stats := bundle.Stats()

	return ToolEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     OperationStatus,
		OK:            true,
		Mutating:      isMutatingOperation(OperationStatus),
		RepoRoot:      resolved.repoRoot,
		KnowledgeDir:  resolved.knowledgeDir,
		Freshness:     freshness,
		Warnings:      loadMeta.Warnings,
		Result: StatusResult{
			Ready:         true,
			ConceptCount:  stats.TotalConcepts,
			UniqueTypes:   stats.UniqueTypes,
			UniqueTags:    stats.UniqueTags,
			KnowledgePath: knowledgePathStatus(resolved),
		},
	}
}

// Init explicitly creates generated repository knowledge.
func (s *Service) Init(_ stdctx.Context, _ InitRequest) ToolEnvelope {
	resolved, err := s.resolve()
	if err != nil {
		return failure(OperationInit, "", "", nil, err)
	}

	cfg := git.DefaultConfig()
	cfg.RepoPath = resolved.repoRoot
	cfg.KnowledgeDir = knowledgeDirForGitConfig(resolved.repoRoot, resolved.knowledgeDir)
	bundle, err := git.GenerateBundle(cfg, true)
	if err != nil {
		return failure(OperationInit, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), err)
	}
	saved, err := git.SaveKnowledgeBase(bundle, cfg)
	if err != nil {
		return failure(OperationInit, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), err)
	}

	return ToolEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     OperationInit,
		OK:            true,
		Mutating:      isMutatingOperation(OperationInit),
		RepoRoot:      resolved.repoRoot,
		KnowledgeDir:  resolved.knowledgeDir,
		Freshness:     readFreshness(resolved),
		Warnings:      []string{},
		Result: InitResult{
			ConceptCount: len(bundle.Concepts),
			SavedCount:   saved,
		},
	}
}

// Refresh explicitly refreshes generated knowledge or derived query state.
func (s *Service) Refresh(ctx stdctx.Context, req RefreshRequest) ToolEnvelope {
	resolved, err := s.resolve()
	if err != nil {
		return failure(OperationRefresh, "", "", nil, err)
	}
	if err := checkContext(ctx); err != nil {
		return failure(OperationRefresh, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), err)
	}

	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = RefreshModeIncremental
	}

	cfg := gitConfigForResolved(resolved)
	switch mode {
	case RefreshModeFull:
		bundle, err := git.GenerateBundle(cfg, true)
		if err != nil {
			return failure(OperationRefresh, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), err)
		}
		saved, err := git.SaveKnowledgeBase(bundle, cfg)
		if err != nil {
			return failure(OperationRefresh, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), err)
		}
		return refreshSuccess(resolved, RefreshResult{
			Mode:           mode,
			GeneratedCount: len(bundle.Concepts),
			SavedCount:     saved,
		})
	case RefreshModeIncremental:
		bundle, updated, err := git.UpdateSinceLastIndexedCommit(cfg)
		if err != nil {
			return failure(OperationRefresh, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), err)
		}
		if bundle != nil && len(bundle.Concepts) > 0 {
			if err := git.ApplyIncrementalUpdate(cfg, bundle); err != nil {
				return failure(OperationRefresh, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), err)
			}
		}
		return refreshSuccess(resolved, RefreshResult{
			Mode:           mode,
			GeneratedCount: lenConcepts(bundle),
			UpdatedCount:   len(updated),
		})
	case RefreshModeCacheOnly:
		if okf.Exists(resolved.knowledgeDir) {
			if _, err := okf.LoadBundle(resolved.knowledgeDir, okf.DefaultLoadOptions()); err != nil {
				return failure(OperationRefresh, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), err)
			}
		}
		return refreshSuccess(resolved, RefreshResult{Mode: mode, RebuiltCache: false})
	default:
		return failure(OperationRefresh, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), toolError{
			code:        ErrInvalidRequest,
			message:     "unsupported refresh mode " + mode,
			remediation: "Use incremental, full, or cache-only. Auto refresh is not enabled by default in V1.",
		})
	}
}

// Query performs a deterministic read-only query over existing OKF knowledge.
func (s *Service) Query(ctx stdctx.Context, req QueryRequest) ToolEnvelope {
	resolved, err := s.resolve()
	if err != nil {
		return failure(OperationQuery, "", "", nil, err)
	}
	freshness := readFreshness(resolved)
	if !okf.Exists(resolved.knowledgeDir) {
		return failure(
			OperationQuery,
			resolved.repoRoot,
			resolved.knowledgeDir,
			freshness,
			knowledgeNotInitialized(resolved.repoRoot),
		)
	}
	if strings.TrimSpace(req.Query) == "" {
		return failure(OperationQuery, resolved.repoRoot, resolved.knowledgeDir, freshness, toolError{
			code:        ErrInvalidQuery,
			message:     "query must not be empty",
			remediation: "Pass a non-empty query string.",
		})
	}
	if err := checkContext(ctx); err != nil {
		return failure(OperationQuery, resolved.repoRoot, resolved.knowledgeDir, freshness, err)
	}

	bundle, loadMeta, err := loadKnowledgeBundle(resolved)
	if err != nil {
		return failure(OperationQuery, resolved.repoRoot, resolved.knowledgeDir, freshness, err)
	}

	trace := []TraceStep(nil)
	traceWarnings := append([]string{}, loadMeta.Warnings...)
	traceWarnings = append(traceWarnings, staleWarnings(freshness)...)
	if req.IncludeTrace {
		trace = append(trace, TraceStep{
			Type:     "path_resolution",
			Message:  "resolved knowledge paths",
			Refs:     knowledgePathRefs(resolved),
			Counts:   map[string]int{"read_paths": len(resolved.readPaths)},
			Warnings: traceWarnings,
		})
		trace = append(trace, TraceStep{
			Type:    "bundle_load",
			Message: "loaded OKF knowledge bundle",
			Counts:  map[string]int{"concepts": len(bundle.Concepts)},
		})
		trace = append(trace, overlayMergeTraceStep(loadMeta.OverlayMerges))
	}
	filters := filtersFromQueryRequest(req)
	if req.IncludeTrace {
		trace = append(trace, TraceStep{
			Type:    "filter_application",
			Message: "applied structured query filters",
			Counts:  map[string]int{"active_filters": activeQueryFilterCount(filters)},
		})
	}
	hits := rankConcepts(bundle.Concepts, req.Query, filters)
	if req.IncludeTrace {
		trace = append(trace, TraceStep{
			Type:       "candidate_scoring",
			Message:    "scored matching candidates",
			Counts:     map[string]int{"matches": len(hits)},
			ScoreDelta: topHitScore(hits),
		})
	}
	if req.Limit > 0 && len(hits) > req.Limit {
		hits = hits[:req.Limit]
	}
	if req.IncludeTrace {
		trace = append(trace, TraceStep{
			Type:    "ranking",
			Message: "ordered candidates by deterministic tie-breaks",
			Refs:    queryHitRefs(hits),
			Counts:  map[string]int{"returned": len(hits)},
		})
	}

	return ToolEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     OperationQuery,
		OK:            true,
		Mutating:      isMutatingOperation(OperationQuery),
		RepoRoot:      resolved.repoRoot,
		KnowledgeDir:  resolved.knowledgeDir,
		Freshness:     freshness,
		Warnings:      append(staleWarnings(freshness), loadMeta.Warnings...),
		Result: QueryResult{
			Query:   req.Query,
			Results: hits,
			Trace:   trace,
		},
	}
}

// Context builds deterministic source snippets from existing OKF knowledge.
func (s *Service) Context(ctx stdctx.Context, req ContextRequest) ToolEnvelope {
	resolved, err := s.resolve()
	if err != nil {
		return failure(OperationContext, "", "", nil, err)
	}
	freshness := readFreshness(resolved)
	if !okf.Exists(resolved.knowledgeDir) {
		return failure(
			OperationContext,
			resolved.repoRoot,
			resolved.knowledgeDir,
			freshness,
			knowledgeNotInitialized(resolved.repoRoot),
		)
	}
	if strings.TrimSpace(req.Query) == "" {
		return failure(OperationContext, resolved.repoRoot, resolved.knowledgeDir, freshness, toolError{
			code:        ErrInvalidQuery,
			message:     "query must not be empty",
			remediation: "Pass a non-empty query string.",
		})
	}
	if err := checkContext(ctx); err != nil {
		return failure(OperationContext, resolved.repoRoot, resolved.knowledgeDir, freshness, err)
	}

	bundle, loadMeta, err := loadKnowledgeBundle(resolved)
	if err != nil {
		return failure(OperationContext, resolved.repoRoot, resolved.knowledgeDir, freshness, err)
	}

	budget := req.BudgetTokens
	if budget <= 0 {
		budget = defaultContextBudgetTokens
	}
	hits := rankConcepts(bundle.Concepts, req.Query, queryFilters{})
	if len(hits) > maxContextCandidates {
		hits = hits[:maxContextCandidates]
	}
	items := []ContextItem{}
	omissions := []ContextOmission{}
	warnings := append(staleWarnings(freshness), loadMeta.Warnings...)
	usedTokens := 0
	omitted := 0
	conceptsByPath := indexConceptsByPath(bundle.Concepts)
	trace := []TraceStep(nil)
	traceWarnings := append([]string{}, loadMeta.Warnings...)
	traceWarnings = append(traceWarnings, staleWarnings(freshness)...)
	if req.IncludeTrace {
		trace = append(trace, TraceStep{
			Type:     "path_resolution",
			Message:  "resolved knowledge paths",
			Refs:     knowledgePathRefs(resolved),
			Counts:   map[string]int{"read_paths": len(resolved.readPaths)},
			Warnings: traceWarnings,
		})
		trace = append(trace, TraceStep{
			Type:    "primary_hits",
			Message: "selected primary query hits",
			Refs:    queryHitRefs(hits),
			Counts:  map[string]int{"hits": len(hits)},
		})
		trace = append(trace, overlayMergeTraceStep(loadMeta.OverlayMerges))
	}

	candidates := buildContextCandidates(hits, bundle.Concepts, conceptsByPath, req.IncludeRelations, req.IncludeTrace, &trace)
	if len(candidates) > maxContextCandidates {
		omitted += len(candidates) - maxContextCandidates
		omissions = append(omissions, ContextOmission{Reason: "candidate_limit_exceeded", Count: len(candidates) - maxContextCandidates})
		candidates = candidates[:maxContextCandidates]
	}
	candidates = mergeContextCandidates(candidates, req.IncludeTrace, &trace)
	sourceCache := map[string][]byte{}
	for i, candidate := range candidates {
		if err := checkContext(ctx); err != nil {
			return failure(OperationContext, resolved.repoRoot, resolved.knowledgeDir, freshness, err)
		}
		if usedTokens >= budget {
			remaining := len(candidates) - i
			omission := omissionForCandidate(candidate, "budget_exceeded", remaining)
			omissions = append(omissions, omission)
			appendTraceOmission(req.IncludeTrace, &trace, omission)
			omitted += remaining
			break
		}
		hit := candidate.primaryHit()
		concept := candidate.primaryConcept(conceptsByPath)
		if concept == nil {
			omission := omissionForCandidate(candidate, "concept_missing", 1)
			omissions = append(omissions, omission)
			appendTraceOmission(req.IncludeTrace, &trace, omission)
			omitted++
			continue
		}
		sourcePath := candidate.SourcePath
		if sourcePath == "" {
			warnings = append(warnings, "no source path for "+concept.Title)
			omission := ContextOmission{Reason: "no_source_path", Title: concept.Title, Count: 1}
			omissions = append(omissions, omission)
			appendTraceOmission(req.IncludeTrace, &trace, omission)
			omitted++
			continue
		}
		fullPath, ok := safeRepoPath(resolved.repoRoot, sourcePath)
		if !ok {
			warnings = append(warnings, "source path escapes repository: "+sourcePath)
			omission := ContextOmission{Reason: "source_path_escapes_repo", Title: concept.Title, SourcePath: sourcePath, Count: 1}
			omissions = append(omissions, omission)
			appendTraceOmission(req.IncludeTrace, &trace, omission)
			omitted++
			continue
		}
		data, ok := sourceCache[fullPath]
		var readErr error
		if !ok {
			data, readErr = readContextSourceFile(fullPath)
			if readErr == nil {
				sourceCache[fullPath] = data
			}
		}
		if readErr != nil {
			warnings = append(warnings, "source file missing: "+sourcePath)
			omission := ContextOmission{Reason: "source_missing", Title: concept.Title, SourcePath: sourcePath, Count: 1}
			omissions = append(omissions, omission)
			appendTraceOmission(req.IncludeTrace, &trace, omission)
			items = append(items, ContextItem{
				Title:         concept.Title,
				Type:          concept.Type,
				SourcePath:    sourcePath,
				Location:      formatLocation(sourcePath, candidate.StartLine, candidate.EndLine),
				StartLine:     candidate.StartLine,
				EndLine:       candidate.EndLine,
				Snippet:       "",
				TokenEstimate: 0,
				Score:         hit.Score,
				Reason:        candidate.reason(),
				Provenance:    hit.Provenance,
			})
			omitted++
			continue
		}
		snippet, startLine, endLine, sourceOmitted := extractPlannedSnippet(string(data), req.Query, candidate.StartLine, candidate.EndLine, budget-usedTokens)
		if snippet == "" {
			omission := ContextOmission{Reason: "snippet_empty", Title: concept.Title, SourcePath: sourcePath, Count: 1}
			omissions = append(omissions, omission)
			appendTraceOmission(req.IncludeTrace, &trace, omission)
			omitted++
			continue
		}
		tokens := estimateTokens(snippet)
		if tokens <= 0 {
			omission := ContextOmission{Reason: "zero_token_snippet", Title: concept.Title, SourcePath: sourcePath, Count: 1}
			omissions = append(omissions, omission)
			appendTraceOmission(req.IncludeTrace, &trace, omission)
			omitted++
			continue
		}
		if usedTokens+tokens > budget {
			omission := omissionForCandidate(candidate, "budget_exceeded", 1)
			omissions = append(omissions, omission)
			appendTraceOmission(req.IncludeTrace, &trace, omission)
			omitted++
			continue
		}
		usedTokens += tokens
		omitted += sourceOmitted
		if sourceOmitted > 0 {
			omission := ContextOmission{Reason: "snippet_truncated", Title: concept.Title, SourcePath: sourcePath, Count: sourceOmitted}
			omissions = append(omissions, omission)
			appendTraceOmission(req.IncludeTrace, &trace, omission)
		}
		if req.IncludeTrace {
			trace = append(trace, TraceStep{
				Type:    "snippet_extraction",
				Message: "extracted source snippet",
				Refs:    uniqueStrings([]string{formatLocation(sourcePath, startLine, endLine), hit.ConceptPath, hit.KnowledgePath}),
				Counts:  map[string]int{"tokens": tokens},
			})
		}
		items = append(items, ContextItem{
			Title:         concept.Title,
			Type:          concept.Type,
			SourcePath:    sourcePath,
			Location:      formatLocation(sourcePath, startLine, endLine),
			StartLine:     startLine,
			EndLine:       endLine,
			Snippet:       snippet,
			TokenEstimate: tokens,
			Score:         hit.Score,
			Reason:        candidate.reason(),
			Provenance:    "repo.source",
		})
	}
	if req.IncludeTrace {
		trace = append(trace, TraceStep{
			Type:    "budget_packing",
			Message: "packed context items under token budget",
			Counts:  map[string]int{"items": len(items), "used_tokens": usedTokens, "budget_tokens": budget, "omitted": omitted},
		})
	}

	return ToolEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     OperationContext,
		OK:            true,
		Mutating:      isMutatingOperation(OperationContext),
		RepoRoot:      resolved.repoRoot,
		KnowledgeDir:  resolved.knowledgeDir,
		Freshness:     freshness,
		Warnings:      warnings,
		Result: ContextResult{
			Query:        req.Query,
			BudgetTokens: budget,
			UsedTokens:   usedTokens,
			Omitted:      omitted,
			Omissions:    omissions,
			Items:        items,
			Trace:        trace,
		},
	}
}

type contextCandidate struct {
	SourcePath string
	StartLine  int
	EndLine    int
	Hits       []QueryHit
	ReasonText string
	Relation   bool
}

func (c contextCandidate) primaryHit() QueryHit {
	if len(c.Hits) == 0 {
		return QueryHit{}
	}
	return c.Hits[0]
}

func (c contextCandidate) primaryConcept(conceptsByPath map[string]*okf.Concept) *okf.Concept {
	for _, hit := range c.Hits {
		if concept := conceptsByPath[hit.ConceptPath]; concept != nil {
			return concept
		}
	}
	return nil
}

func (c contextCandidate) reason() string {
	if strings.TrimSpace(c.ReasonText) != "" {
		return c.ReasonText
	}
	reasons := make([]string, 0, len(c.Hits))
	for _, hit := range c.Hits {
		if hit.Reason != "" {
			reasons = append(reasons, hit.Reason)
		}
	}
	reasons = uniqueStrings(reasons)
	if len(c.Hits) > 1 {
		return fmt.Sprintf("merged %d hits: %s", len(c.Hits), strings.Join(reasons, "; "))
	}
	return strings.Join(reasons, "; ")
}

func buildContextCandidates(hits []QueryHit, concepts []*okf.Concept, conceptsByPath map[string]*okf.Concept, includeRelations, includeTrace bool, trace *[]TraceStep) []contextCandidate {
	primary := make([]contextCandidate, 0, len(hits))
	for _, hit := range hits {
		primary = append(primary, candidateFromHit(hit))
	}
	if !includeRelations || len(primary) == 0 {
		if includeTrace {
			*trace = append(*trace, TraceStep{Type: "relation_expansion", Message: "relation expansion disabled", Counts: map[string]int{"neighbors": 0}})
		}
		return primary
	}

	result := make([]contextCandidate, 0, len(primary))
	seen := map[string]bool{}
	for _, candidate := range primary {
		result = append(result, candidate)
		seen[candidateKey(candidate)] = true
		neighbors := relationNeighborCandidates(candidate.primaryHit(), concepts, conceptsByPath)
		if len(neighbors) > contextNeighborLimit {
			neighbors = neighbors[:contextNeighborLimit]
		}
		added := 0
		for _, neighbor := range neighbors {
			key := candidateKey(neighbor)
			if seen[key] {
				continue
			}
			seen[key] = true
			result = append(result, neighbor)
			added++
		}
		if includeTrace {
			*trace = append(*trace, TraceStep{
				Type:    "relation_expansion",
				Message: "expanded one-hop relation neighbors",
				Refs:    []string{candidate.SourcePath},
				Counts:  map[string]int{"neighbors": added},
			})
		}
	}
	return result
}

func candidateFromHit(hit QueryHit) contextCandidate {
	return contextCandidate{
		SourcePath: hit.SourcePath,
		StartLine:  hit.StartLine,
		EndLine:    hit.EndLine,
		Hits:       []QueryHit{hit},
	}
}

func relationNeighborCandidates(primary QueryHit, concepts []*okf.Concept, conceptsByPath map[string]*okf.Concept) []contextCandidate {
	if primary.SourcePath == "" {
		return nil
	}
	relations := make([]*okf.Concept, 0)
	for _, concept := range concepts {
		if concept == nil {
			continue
		}
		relationKind := stringCustomFieldOrEmpty(concept.CustomFields, "relation_kind")
		if relationKind == "" {
			continue
		}
		source := filepath.Clean(stringCustomFieldOrEmpty(concept.CustomFields, "relation_source"))
		target := filepath.Clean(stringCustomFieldOrEmpty(concept.CustomFields, "relation_target"))
		if source == primary.SourcePath || target == primary.SourcePath {
			relations = append(relations, concept)
		}
	}
	sort.SliceStable(relations, func(i, j int) bool {
		leftKind := stringCustomFieldOrEmpty(relations[i].CustomFields, "relation_kind")
		rightKind := stringCustomFieldOrEmpty(relations[j].CustomFields, "relation_kind")
		if leftKind != rightKind {
			return leftKind < rightKind
		}
		leftTarget := stringCustomFieldOrEmpty(relations[i].CustomFields, "relation_target")
		rightTarget := stringCustomFieldOrEmpty(relations[j].CustomFields, "relation_target")
		if leftTarget != rightTarget {
			return leftTarget < rightTarget
		}
		return relations[i].FilePath < relations[j].FilePath
	})

	neighbors := make([]contextCandidate, 0, len(relations))
	for _, relation := range relations {
		relationKind := stringCustomFieldOrEmpty(relation.CustomFields, "relation_kind")
		source := filepath.Clean(stringCustomFieldOrEmpty(relation.CustomFields, "relation_source"))
		target := filepath.Clean(stringCustomFieldOrEmpty(relation.CustomFields, "relation_target"))
		neighborPath := target
		if neighborPath == primary.SourcePath {
			neighborPath = source
		}
		neighborHit, ok := bestHitForSourcePath(neighborPath, concepts, conceptsByPath, primary.Score-1)
		if !ok {
			continue
		}
		candidate := candidateFromHit(neighborHit)
		candidate.Relation = true
		candidate.ReasonText = fmt.Sprintf("relation %s from %s to %s", relationKind, source, target)
		neighbors = append(neighbors, candidate)
	}
	return neighbors
}

func bestHitForSourcePath(sourcePath string, concepts []*okf.Concept, conceptsByPath map[string]*okf.Concept, score int) (QueryHit, bool) {
	candidates := make([]QueryHit, 0)
	for _, concept := range concepts {
		if concept == nil || sourcePathForConcept(concept) != sourcePath {
			continue
		}
		if stringCustomFieldOrEmpty(concept.CustomFields, "relation_kind") != "" {
			continue
		}
		startLine := intCustomField(concept.CustomFields, "start_line")
		endLine := intCustomField(concept.CustomFields, "end_line")
		symbolKind, _ := stringCustomField(concept.CustomFields, "symbol_kind")
		qualifiedName, _ := stringCustomField(concept.CustomFields, "qualified_name")
		candidates = append(candidates, QueryHit{
			Title:               concept.Title,
			Type:                concept.Type,
			Resource:            concept.Resource,
			FilePath:            sourcePath,
			SourcePath:          sourcePath,
			Location:            formatLocation(sourcePath, startLine, endLine),
			ConceptPath:         concept.FilePath,
			StartLine:           startLine,
			EndLine:             endLine,
			SymbolKind:          symbolKind,
			QualifiedName:       qualifiedName,
			Score:               score,
			Reason:              "relation neighbor",
			Provenance:          provenanceForConcept(concept),
			Generated:           boolCustomField(concept.CustomFields, "generated"),
			Generator:           stringCustomFieldOrEmpty(concept.CustomFields, "generator"),
			KnowledgePath:       stringCustomFieldOrEmpty(concept.CustomFields, "knowledge_path"),
			KnowledgePathSource: stringCustomFieldOrEmpty(concept.CustomFields, "knowledge_path_source"),
			SourceRank:          intCustomField(concept.CustomFields, "source_rank"),
			DuplicateSources:    duplicateSourcesForConcept(concept),
			exactness:           0,
			sourceRank:          intCustomField(concept.CustomFields, "source_rank"),
			typeRank:            sourcePreference(concept),
		})
	}
	if len(candidates) == 0 {
		return QueryHit{}, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].sourceRank != candidates[j].sourceRank {
			return candidates[i].sourceRank < candidates[j].sourceRank
		}
		if candidates[i].typeRank != candidates[j].typeRank {
			return candidates[i].typeRank > candidates[j].typeRank
		}
		if candidates[i].StartLine != candidates[j].StartLine {
			return candidates[i].StartLine < candidates[j].StartLine
		}
		if candidates[i].Title != candidates[j].Title {
			return candidates[i].Title < candidates[j].Title
		}
		return candidates[i].ConceptPath < candidates[j].ConceptPath
	})
	_ = conceptsByPath
	return candidates[0], true
}

func mergeContextCandidates(candidates []contextCandidate, includeTrace bool, trace *[]TraceStep) []contextCandidate {
	merged := make([]contextCandidate, 0, len(candidates))
	mergeCount := 0
	mergeRefs := []string{}
	for _, candidate := range candidates {
		if candidate.SourcePath == "" || candidate.StartLine <= 0 || candidate.EndLine <= 0 {
			merged = append(merged, candidate)
			continue
		}
		mergedIntoExisting := false
		for i := range merged {
			if candidate.Relation || merged[i].Relation || merged[i].SourcePath != candidate.SourcePath {
				continue
			}
			if rangesOverlapOrAdjacent(merged[i].StartLine, merged[i].EndLine, candidate.StartLine, candidate.EndLine) {
				merged[i].StartLine = minPositive(merged[i].StartLine, candidate.StartLine)
				merged[i].EndLine = maxInt(merged[i].EndLine, candidate.EndLine)
				merged[i].Hits = append(merged[i].Hits, candidate.Hits...)
				merged[i].ReasonText = ""
				mergeRefs = append(mergeRefs, formatLocation(merged[i].SourcePath, merged[i].StartLine, merged[i].EndLine))
				mergeCount++
				mergedIntoExisting = true
				break
			}
		}
		if !mergedIntoExisting {
			merged = append(merged, candidate)
		}
	}
	if includeTrace {
		*trace = append(*trace, TraceStep{Type: "range_merge", Message: "merged overlapping or adjacent same-file ranges", Refs: uniqueStrings(mergeRefs), Counts: map[string]int{"merges": mergeCount}})
	}
	return merged
}

func rangesOverlapOrAdjacent(aStart, aEnd, bStart, bEnd int) bool {
	return bStart <= aEnd+1 && aStart <= bEnd+1
}

func minPositive(a, b int) int {
	if a <= 0 {
		return b
	}
	if b <= 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

func candidateKey(candidate contextCandidate) string {
	return fmt.Sprintf("%s:%d:%d", candidate.SourcePath, candidate.StartLine, candidate.EndLine)
}

func omissionForCandidate(candidate contextCandidate, reason string, count int) ContextOmission {
	hit := candidate.primaryHit()
	return ContextOmission{Reason: reason, Title: hit.Title, SourcePath: candidate.SourcePath, Count: count}
}

func appendTraceOmission(includeTrace bool, trace *[]TraceStep, omission ContextOmission) {
	if !includeTrace {
		return
	}
	refs := []string{}
	if omission.SourcePath != "" {
		refs = append(refs, omission.SourcePath)
	}
	*trace = append(*trace, TraceStep{
		Type:           "omission",
		Message:        "omitted context candidate",
		Refs:           refs,
		Counts:         map[string]int{"count": omission.Count},
		OmissionReason: omission.Reason,
	})
}

func readContextSourceFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	limited := io.LimitReader(file, maxContextSourceBytes)
	return io.ReadAll(limited)
}

func loadKnowledgeBundle(resolved resolvedConfig) (*okf.KnowledgeBundle, bundleLoadMetadata, error) {
	readPaths := resolved.readPaths
	if len(readPaths) == 0 {
		readPaths = []okf.ResolvedKnowledgePath{{Path: resolved.knowledgeDir, Rank: 0}}
	}
	combined := okf.NewBundle("overlay")
	meta := bundleLoadMetadata{Warnings: []string{}}
	generatedByIdentity := map[string]*okf.Concept{}

	for _, readPath := range readPaths {
		info, err := os.Stat(readPath.Path)
		if err != nil {
			if os.IsNotExist(err) && readPath.Rank > 0 {
				meta.Warnings = append(meta.Warnings, "overlay knowledge path missing: "+readPath.Path)
				continue
			}
			return nil, meta, knowledgePathInvalid(readPath.Path, "failed to read knowledge path", err)
		}
		if !info.IsDir() {
			return nil, meta, knowledgePathInvalid(readPath.Path, "knowledge path is not a directory", nil)
		}
		bundle, err := okf.LoadBundle(readPath.Path, okf.DefaultLoadOptions())
		if err != nil {
			return nil, meta, err
		}
		for _, concept := range bundle.Concepts {
			if concept == nil {
				continue
			}
			annotateConceptKnowledgePath(concept, readPath)
			if identity := generatedConceptIdentity(concept); identity != "" {
				if existing := generatedByIdentity[identity]; existing != nil {
					appendDuplicateSource(existing, readPath)
					meta.OverlayMerges = append(meta.OverlayMerges, overlayMergeDecision{
						PrimaryPath: stringCustomFieldOrEmpty(existing.CustomFields, "knowledge_path"),
						MergedPaths: []string{readPath.Path},
						Count:       1,
					})
					continue
				}
				generatedByIdentity[identity] = concept
			}
			combined.Concepts = append(combined.Concepts, concept)
		}
	}
	return combined, meta, nil
}

func annotateConceptKnowledgePath(concept *okf.Concept, readPath okf.ResolvedKnowledgePath) {
	if concept.CustomFields == nil {
		concept.CustomFields = map[string]interface{}{}
	}
	concept.CustomFields["knowledge_path"] = readPath.Path
	concept.CustomFields["knowledge_path_source"] = string(readPath.Source)
	concept.CustomFields["source_rank"] = readPath.Rank
}

func appendDuplicateSource(concept *okf.Concept, readPath okf.ResolvedKnowledgePath) {
	if concept.CustomFields == nil {
		concept.CustomFields = map[string]interface{}{}
	}
	duplicate := KnowledgePathResolution{Path: readPath.Path, Source: string(readPath.Source), Rank: readPath.Rank}
	existing, _ := concept.CustomFields["duplicate_sources"].([]KnowledgePathResolution)
	concept.CustomFields["duplicate_sources"] = append(existing, duplicate)
}

func generatedConceptIdentity(concept *okf.Concept) string {
	if concept == nil || !boolCustomField(concept.CustomFields, "generated") {
		return ""
	}
	generator := stringCustomFieldOrEmpty(concept.CustomFields, "generator")
	sourcePath := sourcePathForConcept(concept)
	identity := concept.Resource
	if identity == "" {
		identity = concept.FilePath
	}
	if generator == "" || sourcePath == "" || concept.Type == "" || identity == "" {
		return ""
	}
	return strings.Join([]string{generator, sourcePath, concept.Type, identity}, "\x00")
}

func knowledgePathStatus(resolved resolvedConfig) KnowledgePathStatus {
	return KnowledgePathStatus{
		WritePath:   resolved.knowledgeDir,
		WriteSource: string(resolved.writeSource),
		ReadPaths:   knowledgePathResolutions(resolved.readPaths),
	}
}

func knowledgePathResolutions(paths []okf.ResolvedKnowledgePath) []KnowledgePathResolution {
	result := make([]KnowledgePathResolution, 0, len(paths))
	for _, path := range paths {
		result = append(result, KnowledgePathResolution{Path: path.Path, Source: string(path.Source), Rank: path.Rank})
	}
	return result
}

func knowledgePathRefs(resolved resolvedConfig) []string {
	refs := make([]string, 0, len(resolved.readPaths))
	for _, path := range resolved.readPaths {
		refs = append(refs, path.Path)
	}
	if len(refs) == 0 {
		refs = append(refs, resolved.knowledgeDir)
	}
	return refs
}

func overlayMergeTraceStep(decisions []overlayMergeDecision) TraceStep {
	refs := []string{}
	collapsed := 0
	mergedByPrimary := map[string][]string{}
	for _, decision := range decisions {
		if decision.PrimaryPath != "" {
			refs = append(refs, decision.PrimaryPath)
		}
		collapsed += decision.Count
		mergedByPrimary[decision.PrimaryPath] = append(mergedByPrimary[decision.PrimaryPath], decision.MergedPaths...)
	}
	for _, merged := range mergedByPrimary {
		refs = append(refs, merged...)
	}
	refs = uniqueStrings(refs)
	message := "no overlay merges applied"
	if collapsed > 0 {
		message = "collapsed duplicate generated concepts across overlay paths"
	}
	return TraceStep{
		Type:    "overlay_merge",
		Message: message,
		Refs:    refs,
		Counts:  map[string]int{"collapsed_duplicates": collapsed},
	}
}

type resolvedConfig struct {
	repoRoot     string
	knowledgeDir string
	writeSource  okf.KnowledgePathSource
	readPaths    []okf.ResolvedKnowledgePath
}

func (s *Service) resolve() (resolvedConfig, error) {
	repoPath := s.cfg.RepoPath
	if repoPath == "" {
		wd, err := os.Getwd()
		if err != nil {
			return resolvedConfig{}, err
		}
		repoPath = wd
	}

	repoRoot, err := git.GetRepoRoot(repoPath)
	if err != nil {
		return resolvedConfig{}, toolError{
			code:        ErrNotGitRepository,
			message:     repoPath + " is not a git repository",
			remediation: "Run the command inside a Git repository or pass --repo to one.",
		}
	}

	resolvedPaths, err := okf.ResolveKnowledgePaths(okf.ResolveKnowledgePathsOptions{
		CLIDir:     s.cfg.KnowledgeDir,
		RepoRoot:   repoRoot,
		WorkingDir: repoRoot,
		ReadOnly:   true,
	})
	if err != nil {
		return resolvedConfig{}, err
	}
	knowledgeDir := resolvedPaths.WritePath
	if !filepath.IsAbs(knowledgeDir) {
		knowledgeDir = filepath.Join(repoRoot, knowledgeDir)
	}
	readPaths := make([]okf.ResolvedKnowledgePath, 0, len(resolvedPaths.ReadPaths))
	for _, readPath := range resolvedPaths.ReadPaths {
		if !filepath.IsAbs(readPath.Path) {
			readPath.Path = filepath.Join(repoRoot, readPath.Path)
		}
		readPaths = append(readPaths, readPath)
	}

	return resolvedConfig{repoRoot: repoRoot, knowledgeDir: knowledgeDir, writeSource: resolvedPaths.WriteSource, readPaths: readPaths}, nil
}

func readFreshness(resolved resolvedConfig) *Freshness {
	head, err := git.GetCurrentCommit(resolved.repoRoot)
	if err != nil {
		head = ""
	}
	cfg := git.DefaultConfig()
	cfg.RepoPath = resolved.repoRoot
	cfg.KnowledgeDir = knowledgeDirForGitConfig(resolved.repoRoot, resolved.knowledgeDir)
	state, err := git.ReadState(cfg)
	lastIndexed := ""
	if err == nil && state != nil {
		lastIndexed = state.LastIndexedCommit
	}

	return &Freshness{
		Head:              head,
		LastIndexedCommit: lastIndexed,
		Stale:             lastIndexed != "" && head != "" && lastIndexed != head,
	}
}

func knowledgeDirForGitConfig(repoRoot, knowledgeDir string) string {
	if rel, err := filepath.Rel(repoRoot, knowledgeDir); err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
		return rel
	}
	return knowledgeDir
}

func gitConfigForResolved(resolved resolvedConfig) *git.Config {
	cfg := git.DefaultConfig()
	cfg.RepoPath = resolved.repoRoot
	cfg.KnowledgeDir = knowledgeDirForGitConfig(resolved.repoRoot, resolved.knowledgeDir)
	return cfg
}

func refreshSuccess(resolved resolvedConfig, result RefreshResult) ToolEnvelope {
	return ToolEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     OperationRefresh,
		OK:            true,
		Mutating:      isMutatingOperation(OperationRefresh),
		RepoRoot:      resolved.repoRoot,
		KnowledgeDir:  resolved.knowledgeDir,
		Freshness:     readFreshness(resolved),
		Warnings:      []string{},
		Result:        result,
	}
}

func lenConcepts(bundle *okf.KnowledgeBundle) int {
	if bundle == nil {
		return 0
	}
	return len(bundle.Concepts)
}

type queryFilters struct {
	Type           string
	Tag            string
	FilePath       string
	Language       string
	SymbolKind     string
	QualifiedName  string
	RelationKind   string
	RelationSource string
	RelationTarget string
}

func filtersFromQueryRequest(req QueryRequest) queryFilters {
	filePath := strings.TrimSpace(req.FilePath)
	if filePath != "" {
		filePath = filepath.Clean(filePath)
	}
	return queryFilters{
		Type:           strings.TrimSpace(req.Type),
		Tag:            strings.TrimSpace(req.Tag),
		FilePath:       filePath,
		Language:       strings.TrimSpace(req.Language),
		SymbolKind:     strings.TrimSpace(req.SymbolKind),
		QualifiedName:  strings.TrimSpace(req.QualifiedName),
		RelationKind:   strings.TrimSpace(req.RelationKind),
		RelationSource: cleanOptionalPath(req.RelationSource),
		RelationTarget: cleanOptionalPath(req.RelationTarget),
	}
}

func cleanOptionalPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

func activeQueryFilterCount(filters queryFilters) int {
	count := 0
	if filters.Type != "" {
		count++
	}
	if filters.Tag != "" {
		count++
	}
	if filters.FilePath != "" {
		count++
	}
	if filters.Language != "" {
		count++
	}
	if filters.SymbolKind != "" {
		count++
	}
	if filters.QualifiedName != "" {
		count++
	}
	if filters.RelationKind != "" {
		count++
	}
	if filters.RelationSource != "" {
		count++
	}
	if filters.RelationTarget != "" {
		count++
	}
	return count
}

func topHitScore(hits []QueryHit) int {
	if len(hits) == 0 {
		return 0
	}
	return hits[0].Score
}

func queryHitRefs(hits []QueryHit) []string {
	refs := make([]string, 0, len(hits)*3)
	for _, hit := range hits {
		for _, ref := range []string{hit.Location, hit.SourcePath, hit.ConceptPath, hit.KnowledgePath} {
			if ref != "" {
				refs = append(refs, ref)
			}
		}
	}
	return uniqueStrings(refs)
}

func rankConcepts(concepts []*okf.Concept, query string, filters queryFilters) []QueryHit {
	hits := make([]QueryHit, 0, len(concepts))
	for _, concept := range filteredConceptsForQuery(concepts, filters) {
		if concept == nil {
			continue
		}
		score, reason, exactness := scoreConcept(concept, query)
		if score == 0 {
			continue
		}
		sourcePath := sourcePathForConcept(concept)
		startLine := intCustomField(concept.CustomFields, "start_line")
		endLine := intCustomField(concept.CustomFields, "end_line")
		symbolKind, _ := stringCustomField(concept.CustomFields, "symbol_kind")
		qualifiedName, _ := stringCustomField(concept.CustomFields, "qualified_name")
		relationKind, _ := stringCustomField(concept.CustomFields, "relation_kind")
		relationSource, _ := stringCustomField(concept.CustomFields, "relation_source")
		relationTarget, _ := stringCustomField(concept.CustomFields, "relation_target")
		knowledgePath, _ := stringCustomField(concept.CustomFields, "knowledge_path")
		knowledgePathSource, _ := stringCustomField(concept.CustomFields, "knowledge_path_source")
		sourceRank := intCustomField(concept.CustomFields, "source_rank")
		hits = append(hits, QueryHit{
			Title:               concept.Title,
			Type:                concept.Type,
			Resource:            concept.Resource,
			FilePath:            sourcePath,
			SourcePath:          sourcePath,
			Location:            formatLocation(sourcePath, startLine, endLine),
			ConceptPath:         concept.FilePath,
			StartLine:           startLine,
			EndLine:             endLine,
			SymbolKind:          symbolKind,
			QualifiedName:       qualifiedName,
			RelationKind:        relationKind,
			RelationSource:      relationSource,
			RelationTarget:      relationTarget,
			Score:               score,
			Reason:              reason,
			Provenance:          provenanceForConcept(concept),
			Generated:           boolCustomField(concept.CustomFields, "generated"),
			Generator:           stringCustomFieldOrEmpty(concept.CustomFields, "generator"),
			KnowledgePath:       knowledgePath,
			KnowledgePathSource: knowledgePathSource,
			SourceRank:          sourceRank,
			DuplicateSources:    duplicateSourcesForConcept(concept),
			exactness:           exactness,
			sourceRank:          sourceRank,
			typeRank:            sourcePreference(concept),
		})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].exactness != hits[j].exactness {
			return hits[i].exactness > hits[j].exactness
		}
		if hits[i].sourceRank != hits[j].sourceRank {
			return hits[i].sourceRank < hits[j].sourceRank
		}
		if hits[i].typeRank != hits[j].typeRank {
			return hits[i].typeRank > hits[j].typeRank
		}
		if hits[i].FilePath != hits[j].FilePath {
			return hits[i].FilePath < hits[j].FilePath
		}
		if hits[i].StartLine != hits[j].StartLine {
			return hits[i].StartLine < hits[j].StartLine
		}
		if hits[i].Title != hits[j].Title {
			return hits[i].Title < hits[j].Title
		}
		if hits[i].ConceptPath != hits[j].ConceptPath {
			return hits[i].ConceptPath < hits[j].ConceptPath
		}
		return hits[i].Title < hits[j].Title
	})
	return hits
}

func filteredConceptsForQuery(concepts []*okf.Concept, filters queryFilters) []*okf.Concept {
	if activeQueryFilterCount(filters) == 0 {
		return concepts
	}
	bundle := &querypkg.KnowledgeBundle{Concepts: make([]*querypkg.Concept, 0, len(concepts))}
	byQueryConcept := map[*querypkg.Concept]*okf.Concept{}
	for _, concept := range concepts {
		if concept == nil {
			continue
		}
		queryConcept := &querypkg.Concept{
			Type:         concept.Type,
			Title:        concept.Title,
			Description:  concept.Description,
			Resource:     concept.Resource,
			Tags:         concept.Tags,
			Content:      concept.Content,
			FilePath:     concept.FilePath,
			CustomFields: concept.CustomFields,
		}
		bundle.Concepts = append(bundle.Concepts, queryConcept)
		byQueryConcept[queryConcept] = concept
	}
	q := querypkg.New().
		WithType(filters.Type).
		WithCodeLanguage(filters.Language).
		WithCodeFilePath(filters.FilePath).
		WithCodeSymbolKind(filters.SymbolKind).
		WithCodeQualifiedName(filters.QualifiedName).
		WithCodeRelationKind(filters.RelationKind).
		WithCodeRelationSource(filters.RelationSource).
		WithCodeRelationTarget(filters.RelationTarget).
		Build()
	if filters.Tag != "" {
		q = querypkg.New().
			WithType(filters.Type).
			WithTags(filters.Tag).
			WithCodeLanguage(filters.Language).
			WithCodeFilePath(filters.FilePath).
			WithCodeSymbolKind(filters.SymbolKind).
			WithCodeQualifiedName(filters.QualifiedName).
			WithCodeRelationKind(filters.RelationKind).
			WithCodeRelationSource(filters.RelationSource).
			WithCodeRelationTarget(filters.RelationTarget).
			Build()
	}
	filtered := q.Execute(bundle)
	result := make([]*okf.Concept, 0, len(filtered))
	for _, concept := range filtered {
		if original := byQueryConcept[concept]; original != nil && matchesQueryFilters(original, filters) {
			result = append(result, original)
		}
	}
	return result
}

func scoreConcept(concept *okf.Concept, rawQuery string) (int, string, int) {
	query := strings.ToLower(strings.TrimSpace(rawQuery))
	title := strings.ToLower(concept.Title)
	description := strings.ToLower(concept.Description)
	content := strings.ToLower(concept.Content)
	resource := strings.ToLower(concept.Resource)
	symbolKind := strings.ToLower(strings.TrimSpace(stringCustomFieldOrEmpty(concept.CustomFields, "symbol_kind")))
	qualifiedName := strings.ToLower(strings.TrimSpace(stringCustomFieldOrEmpty(concept.CustomFields, "qualified_name")))
	relationKind := strings.ToLower(strings.TrimSpace(stringCustomFieldOrEmpty(concept.CustomFields, "relation_kind")))
	relationSource := strings.ToLower(strings.TrimSpace(stringCustomFieldOrEmpty(concept.CustomFields, "relation_source")))
	relationTarget := strings.ToLower(strings.TrimSpace(stringCustomFieldOrEmpty(concept.CustomFields, "relation_target")))

	score := 0
	exactness := 0
	reasons := []string{}
	if qualifiedName == query || title == query {
		score += 120
		exactness = maxInt(exactness, 100)
		reasons = append(reasons, "exact symbol match")
	} else if qualifiedName != "" && strings.Contains(qualifiedName, query) {
		score += 70
		exactness = maxInt(exactness, 70)
		reasons = append(reasons, "qualified symbol match")
	}
	if title == query {
		score += 80
		exactness = maxInt(exactness, 90)
		reasons = append(reasons, "exact title match")
	} else if strings.Contains(title, query) {
		score += 40
		exactness = maxInt(exactness, 50)
		reasons = append(reasons, "title match")
	}
	if strings.Contains(resource, query) {
		score += 50
		reasons = append(reasons, "resource match")
	}
	if symbolKind != "" && strings.Contains(symbolKind, query) {
		score += 25
		reasons = append(reasons, "symbol kind match")
	}
	if relationKind != "" && strings.Contains(relationKind, query) {
		score += 25
		reasons = append(reasons, "relation kind match")
	}
	if relationSource != "" && strings.Contains(relationSource, query) {
		score += 45
		exactness = maxInt(exactness, 60)
		reasons = append(reasons, "relation source match")
	}
	if relationTarget != "" && strings.Contains(relationTarget, query) {
		score += 45
		exactness = maxInt(exactness, 60)
		reasons = append(reasons, "relation target match")
	}
	if strings.Contains(description, query) {
		score += 30
		reasons = append(reasons, "description match")
	}
	if strings.Contains(content, query) {
		score += 10
		reasons = append(reasons, "content match")
	}
	if score == 0 {
		return 0, "", 0
	}
	return int(math.Min(float64(score), 1000)), strings.Join(uniqueStrings(reasons), ", "), exactness
}

func matchesQueryFilters(concept *okf.Concept, filters queryFilters) bool {
	if filters.Type != "" && concept.Type != filters.Type {
		return false
	}
	if filters.Tag != "" && !hasTag(concept.Tags, filters.Tag) {
		return false
	}
	if filters.FilePath != "" && sourcePathForConcept(concept) != filters.FilePath {
		return false
	}
	if filters.Language != "" && !strings.EqualFold(stringCustomFieldOrEmpty(concept.CustomFields, "language"), filters.Language) {
		return false
	}
	if filters.SymbolKind != "" && stringCustomFieldOrEmpty(concept.CustomFields, "symbol_kind") != filters.SymbolKind {
		return false
	}
	if filters.QualifiedName != "" && stringCustomFieldOrEmpty(concept.CustomFields, "qualified_name") != filters.QualifiedName {
		return false
	}
	if filters.RelationKind != "" && stringCustomFieldOrEmpty(concept.CustomFields, "relation_kind") != filters.RelationKind {
		return false
	}
	if filters.RelationSource != "" && filepath.Clean(stringCustomFieldOrEmpty(concept.CustomFields, "relation_source")) != filters.RelationSource {
		return false
	}
	if filters.RelationTarget != "" && filepath.Clean(stringCustomFieldOrEmpty(concept.CustomFields, "relation_target")) != filters.RelationTarget {
		return false
	}
	return true
}

func hasTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

func stringCustomFieldOrEmpty(fields map[string]interface{}, key string) string {
	value, _ := stringCustomField(fields, key)
	return value
}

func intCustomField(fields map[string]interface{}, key string) int {
	if fields == nil {
		return 0
	}
	value, ok := fields[key]
	if !ok {
		return 0
	}
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case uint64:
		return int(v)
	case string:
		var parsed int
		if _, err := fmt.Sscanf(v, "%d", &parsed); err == nil {
			return parsed
		}
	}
	return 0
}

func provenanceForConcept(concept *okf.Concept) string {
	if provenance, ok := stringCustomField(concept.CustomFields, "provenance"); ok {
		return provenance
	}
	if strings.HasPrefix(concept.Type, "code_") {
		return "okf.code"
	}
	return "okf.markdown"
}

func sourcePreference(concept *okf.Concept) int {
	switch concept.Type {
	case "code_symbol":
		return 40
	case "code_relation", "code_relation_index":
		return 30
	case "code_file":
		return 20
	default:
		return 10
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func indexConceptsByPath(concepts []*okf.Concept) map[string]*okf.Concept {
	indexed := make(map[string]*okf.Concept, len(concepts))
	for _, concept := range concepts {
		if concept == nil || concept.FilePath == "" {
			continue
		}
		indexed[concept.FilePath] = concept
	}
	return indexed
}

func omissionForHit(hit QueryHit, reason string, count int) ContextOmission {
	return ContextOmission{
		Reason:     reason,
		Title:      hit.Title,
		SourcePath: hit.SourcePath,
		Count:      count,
	}
}

func formatLocation(sourcePath string, startLine, endLine int) string {
	if sourcePath == "" {
		return ""
	}
	if startLine <= 0 {
		return sourcePath
	}
	if endLine <= 0 || endLine == startLine {
		return fmt.Sprintf("%s:%d", sourcePath, startLine)
	}
	return fmt.Sprintf("%s:%d-%d", sourcePath, startLine, endLine)
}

func sourcePathForConcept(concept *okf.Concept) string {
	if concept == nil {
		return ""
	}
	if sourcePath, ok := stringCustomField(concept.CustomFields, "source_path"); ok {
		return filepath.Clean(sourcePath)
	}
	if strings.HasPrefix(concept.Resource, "code://repo/") {
		return filepath.Clean(strings.TrimPrefix(concept.Resource, "code://repo/"))
	}
	if concept.Resource != "" {
		return filepath.Clean(concept.Resource)
	}
	return ""
}

func stringCustomField(fields map[string]interface{}, key string) (string, bool) {
	if fields == nil {
		return "", false
	}
	value, ok := fields[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", false
	}
	return text, true
}

func boolCustomField(fields map[string]interface{}, key string) bool {
	if fields == nil {
		return false
	}
	value, ok := fields[key]
	if !ok {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func duplicateSourcesForConcept(concept *okf.Concept) []KnowledgePathResolution {
	if concept == nil || concept.CustomFields == nil {
		return nil
	}
	if duplicates, ok := concept.CustomFields["duplicate_sources"].([]KnowledgePathResolution); ok {
		return duplicates
	}
	return nil
}

func safeRepoPath(repoRoot, sourcePath string) (string, bool) {
	if filepath.IsAbs(sourcePath) {
		return "", false
	}
	clean := filepath.Clean(sourcePath)
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", false
	}
	fullPath := filepath.Join(repoRoot, clean)
	canonicalRoot, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return "", false
	}
	if canonicalFull, err := filepath.EvalSymlinks(fullPath); err == nil {
		rel, err := filepath.Rel(canonicalRoot, canonicalFull)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return "", false
		}
		return canonicalFull, true
	}
	return fullPath, true
}

func extractSnippet(content, query string, budgetTokens int) (string, int, int, int) {
	if budgetTokens <= 0 {
		return "", 0, 0, 0
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	matchLine := -1
	lowerQuery := strings.ToLower(strings.TrimSpace(query))
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), lowerQuery) {
			matchLine = i
			break
		}
	}
	if matchLine == -1 {
		return "", 0, 0, len(nonEmptyLines(lines))
	}

	start := matchLine
	end := matchLine
	snippet := trimLineAroundQuery(lines[matchLine], query, budgetTokens)
	if snippet == "" {
		return "", 0, 0, len(nonEmptyLines(lines))
	}
	omitted := countNonEmptyLinesOutside(lines, start, end)
	return snippet, start + 1, end + 1, omitted
}

func extractPlannedSnippet(content, query string, startLine, endLine, budgetTokens int) (string, int, int, int) {
	if budgetTokens <= 0 {
		return "", 0, 0, 0
	}
	if startLine <= 0 || endLine <= 0 {
		return extractSnippet(content, query, budgetTokens)
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		return "", 0, 0, 0
	}
	start := startLine - 1 - contextSurroundingLines
	if start < 0 {
		start = 0
	}
	end := endLine - 1 + contextSurroundingLines
	if end >= len(lines) {
		end = len(lines) - 1
	}
	for end > endLine-1 && strings.TrimSpace(lines[end]) == "" {
		end--
	}
	if start > end || start >= len(lines) {
		return "", 0, 0, len(nonEmptyLines(lines))
	}
	snippetLines := append([]string(nil), lines[start:end+1]...)
	snippet := strings.TrimRight(strings.Join(snippetLines, "\n"), "\n")
	if snippet == "" {
		return "", 0, 0, countNonEmptyLinesOutside(lines, start, end)
	}
	if estimateTokens(snippet) > budgetTokens {
		coreStart := startLine - 1
		coreEnd := endLine - 1
		if coreStart < 0 {
			coreStart = 0
		}
		if coreEnd >= len(lines) {
			coreEnd = len(lines) - 1
		}
		if coreStart <= coreEnd {
			snippet = strings.TrimRight(strings.Join(lines[coreStart:coreEnd+1], "\n"), "\n")
			start = coreStart
			end = coreEnd
		}
	}
	if estimateTokens(snippet) > budgetTokens {
		return "", 0, 0, countNonEmptyLinesOutside(lines, start, end)
	}
	omitted := countNonEmptyLinesOutside(lines, start, end)
	return snippet, start + 1, end + 1, omitted
}

func trimLineAroundQuery(line, query string, budgetTokens int) string {
	maxRunes := budgetTokens * 4
	if maxRunes <= 0 {
		return ""
	}
	lineRunes := []rune(strings.TrimRight(line, "\n"))
	if len(lineRunes) == 0 {
		return ""
	}
	lowerQueryRunes := []rune(strings.ToLower(strings.TrimSpace(query)))
	if len(lowerQueryRunes) == 0 {
		return ""
	}
	matchStart := indexRunes([]rune(strings.ToLower(string(lineRunes))), lowerQueryRunes)
	if matchStart == -1 {
		return ""
	}
	if len(lineRunes) <= maxRunes {
		return string(lineRunes)
	}
	if len(lowerQueryRunes) > maxRunes {
		return ""
	}

	padding := (maxRunes - len(lowerQueryRunes)) / 2
	start := matchStart - padding
	if start < 0 {
		start = 0
	}
	end := start + maxRunes
	if end > len(lineRunes) {
		end = len(lineRunes)
		start = end - maxRunes
		if start < 0 {
			start = 0
		}
	}
	snippet := string(lineRunes[start:end])
	if !strings.Contains(strings.ToLower(snippet), strings.ToLower(strings.TrimSpace(query))) {
		return ""
	}
	return snippet
}

func indexRunes(haystack, needle []rune) int {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return -1
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		matched := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				matched = false
				break
			}
		}
		if matched {
			return i
		}
	}
	return -1
}

func countNonEmptyLinesOutside(lines []string, start, end int) int {
	count := 0
	for i, line := range lines {
		if i >= start && i <= end {
			continue
		}
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func estimateTokens(text string) int {
	runes := []rune(text)
	if len(runes) == 0 {
		return 0
	}
	return int(math.Ceil(float64(len(runes)) / 4.0))
}

func nonEmptyLines(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}
	return result
}

func checkContext(ctx stdctx.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func staleWarnings(freshness *Freshness) []string {
	if freshness == nil || !freshness.Stale {
		return []string{}
	}
	return []string{"knowledge is stale relative to HEAD"}
}

func knowledgeNotInitialized(repoRoot string) toolError {
	return toolError{
		code:        ErrKnowledgeNotInitialized,
		message:     ".okf/knowledge does not exist",
		remediation: "Run `bingo okf init --repo " + repoRoot + "` or `bingo okf refresh --mode full`.",
	}
}

func knowledgePathInvalid(path, message string, err error) toolError {
	fullMessage := message + ": " + path
	if err != nil {
		fullMessage += ": " + err.Error()
	}
	return toolError{
		code:        ErrInvalidRequest,
		message:     fullMessage,
		remediation: "Remove the invalid knowledge path from knowledge_paths[] or point it at a readable directory.",
	}
}

type toolError struct {
	code        string
	message     string
	remediation string
}

func (e toolError) Error() string {
	return e.message
}

func failure(operation, repoRoot, knowledgeDir string, freshness *Freshness, err error) ToolEnvelope {
	toolErr := &ToolError{
		Code:    "internal_error",
		Message: err.Error(),
	}
	var typed toolError
	if errors.As(err, &typed) {
		toolErr.Code = typed.code
		toolErr.Message = typed.message
		toolErr.Remediation = typed.remediation
	}

	return ToolEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     operation,
		OK:            false,
		Mutating:      isMutatingOperation(operation),
		RepoRoot:      repoRoot,
		KnowledgeDir:  knowledgeDir,
		Freshness:     freshness,
		Warnings:      []string{},
		Error:         toolErr,
	}
}

func isMutatingOperation(operation string) bool {
	switch operation {
	case OperationInit, OperationRefresh:
		return true
	default:
		return false
	}
}
