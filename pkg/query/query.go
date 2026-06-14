// Package query provides advanced search and filtering for OKF concepts.
package query

import (
	"regexp"
	"strings"
	"sync"
)

var symbolLinePattern = regexp.MustCompile("^- `([^`]+)` `([^`]+)` \\(([^)]+)\\) at `([^`]+)`$")

var (
	codeLanguagePattern = regexp.MustCompile("(?m)^- Language: `([^`]+)`$")
	codeFilePathPattern = regexp.MustCompile("(?m)^- Path: `([^`]+)`$")
	relationRowPattern  = regexp.MustCompile("^\\| ([^|]+) \\| `([^`]+)` \\| `([^`]+)` \\| `([^`]+)` \\| ([^|]+) \\|$")
)

// Concept represents the minimal concept interface needed for querying.
type Concept struct {
	Type        string
	Title       string
	Description string
	Resource    string
	Tags        []string
	Content     string
	FilePath    string
}

// KnowledgeBundle represents a collection of concepts for querying.
type KnowledgeBundle struct {
	Concepts []*Concept
	mu       sync.RWMutex
	index    *Index
}

// Index stores in-memory lookup structures for common query dimensions.
type Index struct {
	concepts         []*Concept
	snapshots        []conceptSnapshot
	byType           map[string][]*Concept
	byTag            map[string][]*Concept
	byResource       map[string][]*Concept
	byTitle          map[string][]*Concept
	byCodeLanguage   map[string][]*Concept
	byCodeFilePath   map[string][]*Concept
	bySymbolKind     map[string][]*Concept
	byQualifiedName  map[string][]*Concept
	byRelationKind   map[string][]*Concept
	symbolsByName    map[string][]indexedSymbol
	symbolsByConcept map[*Concept][]SymbolMatch
}

type indexedSymbol struct {
	concept *Concept
	match   SymbolMatch
}

type conceptSnapshot struct {
	Type     string
	Title    string
	Resource string
	Tags     []string
	Content  string
}

// SearchResult describes a concept match with optional structured symbol hits.
type SearchResult struct {
	Concept       *Concept
	SymbolMatches []SymbolMatch
}

// SymbolMatch describes a symbol hit parsed from generated OKF concept content.
type SymbolMatch struct {
	Kind       string
	Name       string
	Visibility string
	Location   string
}

// Query represents a search query with multiple criteria.
type Query struct {
	Type              string
	Tags              []string
	Resource          string
	Text              string
	TitleRegex        string
	DescriptionRegex  string
	ContentRegex      string
	CodeLanguage      string
	CodeFilePath      string
	CodeSymbolKind    string
	CodeQualifiedName string
	CodeRelationKind  string
}

// Builder helps construct complex queries fluently.
type Builder struct {
	q Query
}

// New creates a new query builder.
func New() *Builder {
	return &Builder{}
}

// WithType adds a type filter.
func (b *Builder) WithType(conceptType string) *Builder {
	b.q.Type = conceptType
	return b
}

// WithTags adds tag filters (concept must have all these tags).
func (b *Builder) WithTags(tags ...string) *Builder {
	b.q.Tags = tags
	return b
}

// WithResource adds a resource filter.
func (b *Builder) WithResource(resource string) *Builder {
	b.q.Resource = resource
	return b
}

// WithText adds a full-text search.
func (b *Builder) WithText(text string) *Builder {
	b.q.Text = text
	return b
}

// WithTitleRegex adds a title regex filter.
func (b *Builder) WithTitleRegex(pattern string) *Builder {
	b.q.TitleRegex = pattern
	return b
}

// WithDescriptionRegex adds a description regex filter.
func (b *Builder) WithDescriptionRegex(pattern string) *Builder {
	b.q.DescriptionRegex = pattern
	return b
}

// WithContentRegex adds a content regex filter.
func (b *Builder) WithContentRegex(pattern string) *Builder {
	b.q.ContentRegex = pattern
	return b
}

// WithCodeLanguage filters code concepts by generated source language metadata.
func (b *Builder) WithCodeLanguage(language string) *Builder {
	b.q.CodeLanguage = language
	return b
}

// WithCodeFilePath filters code concepts by repository-relative source path.
func (b *Builder) WithCodeFilePath(filePath string) *Builder {
	b.q.CodeFilePath = filePath
	return b
}

