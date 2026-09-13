package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/superops-team/okf/pkg/okf"
)

func testLoader() (*okf.KnowledgeBundle, error) {
	return makeTestBundle(), nil
}

func newTestHandler() *APIHandler {
	return NewAPIHandler("/test/bundle", testLoader)
}

func newTestServer() *httptest.Server {
	h := newTestHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, "/api/v1")
	return httptest.NewServer(mux)
}

func TestHandleGraph_Get(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/graph")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("expected content-type application/json, got '%s'", ct)
	}

	var graph GraphData
	if err := json.NewDecoder(resp.Body).Decode(&graph); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(graph.Nodes) != 4 {
		t.Errorf("expected 4 nodes, got %d", len(graph.Nodes))
	}
	if graph.Stats.TotalConcepts != 4 {
		t.Errorf("expected 4 total concepts, got %d", graph.Stats.TotalConcepts)
	}
}

func TestHandleGraph_MethodNotAllowed(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/graph", "application/json", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", resp.StatusCode)
	}
}

func TestHandleConceptDetail_Found(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/concepts/okf_parent123")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var detail ConceptDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if detail.Title != "Parent Concept" {
		t.Errorf("expected title 'Parent Concept', got '%s'", detail.Title)
	}
	if detail.Content != "Parent content here" {
		t.Errorf("expected content, got '%s'", detail.Content)
	}
}

func TestHandleConceptDetail_NotFound(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/concepts/nonexistent")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}

	var errResp apiError
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Code != "not_found" {
		t.Errorf("expected error code 'not_found', got '%s'", errResp.Code)
	}
}

func TestHandleConceptDetail_MissingID(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/concepts/")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestHandleSearch_ByQuery(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/search?q=parent")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result struct {
		Query   string         `json:"query"`
		Count   int            `json:"count"`
		Results []searchResult `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.Count < 1 {
		t.Errorf("expected at least 1 result for 'parent', got %d", result.Count)
	}
	if result.Results[0].MatchScore < 50 {
		t.Errorf("expected match score >= 50 for title match, got %d", result.Results[0].MatchScore)
	}
}

func TestHandleSearch_ByType(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/search?type=Process")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result struct {
		Count   int            `json:"count"`
		Results []searchResult `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.Count != 1 {
		t.Errorf("expected 1 Process result, got %d", result.Count)
	}
	if result.Results[0].Type != "Process" {
		t.Errorf("expected type 'Process', got '%s'", result.Results[0].Type)
	}
}

func TestHandleSearch_ByTag(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/search?tag=tag1")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.Count != 2 {
		t.Errorf("expected 2 results for tag1, got %d", result.Count)
	}
}

func TestHandleSearch_MissingParams(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/search")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestHandleSearch_NoResults(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/search?q=zzzznonexistent")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.Count != 0 {
		t.Errorf("expected 0 results, got %d", result.Count)
	}
}

func TestHandleHealth(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result struct {
		Status   string `json:"status"`
		Concepts int    `json:"concepts"`
		Bundle   string `json:"bundle"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("expected status 'ok', got '%s'", result.Status)
	}
	if result.Concepts != 4 {
		t.Errorf("expected 4 concepts, got %d", result.Concepts)
	}
	if result.Bundle != "/test/bundle" {
		t.Errorf("expected bundle '/test/bundle', got '%s'", result.Bundle)
	}
}

func TestRegisterRoutes_Prefix(t *testing.T) {
	h := newTestHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, "/api/v1")

	// Verify all expected routes are registered by making requests.
	srv := httptest.NewServer(mux)
	defer srv.Close()

	routes := []string{
		"/api/v1/graph",
		"/api/v1/concepts/okf_parent123",
		"/api/v1/search?q=test",
		"/api/v1/health",
	}
	for _, route := range routes {
		resp, err := http.Get(srv.URL + route)
		if err != nil {
			t.Errorf("route %s request failed: %v", route, err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			t.Errorf("route %s returned 404 (not registered)", route)
		}
	}
}

func TestWriteJSON_Headers(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusOK, map[string]string{"key": "value"})

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("expected content-type application/json, got '%s'", ct)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("expected cache-control no-store, got '%s'", cc)
	}
}

func TestWriteError_Structured(t *testing.T) {
	rr := httptest.NewRecorder()
	writeError(rr, http.StatusBadRequest, "test_code", "test message")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}

	var errResp apiError
	if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if errResp.Code != "test_code" {
		t.Errorf("expected code 'test_code', got '%s'", errResp.Code)
	}
	if errResp.Error != "test message" {
		t.Errorf("expected error 'test message', got '%s'", errResp.Error)
	}
}
