package git

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/superops-team/okf/pkg/okf"
)

const (
	CodeEntityKindRepository = "repository"
	CodeEntityKindFile       = "file"
	CodeEntityKindPackage    = "package"
	CodeEntityKindSymbol     = "symbol"
	CodeEntityKindImport     = "import"

	CodeProvenanceOKFAST    = "okf-go-ast"
	CodeProvenanceOKFRegex  = "okf-regex"
	CodeProvenanceCodeGraph = "codegraph"
)

const (
	codeRepositoryResource = "okf://code/repository"
	codeRelationsResource  = "okf://code/relations"
)

// CodeEntity is the normalized code-dimension node model used by OKF.
type CodeEntity struct {
	ID            string
	ExternalID    string
	RepositoryID  string
	Kind          string
	SymbolKind    string
	Name          string
	QualifiedName string
	Language      string
	FilePath      string
	StartLine     int
	EndLine       int
	StartColumn   int
	EndColumn     int
	Docstring     string
	Signature     string
	Visibility    string
	Exported      bool
	Package       string
	Provenance    string
}

// CodeRelation is the normalized code-dimension edge model used by OKF.
type CodeRelation struct {
	SourceID   string
	TargetID   string
	TargetRef  string
	Kind       string
	Provenance string
	FilePath   string
	Line       int
	Column     int
	Metadata   string
}

// CodeKnowledgeIndex groups normalized code entities and relations.
type CodeKnowledgeIndex struct {
	RepositoryID string
	Entities     []CodeEntity
	Relations    []CodeRelation
}

// CodeGraphNode is the subset of CodeGraph's Node shape needed for compatibility mapping.
type CodeGraphNode struct {
	ID            string
	Kind          string
	Name          string
	QualifiedName string
	FilePath      string
	Language      string
	StartLine     int
	EndLine       int
	StartColumn   int
	EndColumn     int
	Docstring     string
	Signature     string
	Visibility    string
	IsExported    bool
}

// CodeGraphEdge is the subset of CodeGraph's Edge shape needed for compatibility mapping.
type CodeGraphEdge struct {
	Source     string
	Target     string
	Kind       string
	Metadata   map[string]string
	Line       int
	Column     int
	Provenance string
}

// CodeGraphFileRecord is the subset of CodeGraph's FileRecord shape used as freshness metadata.
type CodeGraphFileRecord struct {
	Path        string
	ContentHash string
	Language    string
	NodeCount   int
	Errors      []string
}

// StableCodeEntityID derives OKF's primary code identity from source metadata.
func StableCodeEntityID(entity CodeEntity) string {
	repoID := entity.RepositoryID
	if repoID == "" {
		repoID = "repo"
	}

	pathPart := entity.FilePath
	if pathPart == "" {
		pathPart = entity.Name
	}
	if pathPart == "" {
		pathPart = entity.Kind
	}

	identityKind := entity.Kind
	if entity.SymbolKind != "" {
		identityKind += ":" + entity.SymbolKind
	}

	name := entity.QualifiedName
	if name == "" {
		name = entity.Name
	}
	if name == "" {
		name = pathPart
	}

	id := fmt.Sprintf("code:%s:%s#%s:%s", repoID, pathPart, identityKind, name)
	if entity.StartLine > 0 && entity.EndLine > 0 {
		id += fmt.Sprintf("@L%d-L%d", entity.StartLine, entity.EndLine)
	}
	return id
}

// MapCodeGraphNodeToEntity converts a CodeGraph node into OKF's normalized code entity model.
func MapCodeGraphNodeToEntity(repositoryID string, node CodeGraphNode) CodeEntity {
	entityKind := CodeEntityKindSymbol
	symbolKind := node.Kind
	switch node.Kind {
	case "file":
		entityKind = CodeEntityKindFile
		symbolKind = ""
	case "module", "namespace":
		entityKind = CodeEntityKindPackage
		symbolKind = ""
	}

	entity := CodeEntity{
		ExternalID:    node.ID,
		RepositoryID:  repositoryID,
		Kind:          entityKind,
		SymbolKind:    symbolKind,
		Name:          node.Name,
		QualifiedName: node.QualifiedName,
		Language:      node.Language,
		FilePath:      node.FilePath,
		StartLine:     node.StartLine,
		EndLine:       node.EndLine,
		StartColumn:   node.StartColumn,
		EndColumn:     node.EndColumn,
		Docstring:     node.Docstring,
		Signature:     node.Signature,
		Visibility:    node.Visibility,
		Exported:      node.IsExported,
		Provenance:    CodeProvenanceCodeGraph,
	}
	entity.ID = StableCodeEntityID(entity)
	return entity
}