// WithCodeSymbolKind filters code concepts by generated symbol kind.
func (b *Builder) WithCodeSymbolKind(kind string) *Builder {
	b.q.CodeSymbolKind = kind
	return b
}

// WithCodeQualifiedName filters code concepts by generated symbol or relation qualified name.
func (b *Builder) WithCodeQualifiedName(name string) *Builder {
	b.q.CodeQualifiedName = name
	return b
}

// WithCodeRelationKind filters generated relation index concepts by relation kind.
func (b *Builder) WithCodeRelationKind(kind string) *Builder {
	b.q.CodeRelationKind = kind
	return b
}

// Build returns the final query.
func (b *Builder) Build() *Query {
	return &b.q
}

// Execute runs the query against a bundle and returns matching concepts.
func (q *Query) Execute(bundle *KnowledgeBundle) []*Concept {
	if bundle == nil {
		return nil
	}
	idx := bundle.ensureIndex()
	return FilterConcepts(q.indexedCandidates(bundle, idx), q.Matches)
}

func (q *Query) indexedCandidates(bundle *KnowledgeBundle, idx *Index) []*Concept {
	if idx == nil {
		return bundle.Concepts
	}

	candidates := bundle.Concepts
	if q.Type != "" {
		candidates = idx.byType[q.Type]
	}
	for _, tag := range q.Tags {
		candidates = intersectConcepts(candidates, idx.byTag[tag])
	}
	if q.Resource != "" {
		candidates = intersectConcepts(candidates, indexedResourceCandidates(idx, q.Resource))
	}
	if q.Text != "" {
		candidates = intersectConcepts(candidates, indexedTextCandidates(bundle, idx, q.Text))
	}
	if q.CodeLanguage != "" {
		candidates = intersectConcepts(candidates, idx.byCodeLanguage[strings.ToLower(q.CodeLanguage)])
	}
	if q.CodeFilePath != "" {
		candidates = intersectConcepts(candidates, indexedContainsCandidates(idx.byCodeFilePath, q.CodeFilePath))
	}
	if q.CodeSymbolKind != "" {
		candidates = intersectConcepts(candidates, idx.bySymbolKind[strings.ToLower(q.CodeSymbolKind)])
	}
	if q.CodeQualifiedName != "" {
		candidates = intersectConcepts(candidates, indexedContainsCandidates(idx.byQualifiedName, q.CodeQualifiedName))
	}
	if q.CodeRelationKind != "" {
		candidates = intersectConcepts(candidates, idx.byRelationKind[strings.ToLower(q.CodeRelationKind)])
	}
	return candidates
}

// Matches checks if a concept matches the query.
func (q *Query) Matches(c *Concept) bool {
	if q.Type != "" && c.Type != q.Type {
		return false
	}

	if len(q.Tags) > 0 {
		tagSet := make(map[string]bool)
		for _, t := range c.Tags {
			tagSet[t] = true
		}
		for _, requiredTag := range q.Tags {
			if !tagSet[requiredTag] {
				return false
			}
		}
	}

	if q.Resource != "" && !containsFold(c.Resource, q.Resource) {
		return false
	}

	if q.Text != "" {
		textLower := strings.ToLower(q.Text)
		found := containsFold(c.Title, textLower) ||
			containsFold(c.Description, textLower) ||
			containsFold(c.Content, textLower)
		if !found {
			return false
		}
	}

	if q.TitleRegex != "" {
		matched, _ := regexp.MatchString(q.TitleRegex, c.Title)
		if !matched {
			return false
		}
	}

	if q.DescriptionRegex != "" {
		matched, _ := regexp.MatchString(q.DescriptionRegex, c.Description)
		if !matched {
			return false
		}
	}

	if q.ContentRegex != "" {
		matched, _ := regexp.MatchString(q.ContentRegex, c.Content)
		if !matched {
			return false
		}
	}

	if q.CodeLanguage != "" && !conceptHasCodeLanguage(c, q.CodeLanguage) {
		return false
	}
	if q.CodeFilePath != "" && !conceptHasCodeFilePath(c, q.CodeFilePath) {
		return false
	}
	if q.CodeSymbolKind != "" && !conceptHasCodeSymbolKind(c, q.CodeSymbolKind) {
		return false
	}
	if q.CodeQualifiedName != "" && !conceptHasCodeQualifiedName(c, q.CodeQualifiedName) {
		return false
	}
	if q.CodeRelationKind != "" && !conceptHasCodeRelationKind(c, q.CodeRelationKind) {
		return false
	}

	return true
}

