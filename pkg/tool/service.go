// Package tool exposes OKF repository knowledge operations through a stable,
// agent-facing service API.
package tool

import (
	stdctx "context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/superops-team/okf/pkg/git"
	"github.com/superops-team/okf/pkg/okf"
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
	Query        string `json:"query"`
	Limit        int    `json:"limit,omitempty"`
	Type         string `json:"type,omitempty"`
	Tag          string `json:"tag,omitempty"`
	FilePath     string `json:"file_path,omitempty"`
	SymbolKind   string `json:"symbol_kind,omitempty"`
	RelationKind string `json:"relation_kind,omitempty"`
}

type ContextRequest struct {
	Query        string `json:"query"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

type StatusResult struct {
	Ready        bool `json:"ready"`
	ConceptCount int  `json:"concept_count"`
	UniqueTypes  int  `json:"unique_types"`
	UniqueTags   int  `json:"unique_tags"`
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
	Query   string     `json:"query"`
	Results []QueryHit `json:"results"`
}

type ContextResult struct {
	Query        string            `json:"query"`
	BudgetTokens int               `json:"budget_tokens"`
	UsedTokens   int               `json:"used_tokens"`
	Omitted      int               `json:"omitted"`
	Omissions    []ContextOmission `json:"omissions"`
	Items        []ContextItem     `json:"items"`
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

type QueryHit struct {
	Title          string `json:"title"`
	Type           string `json:"type"`
	Resource       string `json:"resource,omitempty"`
	FilePath       string `json:"file_path,omitempty"`
	SourcePath     string `json:"source_path,omitempty"`
	Location       string `json:"location,omitempty"`
	ConceptPath    string `json:"concept_path,omitempty"`
	StartLine      int    `json:"start_line,omitempty"`
	EndLine        int    `json:"end_line,omitempty"`
	SymbolKind     string `json:"symbol_kind,omitempty"`
	QualifiedName  string `json:"qualified_name,omitempty"`
	RelationKind   string `json:"relation_kind,omitempty"`
	RelationSource string `json:"relation_source,omitempty"`
	RelationTarget string `json:"relation_target,omitempty"`
	Score          int    `json:"score"`
	Reason         string `json:"reason"`
	Provenance     string `json:"provenance"`
	exactness      int
	sourceRank     int
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

	bundle, err := okf.LoadBundle(resolved.knowledgeDir, okf.DefaultLoadOptions())
	if err != nil {
		return failure(OperationStatus, resolved.repoRoot, resolved.knowledgeDir, freshness, err)
	}
	stats := bundle.Stats()

	return ToolEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     OperationStatus,
		OK:            true,
		RepoRoot:      resolved.repoRoot,
		KnowledgeDir:  resolved.knowledgeDir,
		Freshness:     freshness,
		Warnings:      []string{},
		Result: StatusResult{
			Ready:        true,
			ConceptCount: stats.TotalConcepts,
			UniqueTypes:  stats.UniqueTypes,
			UniqueTags:   stats.UniqueTags,
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

	bundle, err := okf.LoadBundle(resolved.knowledgeDir, okf.DefaultLoadOptions())
	if err != nil {
		return failure(OperationQuery, resolved.repoRoot, resolved.knowledgeDir, freshness, err)
	}

	hits := rankConcepts(bundle.Concepts, req.Query, filtersFromQueryRequest(req))
	if req.Limit > 0 && len(hits) > req.Limit {
		hits = hits[:req.Limit]
	}

	return ToolEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     OperationQuery,
		OK:            true,
		RepoRoot:      resolved.repoRoot,
		KnowledgeDir:  resolved.knowledgeDir,
		Freshness:     freshness,
		Warnings:      staleWarnings(freshness),
		Result: QueryResult{
			Query:   req.Query,
			Results: hits,
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

	bundle, err := okf.LoadBundle(resolved.knowledgeDir, okf.DefaultLoadOptions())
	if err != nil {
		return failure(OperationContext, resolved.repoRoot, resolved.knowledgeDir, freshness, err)
	}

	budget := req.BudgetTokens
	if budget <= 0 {
		budget = 4000
	}
	hits := rankConcepts(bundle.Concepts, req.Query, queryFilters{})
	items := []ContextItem{}
	omissions := []ContextOmission{}
	warnings := staleWarnings(freshness)
	usedTokens := 0
	omitted := 0
	conceptsByPath := indexConceptsByPath(bundle.Concepts)
	sourceCache := map[string][]byte{}
	for i, hit := range hits {
		if err := checkContext(ctx); err != nil {
			return failure(OperationContext, resolved.repoRoot, resolved.knowledgeDir, freshness, err)
		}
		if usedTokens >= budget {
			remaining := len(hits) - i
			omissions = append(omissions, omissionForHit(hit, "budget_exceeded", remaining))
			omitted += remaining
			break
		}
		concept := conceptsByPath[hit.ConceptPath]
		if concept == nil {
			omissions = append(omissions, omissionForHit(hit, "concept_missing", 1))
			omitted++
			continue
		}
		sourcePath := sourcePathForConcept(concept)
		if sourcePath == "" {
			warnings = append(warnings, "no source path for "+concept.Title)
			omissions = append(omissions, ContextOmission{Reason: "no_source_path", Title: concept.Title, Count: 1})
			omitted++
			continue
		}
		fullPath, ok := safeRepoPath(resolved.repoRoot, sourcePath)
		if !ok {
			warnings = append(warnings, "source path escapes repository: "+sourcePath)
			omissions = append(omissions, ContextOmission{Reason: "source_path_escapes_repo", Title: concept.Title, SourcePath: sourcePath, Count: 1})
			omitted++
			continue
		}
		data, ok := sourceCache[fullPath]
		var readErr error
		if !ok {
			data, readErr = os.ReadFile(fullPath)
			if readErr == nil {
				sourceCache[fullPath] = data
			}
		}
		if readErr != nil {
			warnings = append(warnings, "source file missing: "+sourcePath)
			omissions = append(omissions, ContextOmission{Reason: "source_missing", Title: concept.Title, SourcePath: sourcePath, Count: 1})
			items = append(items, ContextItem{
				Title:         concept.Title,
				Type:          concept.Type,
				SourcePath:    sourcePath,
				Location:      formatLocation(sourcePath, hit.StartLine, hit.EndLine),
				StartLine:     hit.StartLine,
				EndLine:       hit.EndLine,
				Snippet:       "",
				TokenEstimate: 0,
				Score:         hit.Score,
				Reason:        hit.Reason,
				Provenance:    hit.Provenance,
			})
			omitted++
			continue
		}
		snippet, startLine, endLine, sourceOmitted := extractSnippet(string(data), req.Query, budget-usedTokens)
		if snippet == "" {
			omissions = append(omissions, ContextOmission{Reason: "snippet_empty", Title: concept.Title, SourcePath: sourcePath, Count: 1})
			omitted++
			continue
		}
		tokens := estimateTokens(snippet)
		if tokens <= 0 {
			omissions = append(omissions, ContextOmission{Reason: "zero_token_snippet", Title: concept.Title, SourcePath: sourcePath, Count: 1})
			omitted++
			continue
		}
		usedTokens += tokens
		omitted += sourceOmitted
		if sourceOmitted > 0 {
			omissions = append(omissions, ContextOmission{Reason: "snippet_truncated", Title: concept.Title, SourcePath: sourcePath, Count: sourceOmitted})
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
			Reason:        hit.Reason,
			Provenance:    "repo.source",
		})
	}

	return ToolEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     OperationContext,
		OK:            true,
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
		},
	}
}

type resolvedConfig struct {
	repoRoot     string
	knowledgeDir string
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

	knowledgeDir := s.cfg.KnowledgeDir
	if knowledgeDir == "" {
		knowledgeDir = git.DefaultConfig().KnowledgeDir
	}
	if !filepath.IsAbs(knowledgeDir) {
		knowledgeDir = filepath.Join(repoRoot, knowledgeDir)
	}

	return resolvedConfig{repoRoot: repoRoot, knowledgeDir: knowledgeDir}, nil
}

func readFreshness(resolved resolvedConfig) *Freshness {
	head, _ := git.GetCurrentCommit(resolved.repoRoot)
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
	Type         string
	Tag          string
	FilePath     string
	SymbolKind   string
	RelationKind string
}

func filtersFromQueryRequest(req QueryRequest) queryFilters {
	filePath := strings.TrimSpace(req.FilePath)
	if filePath != "" {
		filePath = filepath.Clean(filePath)
	}
	return queryFilters{
		Type:         strings.TrimSpace(req.Type),
		Tag:          strings.TrimSpace(req.Tag),
		FilePath:     filePath,
		SymbolKind:   strings.TrimSpace(req.SymbolKind),
		RelationKind: strings.TrimSpace(req.RelationKind),
	}
}

func rankConcepts(concepts []*okf.Concept, query string, filters queryFilters) []QueryHit {
	hits := make([]QueryHit, 0, len(concepts))
	for _, concept := range concepts {
		if concept == nil {
			continue
		}
		if !matchesQueryFilters(concept, filters) {
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
		hits = append(hits, QueryHit{
			Title:          concept.Title,
			Type:           concept.Type,
			Resource:       concept.Resource,
			FilePath:       sourcePath,
			SourcePath:     sourcePath,
			Location:       formatLocation(sourcePath, startLine, endLine),
			ConceptPath:    concept.FilePath,
			StartLine:      startLine,
			EndLine:        endLine,
			SymbolKind:     symbolKind,
			QualifiedName:  qualifiedName,
			RelationKind:   relationKind,
			RelationSource: relationSource,
			RelationTarget: relationTarget,
			Score:          score,
			Reason:         reason,
			Provenance:     provenanceForConcept(concept),
			exactness:      exactness,
			sourceRank:     sourcePreference(concept),
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
			return hits[i].sourceRank > hits[j].sourceRank
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
	if filters.SymbolKind != "" && stringCustomFieldOrEmpty(concept.CustomFields, "symbol_kind") != filters.SymbolKind {
		return false
	}
	if filters.RelationKind != "" && stringCustomFieldOrEmpty(concept.CustomFields, "relation_kind") != filters.RelationKind {
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

func safeRepoPath(repoRoot, sourcePath string) (string, bool) {
	if filepath.IsAbs(sourcePath) {
		return "", false
	}
	clean := filepath.Clean(sourcePath)
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", false
	}
	return filepath.Join(repoRoot, clean), true
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
		RepoRoot:      repoRoot,
		KnowledgeDir:  knowledgeDir,
		Freshness:     freshness,
		Warnings:      []string{},
		Error:         toolErr,
	}
}