// MapCodeGraphEdgeToRelation converts a CodeGraph edge into OKF's normalized code relation model.
func MapCodeGraphEdgeToRelation(edge CodeGraphEdge, entities []CodeEntity) CodeRelation {
	byExternalID := make(map[string]CodeEntity, len(entities))
	for _, entity := range entities {
		if entity.ExternalID != "" {
			byExternalID[entity.ExternalID] = entity
		}
	}

	source := byExternalID[edge.Source]
	target := byExternalID[edge.Target]
	relation := CodeRelation{
		SourceID:   source.ID,
		TargetID:   target.ID,
		Kind:       edge.Kind,
		Provenance: CodeProvenanceCodeGraph,
		FilePath:   source.FilePath,
		Line:       edge.Line,
		Column:     edge.Column,
		Metadata:   codeGraphMetadataString(edge.Metadata, edge.Provenance),
	}
	if relation.SourceID == "" {
		relation.SourceID = edge.Source
	}
	if relation.TargetID == "" {
		relation.TargetRef = edge.Target
	}
	return relation
}

// MapCodeGraphFileRecordToEntity converts CodeGraph file freshness metadata into an OKF file entity.
func MapCodeGraphFileRecordToEntity(repositoryID string, record CodeGraphFileRecord) CodeEntity {
	entity := CodeEntity{
		ExternalID:    record.ContentHash,
		RepositoryID:  repositoryID,
		Kind:          CodeEntityKindFile,
		Name:          record.Path,
		QualifiedName: record.Path,
		Language:      record.Language,
		FilePath:      record.Path,
		Provenance:    CodeProvenanceCodeGraph,
	}
	entity.ID = StableCodeEntityID(entity)
	return entity
}

func codeGraphMetadataString(metadata map[string]string, provenance string) string {
	parts := make([]string, 0, len(metadata)+1)
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, key+"="+metadata[key])
	}
	if provenance != "" {
		parts = append(parts, "codegraph_provenance="+provenance)
	}
	return strings.Join(parts, "; ")
}

func buildCodeKnowledgeIndex(repoRoot string, summaries []*FileSummary) CodeKnowledgeIndex {
	repositoryID := repositoryID(repoRoot)
	index := CodeKnowledgeIndex{RepositoryID: repositoryID}
	repository := CodeEntity{
		RepositoryID:  repositoryID,
		Kind:          CodeEntityKindRepository,
		Name:          filepath.Base(repoRoot),
		QualifiedName: filepath.Base(repoRoot),
		Provenance:    CodeProvenanceOKFAST,
	}
	repository.ID = StableCodeEntityID(repository)
	index.Entities = append(index.Entities, repository)

	packageByName := map[string]CodeEntity{}
	for _, summary := range summaries {
		fileEntity := fileCodeEntity(repositoryID, summary)
		index.Entities = append(index.Entities, fileEntity)
		index.Relations = append(index.Relations, CodeRelation{
			SourceID:   repository.ID,
			TargetID:   fileEntity.ID,
			Kind:       "contains",
			Provenance: fileEntity.Provenance,
			FilePath:   summary.RelativePath,
		})

		pkg := packageName(summary)
		if pkg != "" {
			packageEntity, exists := packageByName[pkg]
			if !exists {
				packageEntity = CodeEntity{
					RepositoryID:  repositoryID,
					Kind:          CodeEntityKindPackage,
					Name:          pkg,
					QualifiedName: pkg,
					Language:      summary.Type,
					Provenance:    summaryProvenance(summary),
				}
				packageEntity.ID = StableCodeEntityID(packageEntity)
				packageByName[pkg] = packageEntity
				index.Entities = append(index.Entities, packageEntity)
				index.Relations = append(index.Relations, CodeRelation{SourceID: repository.ID, TargetID: packageEntity.ID, Kind: "contains", Provenance: packageEntity.Provenance})
			}
			index.Relations = append(index.Relations, CodeRelation{SourceID: packageEntity.ID, TargetID: fileEntity.ID, Kind: "contains", Provenance: fileEntity.Provenance, FilePath: summary.RelativePath})
		}

		for _, imp := range summary.Imports {
			importEntity := CodeEntity{
				RepositoryID:  repositoryID,
				Kind:          CodeEntityKindImport,
				Name:          imp,
				QualifiedName: imp,
				Language:      summary.Type,
				FilePath:      summary.RelativePath,
				Provenance:    summaryProvenance(summary),
			}
			importEntity.ID = StableCodeEntityID(importEntity)
			index.Entities = append(index.Entities, importEntity)
			index.Relations = append(index.Relations, CodeRelation{SourceID: fileEntity.ID, TargetID: importEntity.ID, TargetRef: imp, Kind: "imports", Provenance: importEntity.Provenance, FilePath: summary.RelativePath})
		}

		for _, symbol := range summary.Symbols {
			symbolEntity := symbolCodeEntity(repositoryID, symbol, summary)
			index.Entities = append(index.Entities, symbolEntity)
			index.Relations = append(index.Relations, CodeRelation{SourceID: fileEntity.ID, TargetID: symbolEntity.ID, Kind: "contains", Provenance: symbolEntity.Provenance, FilePath: summary.RelativePath, Line: symbol.StartLine})
			if pkg != "" {
				packageEntity := packageByName[pkg]
				index.Relations = append(index.Relations, CodeRelation{SourceID: packageEntity.ID, TargetID: symbolEntity.ID, Kind: "contains", Provenance: symbolEntity.Provenance, FilePath: summary.RelativePath, Line: symbol.StartLine})
			}
		}
	}

	index.Entities = dedupeCodeEntities(index.Entities)
	sortCodeEntities(index.Entities)
	index.Relations = dedupeCodeRelations(index.Relations)
	sortCodeRelations(index.Relations)
	return index
}

