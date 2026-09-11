//go:build !race

package main

import "testing"

func skipPuregoUnderRace(t *testing.T) {
	t.Helper()
}
