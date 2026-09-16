package memorymeta

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"github.com/superops-team/okf/pkg/identity"
	"github.com/superops-team/okf/pkg/lexical"
	"github.com/superops-team/okf/pkg/okf"
)

// memory_check classification statuses (spec S20-S23). Only two statuses exist;
// there is deliberately no possible_conflict (semantic conflict needs an LLM).
const (
	// MemoryStatusNoSimilar means no durable concept cleared the Jaccard threshold.
	MemoryStatusNoSimilar = "no_similar"
	// MemoryStatusPossibleDuplicate means at least one durable concept is near-duplicate.
	MemoryStatusPossibleDuplicate = "possible_duplicate"
)

// Tuning bounds (spec §5.3/§5.4, design.md).
const (
	// DefaultDuplicateThreshold is the pilot-validated Jaccard threshold.
	DefaultDuplicateThreshold = 0.20
	// maxDurableCandidates bounds the durable concept set per call (Service keeps no cache).
	maxDurableCandidates = 1000
	// bm25CandidateBudget is how many BM25 raw hits are re-ranked by Jaccard.
	bm25CandidateBudget = 10
	// maxReturnedCandidates caps the returned candidate list (top-3).
	maxReturnedCandidates = 3
	// candidateBodyChars truncates the body used for both BM25 and Jaccard.
	candidateBodyChars = 500
)

// defaultDurableTypes is the candidate source when no explicit typeFilter is
// given. Durable note/event/feedback concepts only; code_file concepts are
// excluded to avoid polluting duplicate detection.
var defaultDurableTypes = []string{"note", "event", "feedback"}

// MemoryCandidate is one near-duplicate durable concept (spec §5.7).
type MemoryCandidate struct {
	OKFID        string  `json:"okf_id"`
	Ref          string  `json:"ref"`
	JaccardScore float64 `json:"jaccard_score"`
}

// MemoryCheckResult is the dedicated envelope returned when memory_check=true.
// It replaces (does not mix with) the normal Query ranking output.
type MemoryCheckResult struct {
	Status         string            `json:"status"`
	Threshold      float64           `json:"threshold"`
	Candidates     []MemoryCandidate `json:"candidates"`
	CandidateTypes []string          `json:"candidate_types"`
	CandidateCount int               `json:"candidate_count"`
}

// CheckMemory runs a read-only, per-call duplicate check over the given durable
// concept set. It builds a fresh BM25 index (no Service cache), takes the top-10
// raw hits, and re-ranks them with normalized token Jaccard. It never mutates
// concepts.
//
//   - content:     the proposed knowledge text (also the BM25/Jaccard query).
//   - typeFilter: if non-empty, restricts candidates to that single type;
//     otherwise candidates default to durable types (note, event, feedback).
//   - project:     optional CustomFields["project"] equality filter.
//   - tag:         optional concept.Tags membership filter.
//   - dupThreshold: Jaccard threshold; <= 0 uses DefaultDuplicateThreshold.
func CheckMemory(concepts []*okf.Concept, content, typeFilter, project, tag string, dupThreshold float64) MemoryCheckResult {
	threshold := dupThreshold
	if threshold <= 0 {
		threshold = DefaultDuplicateThreshold
	}

	candidateTypes := defaultDurableTypes
	if tf := strings.TrimSpace(typeFilter); tf != "" {
		candidateTypes = []string{tf}
	}

	// Step 1: select the bounded durable candidate pool.
	pool := selectDurablePool(concepts, candidateTypes, project, tag)

	result := MemoryCheckResult{
		Status:         MemoryStatusNoSimilar,
		Threshold:      threshold,
		Candidates:     []MemoryCandidate{},
		CandidateTypes: slices.Clone(candidateTypes),
		CandidateCount: len(pool),
	}
	if len(pool) == 0 || strings.TrimSpace(content) == "" {
		return result
	}

	// Step 2: pre-compute per-candidate text, key, identity (avoids recomputing
	// for top-10 Jaccard re-ranking and double FromConcept calls).
	entries := make([]candidateEntry, len(pool))
	bm25 := lexical.NewBM25()
	for i, c := range pool {
		ident := identity.FromConcept(c)
		entries[i] = candidateEntry{
			concept: c,
			text:    candidateText(c),
			key:     keyFromIdentity(ident, c.FilePath),
			ident:   ident,
		}
		tf := make(map[string]int, 32)
		docLen := tokenizeBM25Freq(entries[i].text, tf)
		bm25.AddFromFreq(entries[i].key, tf, docLen)
	}
	bm25.Finalize()

	// Step 3: BM25 raw top-10 (raw scores are not [0,1]; used only to shortlist).
	hits := bm25.Search(content, bm25CandidateBudget)
	if len(hits) == 0 {
		return result
	}

	queryTokens := jaccardTokens(content)

	// Step 4: re-rank candidates with normalized token Jaccard, keep threshold passers.
	var scored []MemoryCandidate
	for _, h := range hits {
		entry := entryByKey(entries, h.Key)
		if entry == nil {
			continue
		}
		j := jaccardSimilarity(queryTokens, jaccardTokens(entry.text))
		if j < threshold {
			continue
		}
		ref := entry.concept.FilePath
		if ref == "" {
			ref = entry.ident.URI
		}
		scored = append(scored, MemoryCandidate{
			OKFID:        entry.ident.ID,
			Ref:          ref,
			JaccardScore: j,
		})
	}

	if len(scored) == 0 {
		return result
	}

	// Step 5: deterministic tie-break: Jaccard desc, then okf_id asc. Empty okf_id
	// falls back to Ref so the order stays fully deterministic.
	slices.SortFunc(scored, func(a, b MemoryCandidate) int {
		if a.JaccardScore != b.JaccardScore {
			// higher score first => reverse comparison
			return cmp.Compare(b.JaccardScore, a.JaccardScore)
		}
		if a.OKFID != b.OKFID {
			return cmp.Compare(a.OKFID, b.OKFID)
		}
		return cmp.Compare(a.Ref, b.Ref)
	})
	if len(scored) > maxReturnedCandidates {
		scored = scored[:maxReturnedCandidates]
	}

	result.Status = MemoryStatusPossibleDuplicate
	result.Candidates = scored
	return result
}

