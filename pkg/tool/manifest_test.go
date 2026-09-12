package tool

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/superops-team/okf/pkg/manifest"
)

// S17/S20/S25: Service.Manifest returns a metadata-only, ordered listing and
// fails closed on duplicate ids.
func TestServiceManifest(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "beta.md"), `---
type: concept
title: Beta
tags: [x]
---
Beta body that must never be returned.
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "alpha.md"), `---
type: source
title: Alpha
okf_id: okf_17a2c56db85c4889b4f8fe02ca9ac67e
status: stable
---
Alpha body.
`)

	svc := NewService(Config{RepoPath: repo})
	env := svc.Manifest(context.Background(), ManifestRequest{})
	if !env.OK {
		t.Fatalf("manifest not ok: %#v", env.Error)
	}
	if env.Operation != OperationManifest {
		t.Fatalf("operation = %q", env.Operation)
	}
	result, ok := env.Result.(*manifest.ManifestResult)
	if !ok {
		t.Fatalf("result type = %T", env.Result)
	}
	if result.Limit != defaultManifestLimit {
		t.Fatalf("limit = %d, want default %d", result.Limit, defaultManifestLimit)
	}
	if result.Total != 2 || len(result.Items) != 2 {
		t.Fatalf("total/items = %d/%d", result.Total, len(result.Items))
	}
	// Ordered path ASC.
	if result.Items[0].Path != "concepts/alpha.md" || result.Items[1].Path != "concepts/beta.md" {
		t.Fatalf("order = %q, %q", result.Items[0].Path, result.Items[1].Path)
	}
	alpha := result.Items[0]
	if alpha.OKFID != "okf_17a2c56db85c4889b4f8fe02ca9ac67e" || alpha.Ref != "okf://concept/okf_17a2c56db85c4889b4f8fe02ca9ac67e" {
		t.Fatalf("identity/ref = %q/%q", alpha.OKFID, alpha.Ref)
	}
	if alpha.IdentityState != "stable" {
		t.Fatalf("identity state = %q", alpha.IdentityState)
	}
	if beta := result.Items[1]; beta.IdentityState != "legacy-unstable" {
		t.Fatalf("beta identity state = %q", beta.IdentityState)
	}
	// No Markdown body anywhere.
	for _, item := range result.Items {
		if item.Description != "" {
			t.Fatalf("description leaked: %q", item.Description)
		}
	}
	if result.IndexStatus != manifest.IndexMissing {
		t.Fatalf("index status = %q", result.IndexStatus)
	}
}

func TestServiceManifestPaginationValidation(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "c.md"), "---\ntype: concept\ntitle: C\n---\n")
	svc := NewService(Config{RepoPath: repo})

	zero := 0
	env := svc.Manifest(context.Background(), ManifestRequest{Limit: &zero})
	if env.OK || env.Error == nil || env.Error.Code != ErrInvalidRequest {
		t.Fatalf("explicit limit 0 envelope = %#v", env)
	}
	neg := -1
	env = svc.Manifest(context.Background(), ManifestRequest{Offset: neg})
	if env.OK || env.Error == nil || env.Error.Code != ErrInvalidRequest {
		t.Fatalf("negative offset envelope = %#v", env)
	}
}

// S25: duplicate valid ids fail the entire request via the Service.
func TestServiceManifestDuplicateIDFails(t *testing.T) {
	repo := initToolTestRepo(t)
	id := "okf_17a2c56db85c4889b4f8fe02ca9ac67e"
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "a.md"), "---\ntype: concept\ntitle: A\nokf_id: "+id+"\n---\n")
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "b.md"), "---\ntype: concept\ntitle: B\nokf_id: "+id+"\n---\n")

	svc := NewService(Config{RepoPath: repo})
	env := svc.Manifest(context.Background(), ManifestRequest{})
	if env.OK {
		t.Fatal("duplicate id should fail")
	}
	if env.Error == nil || env.Error.Code != "duplicate_concept_id" {
		t.Fatalf("error = %#v, want duplicate_concept_id", env.Error)
	}
}

// Manifest output must round-trip through the existing okf.tool.v1 envelope.
func TestServiceManifestEnvelopeJSON(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "c.md"), "---\ntype: concept\ntitle: C\n---\n")
	svc := NewService(Config{RepoPath: repo})
	env := svc.Manifest(context.Background(), ManifestRequest{})
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("envelope not valid JSON: %v", err)
	}
	if decoded["schema_version"] != SchemaVersion {
		t.Fatalf("schema_version = %v", decoded["schema_version"])
	}
}

const defaultManifestLimit = 100
