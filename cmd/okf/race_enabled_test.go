//go:build race

package main

import "testing"

func skipPuregoUnderRace(t *testing.T) {
	t.Helper()
	t.Skip("purego tokenizer is incompatible with the race runtime; this test runs in the non-race gauntlet layers")
}