// FilterConcepts filters a slice of concepts by predicate.
func FilterConcepts(concepts []*Concept, pred func(*Concept) bool) []*Concept {
	var result []*Concept
	for _, c := range concepts {
		if pred(c) {
			result = append(result, c)
		}
	}
	return result
}

// Search performs a full-text search across concept titles, descriptions, and content.
func Search(bundle *KnowledgeBundle, text string) []*Concept {
	if bundle == nil {
		return nil
	}
	idx := bundle.ensureIndex()
	return FilterConcepts(indexedTextCandidates(bundle, idx, text), func(c *Concept) bool {
		return containsFold(c.Title, text) || containsFold(c.Description, text) || containsFold(c.Content, text)
	})
}

// SearchWithMatches performs full-text search and includes structured symbol matches when available.
func SearchWithMatches(bundle *KnowledgeBundle, text string) []SearchResult {
	if bundle == nil {
		return nil
	}
	idx := bundle.ensureIndex()

	var results []SearchResult
	for _, concept := range indexedTextCandidates(bundle, idx, text) {
		symbols := idx.matchingSymbols(concept, text)
		if containsFold(concept.Title, text) || containsFold(concept.Description, text) || containsFold(concept.Content, text) || len(symbols) > 0 {
			results = append(results, SearchResult{Concept: concept, SymbolMatches: symbols})
		}
	}
	return results
}

// FilterByType returns all concepts of the given type.
func FilterByType(bundle *KnowledgeBundle, conceptType string) []*Concept {
	if bundle == nil {
		return nil
	}
	idx := bundle.ensureIndex()
	return append([]*Concept(nil), idx.byType[conceptType]...)
}

// FilterByTag returns all concepts containing the given tag.
func FilterByTag(bundle *KnowledgeBundle, tag string) []*Concept {
	if bundle == nil {
		return nil
	}
	idx := bundle.ensureIndex()
	return append([]*Concept(nil), idx.byTag[tag]...)
}

// BuildIndex rebuilds in-memory lookup maps for common query dimensions.
func (b *KnowledgeBundle) BuildIndex() {
	if b == nil {
		return
	}
	idx := buildIndex(b.Concepts)
	b.mu.Lock()
	b.index = idx
	b.mu.Unlock()
}

func buildIndex(concepts []*Concept) *Index {
	idx := &Index{
		concepts:         append([]*Concept(nil), concepts...),
		snapshots:        make([]conceptSnapshot, 0, len(concepts)),
		byType:           make(map[string][]*Concept),
		byTag:            make(map[string][]*Concept),
		byResource:       make(map[string][]*Concept),
		byTitle:          make(map[string][]*Concept),
		byCodeLanguage:   make(map[string][]*Concept),
		byCodeFilePath:   make(map[string][]*Concept),
		bySymbolKind:     make(map[string][]*Concept),
		byQualifiedName:  make(map[string][]*Concept),
		byRelationKind:   make(map[string][]*Concept),
		symbolsByName:    make(map[string][]indexedSymbol),
		symbolsByConcept: make(map[*Concept][]SymbolMatch),
	}

	for _, concept := range concepts {
		if concept == nil {
			idx.snapshots = append(idx.snapshots, conceptSnapshot{})
			continue
		}
		idx.snapshots = append(idx.snapshots, snapshotConcept(concept))
		if concept.Type != "" {
			idx.byType[concept.Type] = append(idx.byType[concept.Type], concept)
		}
		for _, tag := range concept.Tags {
			idx.byTag[tag] = append(idx.byTag[tag], concept)
		}
		if concept.Resource != "" {
			idx.byResource[concept.Resource] = append(idx.byResource[concept.Resource], concept)
		}
		if concept.Title != "" {
			idx.byTitle[strings.ToLower(concept.Title)] = append(idx.byTitle[strings.ToLower(concept.Title)], concept)
		}
		indexCodeMetadata(idx, concept)
		for _, symbol := range matchingSymbols(concept.Content, "") {
			key := strings.ToLower(symbol.Name)
			idx.symbolsByName[key] = append(idx.symbolsByName[key], indexedSymbol{concept: concept, match: symbol})
			idx.symbolsByConcept[concept] = append(idx.symbolsByConcept[concept], symbol)
		}
	}

	return idx
}