func fileCodeEntity(repositoryID string, summary *FileSummary) CodeEntity {
	entity := CodeEntity{
		RepositoryID:  repositoryID,
		Kind:          CodeEntityKindFile,
		Name:          summary.RelativePath,
		QualifiedName: summary.RelativePath,
		Language:      summary.Type,
		FilePath:      summary.RelativePath,
		StartLine:     1,
		EndLine:       summary.LineCount,
		Provenance:    summaryProvenance(summary),
	}
	entity.ID = StableCodeEntityID(entity)
	return entity
}

func symbolCodeEntity(repositoryID string, symbol Symbol, summary *FileSummary) CodeEntity {
	qualified := qualifiedSymbolName(symbol)
	entity := CodeEntity{
		RepositoryID:  repositoryID,
		Kind:          CodeEntityKindSymbol,
		SymbolKind:    symbol.Kind,
		Name:          displaySymbolName(symbol),
		QualifiedName: qualified,
		Language:      summary.Type,
		FilePath:      summary.RelativePath,
		StartLine:     symbol.StartLine,
		EndLine:       symbol.EndLine,
		Exported:      symbol.Exported,
		Package:       symbol.Package,
		Provenance:    summaryProvenance(summary),
	}
	entity.ID = StableCodeEntityID(entity)
	return entity
}

func summaryProvenance(summary *FileSummary) string {
	if summary.Type == "go" && len(summary.Symbols) > 0 {
		return CodeProvenanceOKFAST
	}
	return CodeProvenanceOKFRegex
}

func repositoryID(repoRoot string) string {
	base := filepath.Base(repoRoot)
	if base != "." && base != string(filepath.Separator) && base != "" {
		return sanitizeCodeIdentityPart(base)
	}
	sum := sha1.Sum([]byte(repoRoot))
	return hex.EncodeToString(sum[:])[:12]
}

func sanitizeCodeIdentityPart(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "-")
	if s == "" {
		return "repo"
	}
	return s
}

func dedupeCodeEntities(entities []CodeEntity) []CodeEntity {
	seen := make(map[string]bool, len(entities))
	result := make([]CodeEntity, 0, len(entities))
	for _, entity := range entities {
		if entity.ID == "" {
			entity.ID = StableCodeEntityID(entity)
		}
		if seen[entity.ID] {
			continue
		}
		seen[entity.ID] = true
		result = append(result, entity)
	}
	return result
}

func dedupeCodeRelations(relations []CodeRelation) []CodeRelation {
	seen := make(map[string]bool, len(relations))
	result := make([]CodeRelation, 0, len(relations))
	for _, relation := range relations {
		key := strings.Join([]string{
			relation.Kind,
			relation.SourceID,
			relation.TargetID,
			relation.TargetRef,
			relation.FilePath,
			fmt.Sprintf("%d", relation.Line),
		}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, relation)
	}
	return result
}

func sortCodeEntities(entities []CodeEntity) {
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].ID < entities[j].ID
	})
}