// candidateEntry holds per-candidate pre-computed values to avoid recomputation
// during BM25 build and Jaccard re-ranking.
type candidateEntry struct {
	concept *okf.Concept
	text    string
	key     string
	ident   identity.Ref
}

// entryByKey finds the entry whose key matches, using a linear scan (top-10
// hits against ≤1000 entries is cheaper than a map allocation per call).
func entryByKey(entries []candidateEntry, key string) *candidateEntry {
	for i := range entries {
		if entries[i].key == key {
			return &entries[i]
		}
	}
	return nil
}

// selectDurablePool filters concepts down to the bounded durable candidate set.
func selectDurablePool(concepts []*okf.Concept, candidateTypes []string, project, tag string) []*okf.Concept {
	pool := make([]*okf.Concept, 0, len(concepts))
	for _, c := range concepts {
		if c == nil {
			continue
		}
		if !slices.Contains(candidateTypes, c.Type) {
			continue
		}
		if project != "" && conceptProject(c) != project {
			continue
		}
		if tag != "" && !slices.Contains(c.Tags, tag) {
			continue
		}
		pool = append(pool, c)
		if len(pool) >= maxDurableCandidates {
			break
		}
	}
	return pool
}

func conceptProject(c *okf.Concept) string {
	if c.CustomFields == nil {
		return ""
	}
	v, _ := c.CustomFields["project"].(string)
	return v
}

// keyFromIdentity derives a stable BM25 key from a pre-computed identity.Ref,
// avoiding a second identity.FromConcept call. Falls back to file path when the
// concept has no stable ID.
func keyFromIdentity(ident identity.Ref, filePath string) string {
	if ident.ID != "" {
		return ident.ID
	}
	return "file:" + filePath
}

// candidateText returns title + first candidateBodyChars runes of body, used
// for both BM25 indexing and Jaccard re-ranking (same text, different tokenizer).
func candidateText(c *okf.Concept) string {
	return c.Title + " " + firstNRunes(c.Content, candidateBodyChars)
}

// firstNRunes returns the first n runes of s without allocating a []rune slice.
// It finds the byte offset after n runes by ranging over s (which decodes runes
// without allocation) and slicing the original string.
func firstNRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count >= n {
			return s[:i]
		}
		count++
	}
	return s
}

// tokenizeBM25Freq streams BM25-compatible tokens into tf and returns the
// document length (total token count including duplicates).
//
// It mirrors lexical.Tokenize exactly: latin whole-token lowercasing,
// splitIdentifier subword expansion (camelCase / snake_case / kebab-case /
// acronym boundaries), and CJK overlapping bigrams. The streaming form avoids
// the []string intermediate and strings.FieldsFunc allocation that lexical
// uses, while producing identical frequency maps and document lengths (verified
// by TestTokenizeBM25FreqParity).
func tokenizeBM25Freq(text string, tf map[string]int) int {
	docLen := 0
	var latin []rune     // lowercased runes (for whole token)
	var latinOrig []rune // original-case runes (for subword boundary detection)
	var cjk []rune

	flushLatin := func() {
		if len(latin) == 0 {
			return
		}
		whole := string(latin) // latin is already lowercased
		tf[whole]++
		docLen++
		docLen += addSubwords(latinOrig, whole, tf)
		latin = latin[:0]
		latinOrig = latinOrig[:0]
	}
	flushCJK := func() {
		switch {
		case len(cjk) == 0:
		case len(cjk) == 1:
			tf[string(cjk)]++
			docLen++
		default:
			for i := 0; i+1 < len(cjk); i++ {
				tf[string(cjk[i:i+2])]++
				docLen++
			}
		}
		cjk = cjk[:0]
	}

	for _, r := range text {
		switch {
		case isCJKRune(r):
			flushLatin()
			cjk = append(cjk, r)
		case isWordRuneBM25(r):
			flushCJK()
			latin = append(latin, unicode.ToLower(r))
			latinOrig = append(latinOrig, r)
		default:
			flushLatin()
			flushCJK()
		}
	}
	flushLatin()
	flushCJK()
	return docLen
}