func (b *KnowledgeBundle) ensureIndex() *Index {
	if b == nil {
		return nil
	}
	b.mu.RLock()
	idx := b.index
	fresh := indexMatchesConcepts(idx, b.Concepts)
	b.mu.RUnlock()
	if fresh {
		return idx
	}

	rebuilt := buildIndex(b.Concepts)
	b.mu.Lock()
	if !indexMatchesConcepts(b.index, b.Concepts) {
		b.index = rebuilt
	}
	idx = b.index
	b.mu.Unlock()
	return idx
}

func indexMatchesConcepts(idx *Index, concepts []*Concept) bool {
	if idx == nil || len(idx.concepts) != len(concepts) || len(idx.snapshots) != len(concepts) {
		return false
	}
	for i, concept := range concepts {
		if idx.concepts[i] != concept {
			return false
		}
		if !idx.snapshots[i].matches(concept) {
			return false
		}
	}
	return true
}

func snapshotConcept(concept *Concept) conceptSnapshot {
	if concept == nil {
		return conceptSnapshot{}
	}
	return conceptSnapshot{
		Type:     concept.Type,
		Title:    concept.Title,
		Resource: concept.Resource,
		Tags:     append([]string(nil), concept.Tags...),
		Content:  concept.Content,
	}
}

func (s conceptSnapshot) matches(concept *Concept) bool {
	if concept == nil {
		return s.Type == "" && s.Title == "" && s.Resource == "" && s.Content == "" && len(s.Tags) == 0
	}
	if s.Type != concept.Type || s.Title != concept.Title || s.Resource != concept.Resource || s.Content != concept.Content || len(s.Tags) != len(concept.Tags) {
		return false
	}
	for i, tag := range concept.Tags {
		if s.Tags[i] != tag {
			return false
		}
	}
	return true
}

func indexedTextCandidates(bundle *KnowledgeBundle, idx *Index, text string) []*Concept {
	if text == "" || idx == nil {
		return bundle.Concepts
	}

	candidateSet := make(map[*Concept]struct{}, len(bundle.Concepts))
	add := func(concept *Concept) {
		if concept == nil {
			return
		}
		candidateSet[concept] = struct{}{}
	}

	lower := strings.ToLower(text)
	for title, concepts := range idx.byTitle {
		if strings.Contains(title, lower) {
			for _, concept := range concepts {
				add(concept)
			}
		}
	}
	for name, symbols := range idx.symbolsByName {
		if strings.Contains(name, lower) {
			for _, symbol := range symbols {
				add(symbol.concept)
			}
		}
	}

	for _, concept := range bundle.Concepts {
		if containsFold(concept.Description, text) || containsFold(concept.Content, text) {
			add(concept)
		}
	}

	candidates := make([]*Concept, 0, len(candidateSet))
	for _, concept := range bundle.Concepts {
		if _, ok := candidateSet[concept]; ok {
			candidates = append(candidates, concept)
		}
	}
	return candidates
}

func indexedResourceCandidates(index *Index, resource string) []*Concept {
	if index == nil {
		return nil
	}
	var candidates []*Concept
	for indexedResource, concepts := range index.byResource {
		if containsFold(indexedResource, resource) {
			candidates = append(candidates, concepts...)
		}
	}
	return candidates
}

func indexCodeMetadata(idx *Index, concept *Concept) {
	if idx == nil || concept == nil {
		return
	}

	for _, match := range codeLanguagePattern.FindAllStringSubmatch(concept.Content, -1) {
		idx.byCodeLanguage[normalizedCodeKey(match[1])] = append(idx.byCodeLanguage[normalizedCodeKey(match[1])], concept)
	}
	for _, match := range codeFilePathPattern.FindAllStringSubmatch(concept.Content, -1) {
		idx.byCodeFilePath[normalizedCodeKey(match[1])] = append(idx.byCodeFilePath[normalizedCodeKey(match[1])], concept)
	}

	for _, symbol := range matchingSymbols(concept.Content, "") {
		idx.bySymbolKind[normalizedCodeKey(symbol.Kind)] = append(idx.bySymbolKind[normalizedCodeKey(symbol.Kind)], concept)
		idx.byQualifiedName[normalizedCodeKey(symbol.Name)] = append(idx.byQualifiedName[normalizedCodeKey(symbol.Name)], concept)
	}

	for _, line := range strings.Split(concept.Content, "\n") {
		parts := relationRowPattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(parts) != 6 {
			continue
		}
		idx.byRelationKind[normalizedCodeKey(parts[1])] = append(idx.byRelationKind[normalizedCodeKey(parts[1])], concept)
		idx.byQualifiedName[normalizedCodeKey(parts[2])] = append(idx.byQualifiedName[normalizedCodeKey(parts[2])], concept)
		idx.byQualifiedName[normalizedCodeKey(parts[3])] = append(idx.byQualifiedName[normalizedCodeKey(parts[3])], concept)
	}
}

