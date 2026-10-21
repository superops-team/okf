package memorydefense

import "testing"

func BenchmarkScreen10k(b *testing.B) {
	// 10KB of normal text with one GitHub PAT.
	content := make([]byte, 0, 10240)
	for i := 0; i < 200; i++ {
		content = append(content, []byte("This is a normal knowledge entry about system design patterns and distributed systems. ")...)
	}
	content = append(content, []byte("The token is ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234 for the CI bot.")...)
	pol := Policy{Enabled: true, Action: "redact"}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _ = Screen(string(content), pol)
	}
}

func BenchmarkScreen10kBlock(b *testing.B) {
	content := make([]byte, 0, 10240)
	for i := 0; i < 200; i++ {
		content = append(content, []byte("Normal text about distributed systems. ")...)
	}
	content = append(content, []byte("AKIAIOSFODNN7EXAMPLE")...)
	pol := Policy{Enabled: true, Action: "block"}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _ = Screen(string(content), pol)
	}
}
