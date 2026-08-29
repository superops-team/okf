package convert

import (
	"strings"
	"testing"
	"testing/quick"
	"unicode/utf8"

	"github.com/superops-team/okf/pkg/parser"
)

// Property-based test for the document-import core contract: whatever title a
// document carries, the frontmatter emitted by WrapConcept must round-trip
// through the OKF parser unchanged. This catches %q-vs-YAML escaping mismatches
// (quotes, newlines, tabs) that example-based tests miss.
func TestPropertyWrapConceptRoundTrip(t *testing.T) {
	prop := func(title, body string) bool {
		// Skip inputs outside realistic document titles (NUL / invalid UTF-8).
		if strings.ContainsRune(title, '\x00') || !utf8.ValidString(title) {
			return true
		}
		doc := WrapConcept(title, "p.pdf", "pdf", "source", body)
		c, err := parser.ParseConceptBytes("p.pdf.md", []byte(doc))
		if err != nil {
			return false
		}
		if title == "" {
			return true // empty titles are normalized by the parser
		}
		return c.Title == title
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}