func indexedContainsCandidates(m map[string][]*Concept, substr string) []*Concept {
	if substr == "" {
		return nil
	}
	var candidates []*Concept
	needle := normalizedCodeKey(substr)
	for key, concepts := range m {
		if strings.Contains(key, needle) {
			candidates = append(candidates, concepts...)
		}
	}
	return candidates
}

func conceptHasCodeLanguage(c *Concept, language string) bool {
	if c == nil || language == "" {
		return false
	}
	needle := normalizedCodeKey(language)
	for _, match := range codeLanguagePattern.FindAllStringSubmatch(c.Content, -1) {
		if normalizedCodeKey(match[1]) == needle {
			return true
		}
	}
	return false
}

func conceptHasCodeFilePath(c *Concept, filePath string) bool {
	if c == nil || filePath == "" {
		return false
	}
	needle := normalizedCodeKey(filePath)
	for _, match := range codeFilePathPattern.FindAllStringSubmatch(c.Content, -1) {
		if strings.Contains(normalizedCodeKey(match[1]), needle) {
			return true
		}
	}
	return false
}

func conceptHasCodeSymbolKind(c *Concept, kind string) bool {
	if c == nil || kind == "" {
		return false
	}
	needle := normalizedCodeKey(kind)
	for _, symbol := range matchingSymbols(c.Content, "") {
		if normalizedCodeKey(symbol.Kind) == needle {
			return true
		}
	}
	return false
}

func conceptHasCodeQualifiedName(c *Concept, name string) bool {
	if c == nil || name == "" {
		return false
	}
	needle := normalizedCodeKey(name)
	for _, symbol := range matchingSymbols(c.Content, "") {
		if strings.Contains(normalizedCodeKey(symbol.Name), needle) {
			return true
		}
	}
	for _, line := range strings.Split(c.Content, "\n") {
		parts := relationRowPattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(parts) != 6 {
			continue
		}
		if strings.Contains(normalizedCodeKey(parts[2]), needle) || strings.Contains(normalizedCodeKey(parts[3]), needle) {
			return true
		}
	}
	return false
}

func conceptHasCodeRelationKind(c *Concept, kind string) bool {
	if c == nil || kind == "" {
		return false
	}
	needle := normalizedCodeKey(kind)
	for _, line := range strings.Split(c.Content, "\n") {
		parts := relationRowPattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(parts) == 6 && normalizedCodeKey(parts[1]) == needle {
			return true
		}
	}
	return false
}

func normalizedCodeKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func intersectConcepts(left, right []*Concept) []*Concept {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	set := make(map[*Concept]struct{}, len(right))
	for _, concept := range right {
		set[concept] = struct{}{}
	}
	result := make([]*Concept, 0, min(len(left), len(right)))
	for _, concept := range left {
		if _, ok := set[concept]; ok {
			result = append(result, concept)
		}
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (idx *Index) matchingSymbols(concept *Concept, text string) []SymbolMatch {
	if idx == nil || text == "" {
		return nil
	}
	var matches []SymbolMatch
	for _, symbol := range idx.symbolsByConcept[concept] {
		if !containsFold(symbol.Name, text) {
			continue
		}
		matches = append(matches, symbol)
	}
	return matches
}

func matchingSymbols(content, text string) []SymbolMatch {
	var matches []SymbolMatch
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		parts := symbolLinePattern.FindStringSubmatch(line)
		if len(parts) != 5 {
			continue
		}
		name := parts[2]
		if text != "" && !containsFold(name, text) {
			continue
		}
		matches = append(matches, SymbolMatch{
			Kind:       parts[1],
			Name:       name,
			Visibility: parts[3],
			Location:   parts[4],
		})
	}
	return matches
}

func containsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
