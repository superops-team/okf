//go:build !race

package eval

import "testing"

func skipPuregoUnderRace(t *testing.T) {
	t.Helper()
}
