package okf

import "strings"

func sanitizeFilename(name string) string {
	result := name
	for _, r := range []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"} {
		result = strings.ReplaceAll(result, r, "_")
	}
	return result
}

func containsFold(s, substr string) bool {
	return indexFold(s, substr) >= 0
}

func indexFold(s, substr string) int {
	n := len(substr)
	for i := 0; i <= len(s)-n; i++ {
		if equalFold(s[i:i+n], substr) {
			return i
		}
	}
	return -1
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca == cb {
			continue
		}
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func containsString(slice []string, val string) bool {
	for _, s := range slice {
		if equalFold(s, val) {
			return true
		}
	}
	return false
}
