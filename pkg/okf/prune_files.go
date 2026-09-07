package okf

// Generated-file removal for knowledge bases (single source of truth).
//
// Both the git incremental-update path (SaveKnowledgeBase) and okf sync -prune
// delete generated knowledge files through RemoveGeneratedKnowledgeFiles. The
// deletion is metadata-driven, never name-driven:
//   - the target file must carry trusted generated metadata (generator
//     "okf.git" + generated marker + matching source_path), so hand-written
//     concepts are never deleted;
//   - derived chunk files (<base>__c<digits>.md) are removed together with the
//     target only when their frontmatter carries source_path == resource AND
//     derived == true — a chunk whose derived marker was edited away survives.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/superops-team/okf/pkg/parser"
)

// RemoveGeneratedKnowledgeFiles deletes the knowledge file for resource (with
// trusted generated metadata) and any derived chunk files, under knowledgeDir.
// It is a no-op when the file is not trusted-generated (author-owned content).
func RemoveGeneratedKnowledgeFiles(knowledgeDir, resource string) {
	if resource == "" {
		return
	}
	path := filepath.Join(knowledgeDir, resource)
	if !strings.HasSuffix(path, ".md") {
		path += ".md"
	}
	concept, err := parser.ParseConcept(path)
	if err != nil {
		return
	}
	// Check both legacy boolean "generated" in CustomFields and v0.2 Generated struct.
	trusted := hasTrustedGeneratedMetadata(concept.CustomFields, resource)
	if !trusted && concept.Generated != nil {
		if gen, ok := concept.CustomFields["generator"].(string); ok && gen == "okf.git" {
			if sp, ok := concept.CustomFields["source_path"].(string); ok && (sp == resource || codeFileResourceKey(sp) == resource) {
				trusted = true
			}
		}
	}
	if !trusted {
		return
	}
	_ = os.Remove(path)
	removeDerivedChunks(knowledgeDir, resource)
}

// removeDerivedChunks removes chunk concepts derived from resource: files
// named <resource>__c<digits>.md in the same directory whose frontmatter
// carries source_path == resource AND derived == true. The lifecycle is
// metadata-driven, not name-driven.
func removeDerivedChunks(knowledgeDir, resource string) {
	// resource may be "big.txt" (git lifecycle) or "big.txt.md" (document
	// lifecycle); chunk files are always named <base>__c<digits>.md where base
	// is the source file WITHOUT the .md extension (see cmd_add chunking).
	base := strings.TrimSuffix(filepath.Base(resource), ".md")
	dir := filepath.Join(knowledgeDir, filepath.Dir(resource))
	prefix := base + "__c"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".md") {
			continue
		}
		digits := name[len(prefix) : len(name)-len(".md")]
		if digits == "" {
			continue
		}
		allDigits := true
		for _, r := range digits {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if !allDigits {
			continue
		}
		p := filepath.Join(dir, name)
		concept, perr := parser.ParseConcept(p)
		if perr != nil {
			continue
		}
		if sp, _ := concept.CustomFields["source_path"].(string); sp != resource && sp+".md" != resource {
			continue
		}
		if fmt.Sprint(concept.CustomFields["derived"]) != "true" {
			continue
		}
		_ = os.Remove(p)
	}
}

// generatorGit and generatorDocument are the two trusted generators whose
// products may be removed by the generated-file lifecycle. Anything else
// (hand-written concepts) is author-owned and never deleted.
const (
	generatorGit      = "okf.git"
	generatorDocument = "okf.document"
)

// hasTrustedGeneratedMetadata reports whether fields carry a trusted okf
// generation marker bound to sourcePath.
func hasTrustedGeneratedMetadata(fields map[string]interface{}, sourcePath string) bool {
	if fields == nil {
		return false
	}
	// Accept both legacy boolean form (generated: true) and v0.2 mapping form (generated: {by: ...}).
	generatedOk := false
	switch v := fields["generated"].(type) {
	case bool:
		generatedOk = v
	case map[string]interface{}:
		generatedOk = v != nil
	default:
		generatedOk = fields["generated"] != nil
	}
	if !generatedOk {
		return false
	}
	generator, ok := fields["generator"].(string)
	if !ok || (generator != generatorGit && generator != generatorDocument) {
		return false
	}
	metadataSourcePath, ok := fields["source_path"].(string)
	if !ok || metadataSourcePath == "" {
		return false
	}
	return metadataSourcePath == sourcePath ||
		metadataSourcePath+".md" == sourcePath || // document products: source_path is the bare source file
		codeFileResourceKey(metadataSourcePath) == sourcePath
}

// codeFileResourceKey mirrors pkg/git.codeFileResource for source-path matching
// (kept local to avoid an import cycle: pkg/git already imports pkg/okf).
func codeFileResourceKey(path string) string {
	p := strings.TrimPrefix(path, "./")
	return "code://repo/" + p
}
