package memorymeta

import (
	"fmt"
	"path"
	"strings"

	"github.com/superops-team/okf/pkg/okf"
)

// code_refs bounds (S16).
const (
	maxCodeRefPatterns     = 16
	maxCodeRefPatternBytes = 256
	maxCodeRefPathSegments = 32
	maxDoubleStarOps       = 2
	maxDoubleStarSpan      = 8 // each ** matches at most 8 path segments
)

// CodeRefs reads, normalizes, and validates code_refs from CustomFields.
// Returns normalized patterns and warnings for invalid entries.
// Invalid patterns are dropped from the result.
func CodeRefs(c *okf.Concept) ([]string, []string) {
	if c == nil || c.CustomFields == nil {
		return nil, nil
	}
	raw, ok := c.CustomFields["code_refs"]
	if !ok {
		return nil, nil
	}

	var patterns []string
	var warns []string

	switch v := raw.(type) {
	case []any:
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				warns = append(warns, fmt.Sprintf("code_refs[%d] is not a string: %T", i, item))
				continue
			}
			norm, err := normalizeCodeRef(s)
			if err != nil {
				warns = append(warns, fmt.Sprintf("code_refs[%d]: %v", i, err))
				continue
			}
			patterns = append(patterns, norm)
		}
	case []string:
		for i, s := range v {
			norm, err := normalizeCodeRef(s)
			if err != nil {
				warns = append(warns, fmt.Sprintf("code_refs[%d]: %v", i, err))
				continue
			}
			patterns = append(patterns, norm)
		}
	default:
		warns = append(warns, fmt.Sprintf("code_refs is not a list: %T", raw))
		return nil, warns
	}

	// Bounds check
	if len(patterns) > maxCodeRefPatterns {
		warns = append(warns, fmt.Sprintf("code_refs has %d patterns, exceeds limit %d", len(patterns), maxCodeRefPatterns))
		patterns = patterns[:maxCodeRefPatterns]
	}

	return patterns, warns
}

// SetCodeRefs writes code_refs to CustomFields. Initializes map if nil.
func SetCodeRefs(c *okf.Concept, refs []string) {
	if c == nil {
		return
	}
	if c.CustomFields == nil {
		c.CustomFields = make(map[string]any)
	}
	// Convert to []any for YAML compatibility
	anyRefs := make([]any, len(refs))
	for i, r := range refs {
		anyRefs[i] = r
	}
	c.CustomFields["code_refs"] = anyRefs
}

// normalizeCodeRef normalizes a code_ref pattern: trims, removes ./ prefix,
// converts backslashes to forward slashes (then rejects if original had backslash),
// rejects absolute paths and .. traversal.
func normalizeCodeRef(pattern string) (string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "", fmt.Errorf("empty pattern")
	}
	// Reject backslash (ambiguity; forward slash canonical)
	if strings.Contains(pattern, "\\") {
		return "", fmt.Errorf("pattern contains backslash (use forward slash): %q", pattern)
	}
	// Reject NUL
	if strings.ContainsRune(pattern, 0) {
		return "", fmt.Errorf("pattern contains NUL byte")
	}
	// Reject absolute paths
	if strings.HasPrefix(pattern, "/") {
		return "", fmt.Errorf("absolute path not allowed: %q", pattern)
	}
	// Normalize ./ prefix
	pattern = strings.TrimPrefix(pattern, "./")
	// Clean path (resolves . and ..)
	cleaned := path.Clean(pattern)
	// Reject if cleaning reveals .. traversal escaping root
	if strings.HasPrefix(cleaned, "..") {
		return "", fmt.Errorf("path escapes repo root via ..: %q", pattern)
	}
	// Segment count check
	segments := strings.Split(cleaned, "/")
	if len(segments) > maxCodeRefPathSegments {
		return "", fmt.Errorf("pattern has %d segments, exceeds limit %d", len(segments), maxCodeRefPathSegments)
	}
	// ** operator count
	doubleStarCount := strings.Count(cleaned, "**")
	if doubleStarCount > maxDoubleStarOps {
		return "", fmt.Errorf("pattern has %d ** operators, exceeds limit %d", doubleStarCount, maxDoubleStarOps)
	}
	// Pattern length check
	if len(cleaned) > maxCodeRefPatternBytes {
		return "", fmt.Errorf("pattern is %d bytes, exceeds limit %d", len(cleaned), maxCodeRefPatternBytes)
	}
	return cleaned, nil
}