func sortCodeRelations(relations []CodeRelation) {
	sort.Slice(relations, func(i, j int) bool {
		if relations[i].Kind != relations[j].Kind {
			return relations[i].Kind < relations[j].Kind
		}
		if relations[i].SourceID != relations[j].SourceID {
			return relations[i].SourceID < relations[j].SourceID
		}
		if relations[i].TargetID != relations[j].TargetID {
			return relations[i].TargetID < relations[j].TargetID
		}
		return relations[i].TargetRef < relations[j].TargetRef
	})
}

func createCodeRepositoryConcept(repoRoot string, summaries []*FileSummary, index CodeKnowledgeIndex) *okf.Concept {
	c := okf.NewConcept("code_repository", "Code Repository")
	c.FilePath = "project/code_repository.md"
	c.Resource = codeRepositoryResource
	c.Description = "Generated code knowledge dimension overview"
	c.Tags = []string{"code", "repository", "generated"}

	languageCounts := make(map[string]int)
	symbolCounts := make(map[string]int)
	for _, summary := range summaries {
		languageCounts[summary.Type]++
		for _, symbol := range summary.Symbols {
			symbolCounts[symbol.Kind]++
		}
	}

	var content strings.Builder
	fmt.Fprintf(&content, "## Code Repository\n\n")
	fmt.Fprintf(&content, "**Repository:** `%s`\n\n", filepath.Base(repoRoot))
	fmt.Fprintf(&content, "**Repository ID:** `%s`\n\n", index.RepositoryID)
	fmt.Fprintf(&content, "**Indexed Files:** %d\n\n", len(summaries))
	fmt.Fprintf(&content, "**Code Entities:** %d\n\n", len(index.Entities))
	fmt.Fprintf(&content, "**Code Relations:** %d\n\n", len(index.Relations))

	writeCountSection(&content, "Language Distribution", languageCounts)
	writeCountSection(&content, "Symbol Counts", symbolCounts)

	fmt.Fprintf(&content, "### Generated Artifacts\n\n")
	fmt.Fprintf(&content, "- `project/code_repository.md`\n")
	fmt.Fprintf(&content, "- `project/code_relations.md`\n")
	for _, summary := range summaries {
		fmt.Fprintf(&content, "- `%s.md`\n", summary.RelativePath)
	}
	c.Content = content.String()
	return c
}

func createCodeRelationIndexConcept(index CodeKnowledgeIndex) *okf.Concept {
	c := okf.NewConcept("code_relation_index", "Code Relation Index")
	c.FilePath = "project/code_relations.md"
	c.Resource = codeRelationsResource
	c.Description = "Generated code dimension relation index"
	c.Tags = []string{"code", "relations", "generated"}

	var content strings.Builder
	fmt.Fprintf(&content, "## Code Relation Index\n\n")
	fmt.Fprintf(&content, "**Total Relations:** %d\n\n", len(index.Relations))
	fmt.Fprintf(&content, "| Kind | Source | Target | Location | Provenance |\n")
	fmt.Fprintf(&content, "| --- | --- | --- | --- | --- |\n")
	for _, relation := range index.Relations {
		target := relation.TargetID
		if target == "" {
			target = relation.TargetRef
		}
		location := relation.FilePath
		if relation.Line > 0 {
			location = fmt.Sprintf("%s:%d", relation.FilePath, relation.Line)
		}
		fmt.Fprintf(
			&content,
			"| %s | `%s` | `%s` | `%s` | %s |\n",
			relation.Kind,
			relation.SourceID,
			target,
			location,
			relation.Provenance,
		)
	}
	c.Content = content.String()
	return c
}

func writeCountSection(content *strings.Builder, title string, counts map[string]int) {
	if len(counts) == 0 {
		return
	}
	fmt.Fprintf(content, "### %s\n\n", title)
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(content, "- **%s:** %d\n", key, counts[key])
	}
	fmt.Fprintf(content, "\n")
}

func isCodeGeneratedConcept(concept *okf.Concept) bool {
	if concept == nil {
		return false
	}
	return strings.HasPrefix(concept.Type, "code_") ||
		concept.Resource == codeRepositoryResource ||
		concept.Resource == codeRelationsResource
}

func isCodeDerivedConcept(concept *okf.Concept) bool {
	if concept == nil {
		return false
	}
	return concept.Resource == codeRepositoryResource ||
		concept.Resource == codeRelationsResource ||
		concept.Type == "code_repository" ||
		concept.Type == "code_relation_index" ||
		concept.Type == "code_context_view"
}
