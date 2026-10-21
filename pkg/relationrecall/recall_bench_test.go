package relationrecall

import (
	"fmt"
	"testing"

	"github.com/superops-team/okf/pkg/memorymeta"
	"github.com/superops-team/okf/pkg/okf"
)

func BenchmarkRecall10k(b *testing.B) {
	// Build 10k concepts: each concept has an extends relation to the previous one.
	n := 10000
	concepts := make([]*okf.Concept, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("okf_%032d", i)
		fields := map[string]any{"memory_state": "approved"}
		if i > 0 {
			fields["memory_relation"] = map[string]any{
				"kind":    "extends",
				"targets": []any{fmt.Sprintf("okf_%032d", i-1)},
			}
		}
		concepts[i] = &okf.Concept{Type: "note", Title: id, CustomFields: map[string]any{"okf_id": id}}
		for k, v := range fields {
			concepts[i].CustomFields[k] = v
		}
	}
	view := memorymeta.BuildTemporalView(concepts)
	anchor := fmt.Sprintf("okf_%032d", n-1)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := Recall(anchor, view)
		if err != nil {
			b.Fatal(err)
		}
	}
}