// MatchCodeRefs tests if inputPath matches any pattern.
// Returns the matched pattern and true if matched, empty string and false otherwise.
// Matching is lexical only — no filesystem access.
func MatchCodeRefs(inputPath string, patterns []string) (string, bool) {
	// Normalize input path
	inputPath = strings.TrimSpace(inputPath)
	inputPath = strings.TrimPrefix(inputPath, "./")
	inputPath = path.Clean(inputPath)
	// Reject invalid input
	if strings.HasPrefix(inputPath, "..") || strings.HasPrefix(inputPath, "/") {
		return "", false
	}
	inputSegments := strings.Split(inputPath, "/")

	for _, pattern := range patterns {
		if matchPattern(inputSegments, pattern) {
			return pattern, true
		}
	}
	return "", false
}

// matchPattern matches input segments against a glob pattern.
// Supports * (single segment), ** (recursive, max 8 segments), ? (single char), [abc] (char class).
func matchPattern(inputSegments []string, pattern string) bool {
	patternSegments := strings.Split(pattern, "/")
	return matchSegments(inputSegments, patternSegments, 0, 0)
}

// matchSegments recursively matches input segments against pattern segments.
func matchSegments(input []string, pattern []string, iIdx, pIdx int) bool {
	// Both consumed = match
	if pIdx == len(pattern) {
		return iIdx == len(input)
	}

	seg := pattern[pIdx]

	if seg == "**" {
		// ** matches 0 to maxDoubleStarSpan segments
		// Try matching remaining pattern against input[iIdx+j:] for j in 0..maxDoubleStarSpan
		for j := 0; j <= maxDoubleStarSpan && iIdx+j <= len(input); j++ {
			if matchSegments(input, pattern, iIdx+j, pIdx+1) {
				return true
			}
		}
		return false
	}

	// Need an input segment to match
	if iIdx == len(input) {
		return false
	}

	// Single-segment glob match
	if matchSingleSegment(input[iIdx], seg) {
		return matchSegments(input, pattern, iIdx+1, pIdx+1)
	}
	return false
}

// matchSingleSegment matches a single input segment against a single-segment glob pattern.
func matchSingleSegment(input, pattern string) bool {
	// Fast path: no glob chars
	if !strings.ContainsAny(pattern, "*?[") {
		return input == pattern
	}
	return globMatch(input, pattern)
}

// globMatch implements single-segment glob matching with *, ?, [].
func globMatch(input, pattern string) bool {
	return globMatchHelper(input, pattern, 0, 0)
}

func globMatchHelper(input, pattern string, iIdx, pIdx int) bool {
	for pIdx < len(pattern) {
		c := pattern[pIdx]
		switch c {
		case '*':
			// * matches any sequence (including empty) within a segment
			if pIdx+1 == len(pattern) {
				return true // trailing * matches rest
			}
			// Try all positions
			for j := iIdx; j <= len(input); j++ {
				if globMatchHelper(input, pattern, j, pIdx+1) {
					return true
				}
			}
			return false
		case '?':
			if iIdx == len(input) {
				return false
			}
			iIdx++
			pIdx++
		case '[':
			if iIdx == len(input) {
				return false
			}
			// Find closing ]
			end := strings.IndexByte(pattern[pIdx+1:], ']')
			if end < 0 {
				// No closing ], treat [ as literal
				if input[iIdx] != '[' {
					return false
				}
				iIdx++
				pIdx++
				continue
			}
			charClass := pattern[pIdx+1 : pIdx+1+end]
			if !matchCharClass(input[iIdx], charClass) {
				return false
			}
			iIdx++
			pIdx += end + 2
		default:
			if iIdx == len(input) || input[iIdx] != c {
				return false
			}
			iIdx++
			pIdx++
		}
	}
	return iIdx == len(input)
}

// matchCharClass checks if c matches a glob char class like abc or a-z.
func matchCharClass(c byte, class string) bool {
	negate := false
	if strings.HasPrefix(class, "!") || strings.HasPrefix(class, "^") {
		negate = true
		class = class[1:]
	}
	matched := false
	for i := 0; i < len(class); i++ {
		if i+2 < len(class) && class[i+1] == '-' {
			// Range a-z
			if c >= class[i] && c <= class[i+2] {
				matched = true
			}
			i += 2
		} else if class[i] == c {
			matched = true
		}
	}
	if negate {
		return !matched
	}
	return matched
}
