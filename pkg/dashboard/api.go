package dashboard

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/superops-team/okf/pkg/okf"
)

// APIHandler holds the dependencies for dashboard API endpoints.
// It is safe for concurrent use once created.
type APIHandler struct {
	bundleLoader func() (*okf.KnowledgeBundle, error)
	// bundlePath is used for logging and error messages.
	bundlePath string
}

// NewAPIHandler creates an API handler with the given bundle loader.
// The loader is called on each request so the dashboard always reflects
// the current on-disk state (cheap for typical bundle sizes).
func NewAPIHandler(bundlePath string, loader func() (*okf.KnowledgeBundle, error)) *APIHandler {
	return &APIHandler{bundleLoader: loader, bundlePath: bundlePath}
}

// RegisterRoutes mounts all API endpoints on the given mux.
// Prefix is typically "/api/v1".
func (h *APIHandler) RegisterRoutes(mux *http.ServeMux, prefix string) {
	prefix = strings.TrimSuffix(prefix, "/")
	mux.HandleFunc(prefix+"/graph", h.handleGraph)
	mux.HandleFunc(prefix+"/concepts/", h.handleConceptDetail)
	mux.HandleFunc(prefix+"/search", h.handleSearch)
	mux.HandleFunc(prefix+"/health", h.handleHealth)
}

// apiError is the structured error response body.
type apiError struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// writeJSON serializes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("dashboard: json encode error: %v", err)
	}
}

// writeError writes a structured JSON error.
func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, apiError{Error: msg, Code: code})
}

// handleGraph returns the full graph data (nodes + edges + stats).
//
//	GET /api/v1/graph
func (h *APIHandler) handleGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
		return
	}
	bundle, err := h.bundleLoader()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bundle_load_failed",
			"failed to load knowledge bundle: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, BuildGraph(bundle))
}

// handleConceptDetail returns full detail for a single concept.
//
//	GET /api/v1/concepts/{id}
func (h *APIHandler) handleConceptDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/concepts/")
	id = strings.TrimSpace(id)
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "concept id is required")
		return
	}
	bundle, err := h.bundleLoader()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bundle_load_failed",
			"failed to load knowledge bundle: "+err.Error())
		return
	}
	detail := BuildConceptDetail(bundle, id)
	if detail == nil {
		writeError(w, http.StatusNotFound, "not_found", "concept not found: "+id)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// searchResult is a single search hit.
type searchResult struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	FilePath    string   `json:"filePath,omitempty"`
	Folder      string   `json:"folder,omitempty"`
	// MatchScore is a simple relevance score (0–100) for client-side sorting.
	MatchScore int `json:"matchScore"`
}

// handleSearch performs a simple substring search across concept titles,
// descriptions, tags, and file paths. Returns at most 50 results.
//
//	GET /api/v1/search?q=keyword&type=optional&tag=optional
func (h *APIHandler) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
		return
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	typeFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))
	tagFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("tag")))

	if q == "" && typeFilter == "" && tagFilter == "" {
		writeError(w, http.StatusBadRequest, "missing_query", "at least one of q, type, or tag is required")
		return
	}

	bundle, err := h.bundleLoader()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bundle_load_failed",
			"failed to load knowledge bundle: "+err.Error())
		return
	}

	results := make([]searchResult, 0, 50)
	for _, c := range bundle.Concepts {
		if c == nil {
			continue
		}
		// Type filter.
		if typeFilter != "" && strings.ToLower(c.Type) != typeFilter {
			continue
		}
		// Tag filter.
		if tagFilter != "" {
			found := false
			for _, t := range c.Tags {
				if strings.ToLower(t) == tagFilter {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		// Text search.
		score := 0
		if q != "" {
			score = matchScore(c, q)
			if score == 0 {
				continue
			}
		}
		node := conceptToNode(c)
		results = append(results, searchResult{
			ID:          node.ID,
			Title:       node.Title,
			Type:        node.Type,
			Description: node.Description,
			Tags:        node.Tags,
			FilePath:    node.FilePath,
			Folder:      node.Folder,
			MatchScore:  score,
		})
		if len(results) >= 50 {
			break
		}
	}

	// Sort by score descending.
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].MatchScore > results[i].MatchScore {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":   q,
		"count":   len(results),
		"results": results,
	})
}

// matchScore computes a simple relevance score for a concept against a query.
// Title match = 50, tag match = 30, description match = 20, path match = 10.
// Maximum score is 100 (sum of all match types).
func matchScore(c *okf.Concept, q string) int {
	score := 0
	if strings.Contains(strings.ToLower(c.Title), q) {
		score += 50
	}
	for _, t := range c.Tags {
		if strings.Contains(strings.ToLower(t), q) {
			score += 30
			break
		}
	}
	if strings.Contains(strings.ToLower(c.Description), q) {
		score += 20
	}
	if strings.Contains(strings.ToLower(c.FilePath), q) {
		score += 10
	}
	return score
}

// handleHealth returns server health and bundle stats.
//
//	GET /api/v1/health
func (h *APIHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
		return
	}
	bundle, err := h.bundleLoader()
	conceptCount := 0
	if err == nil && bundle != nil {
		conceptCount = len(bundle.Concepts)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"bundle":    h.bundlePath,
		"concepts":  conceptCount,
		"loadError": errToString(err),
	})
}

// errToString converts an error to a string for JSON serialization.
func errToString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
