package convert

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// goldenFiles pins the exact conversion output for every supported format to
// downmark v0.10.0. Regenerate ONLY after an intentional downmark upgrade:
//
//	UPDATE_GOLDEN=1 go test ./pkg/convert -run TestConvertGolden
//
// then review the diff manually. See testdata/golden/README.md.
var goldenFiles = []string{
	"sample.pdf", "sample.docx", "sample.xlsx",
	"sample.pptx", "sample.html", "sample.csv", "sample.txt",
}

func TestConvertGolden(t *testing.T) {
	for _, f := range goldenFiles {
		t.Run(f, func(t *testing.T) {
			r, err := ConvertToMarkdown(context.Background(), testdata(f), nil)
			if err != nil {
				t.Fatalf("ConvertToMarkdown(%s): %v", f, err)
			}
			golden := filepath.Join("testdata", "golden", f+".golden")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(r.Markdown), 0o644); err != nil {
					t.Fatal(err)
				}
				t.Logf("wrote %s", golden)
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden %s: %v (run UPDATE_GOLDEN=1 to generate)", golden, err)
			}
			if string(want) != r.Markdown {
				t.Errorf("conversion output for %s drifted from golden (downmark v0.10.0 pinned); run UPDATE_GOLDEN=1 and review diff", f)
			}
		})
	}
}