// addSubwords splits original-case runes into identifier subwords (mirrors
// lexical.splitIdentifier) and adds each lowercased subword that differs from
// whole. Returns the number of subwords added (for docLen accounting).
func addSubwords(original []rune, whole string, tf map[string]int) int {
	if len(original) == 0 {
		return 0
	}
	added := 0
	// Split by delimiters _ - . (same as lexical.splitIdentifier FieldsFunc).
	segStart := 0
	for segEnd := 0; segEnd <= len(original); segEnd++ {
		if segEnd < len(original) && original[segEnd] != '_' && original[segEnd] != '-' && original[segEnd] != '.' {
			continue
		}
		// Process segment [segStart, segEnd).
		if segEnd > segStart {
			seg := original[segStart:segEnd]
			added += addCamelSubwords(seg, whole, tf)
		}
		segStart = segEnd + 1
	}
	return added
}

// addCamelSubwords detects camelCase and acronym boundaries in a delimiter-free
// segment and adds each lowercased subword that differs from whole. Mirrors
// lexical.splitIdentifier's inner loop.
func addCamelSubwords(seg []rune, whole string, tf map[string]int) int {
	if len(seg) == 0 {
		return 0
	}
	added := 0
	start := 0
	for i := 1; i < len(seg); i++ {
		prev, cur := seg[i-1], seg[i]
		lowerToUpper := (unicode.IsLower(prev) || unicode.IsDigit(prev)) && unicode.IsUpper(cur)
		acronymEnd := i+1 < len(seg) && unicode.IsUpper(prev) && unicode.IsUpper(cur) && unicode.IsLower(seg[i+1])
		if lowerToUpper || acronymEnd {
			if sub := lowerRunes(seg[start:i]); sub != "" && sub != whole {
				tf[sub]++
				added++
			}
			start = i
		}
	}
	if sub := lowerRunes(seg[start:]); sub != "" && sub != whole {
		tf[sub]++
		added++
	}
	return added
}

// lowerRunes lowercases a rune slice and returns it as a string (no intermediate
// []rune allocation beyond the input slice).
func lowerRunes(rs []rune) string {
	for i := range rs {
		rs[i] = unicode.ToLower(rs[i])
	}
	return string(rs)
}

// isCJKRune reports whether r is a CJK ideograph (same range as lexical.isCJK).
func isCJKRune(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0x3040 && r <= 0x30FF) || // Hiragana + Katakana
		(r >= 0xAC00 && r <= 0xD7AF) // Hangul Syllables
}

// isWordRuneBM25 reports whether r participates in a latin word token
// (letters, digits, underscore — exactly lexical.isWordRune). Hyphen is a
// delimiter, not a word character, matching lexical.Tokenize behavior.
func isWordRuneBM25(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// jaccardTokenRune reports whether r participates in a continuous word token
// (letters, digits, underscore, hyphen). Han is handled separately as a per-char
// token, so it is excluded here.
func jaccardTokenRune(r rune) bool {
	if unicode.Is(unicode.Han, r) {
		return false
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-'
}

// jaccardTokens implements the spec tokenization for normalized Jaccard:
//   - Unicode lowercase all runes (in-place on the rune buffer, no intermediate string);
//   - continuous letters/digits/_/- are one token;
//   - each Han character is its own token;
//   - everything else is a delimiter.
func jaccardTokens(s string) []string {
	var tokens []string
	var buf []rune
	flush := func() {
		if len(buf) > 0 {
			tokens = append(tokens, string(buf))
			buf = buf[:0]
		}
	}
	for _, r := range s {
		r = unicode.ToLower(r)
		switch {
		case unicode.Is(unicode.Han, r):
			flush()
			tokens = append(tokens, string(r))
		case jaccardTokenRune(r):
			buf = append(buf, r)
		default:
			flush()
		}
	}
	flush()
	return tokens
}

// jaccardSimilarity computes set Jaccard = |A∩B|/|A∪B| over the token slices;
// an empty side yields 0.
func jaccardSimilarity(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setA := make(map[string]struct{}, len(a))
	for _, t := range a {
		setA[t] = struct{}{}
	}
	setB := make(map[string]struct{}, len(b))
	for _, t := range b {
		setB[t] = struct{}{}
	}
	small, large := setA, setB
	if len(setA) > len(setB) {
		small, large = setB, setA
	}
	inter := 0
	for t := range small {
		if _, ok := large[t]; ok {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}
