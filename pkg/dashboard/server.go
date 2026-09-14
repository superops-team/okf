package dashboard

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/superops-team/okf/pkg/okf"
)

//go:embed all:web
var webAssets embed.FS

// ServerConfig holds configuration for the dashboard HTTP server.
type ServerConfig struct {
	// BundlePath is the path to the OKF knowledge bundle directory.
	BundlePath string
	// Addr is the listen address (e.g. ":8080").
	Addr string
	// OpenBrowser indicates whether to open the default browser on start.
	OpenBrowser bool
	// Logger is used for server log output. If nil, log.Default() is used.
	Logger *log.Logger
}

// Server is the dashboard HTTP server.
type Server struct {
	cfg     ServerConfig
	handler *APIHandler
	httpSrv *http.Server
	logger  *log.Logger
	loader  func() (*okf.KnowledgeBundle, error)
}

// NewServer creates a new dashboard server with the given configuration.
func NewServer(cfg ServerConfig) (*Server, error) {
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}

	bundlePath := cfg.BundlePath

	// Cached bundle loader: reloads from disk at most once per cacheTTL.
	// This avoids re-parsing the entire knowledge bundle on every API call
	// (which can be slow for large bundles) while keeping data fresh enough
	// for interactive use. Users can restart the dashboard to force a reload.
	const cacheTTL = 5 * time.Second
	var (
		cachedBundle *okf.KnowledgeBundle
		cachedErr    error
		cachedAt     time.Time
	)
	loadBundle := func() (*okf.KnowledgeBundle, error) {
		okfDir := filepath.Join(bundlePath, ".okf", "knowledge")
		if okf.Exists(okfDir) {
			return okf.LoadBundle(okfDir, okf.DefaultLoadOptions())
		}
		return okf.LoadBundle(bundlePath, okf.DefaultLoadOptions())
	}
	loader := func() (*okf.KnowledgeBundle, error) {
		if cachedBundle != nil && time.Since(cachedAt) < cacheTTL {
			return cachedBundle, cachedErr
		}
		cachedBundle, cachedErr = loadBundle()
		cachedAt = time.Now()
		return cachedBundle, cachedErr
	}

	handler := NewAPIHandler(bundlePath, loader)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux, "/api/v1")

	// Serve embedded static assets at root.
	subFS, err := fs.Sub(webAssets, "web")
	if err != nil {
		return nil, fmt.Errorf("dashboard: failed to mount web assets: %w", err)
	}
	fileServer := http.FileServer(http.FS(subFS))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// SPA fallback: any non-API, non-asset path serves index.html.
		if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/api/") {
			// Check if the requested file exists in the embedded FS.
			if _, err := fs.Stat(subFS, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
				r.URL.Path = "/"
			}
		}
		fileServer.ServeHTTP(w, r)
	})

	httpSrv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      withLogging(mux, cfg.Logger),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return &Server{
		cfg:     cfg,
		handler: handler,
		httpSrv: httpSrv,
		logger:  cfg.Logger,
		loader:  loader,
	}, nil
}

// Start begins listening and blocks until the server is stopped.
// If OpenBrowser is true, it attempts to open the default browser.
func (s *Server) Start() error {
	url := "http://localhost" + s.cfg.Addr
	s.logger.Printf("OKF Dashboard starting at %s", url)
	s.logger.Printf("  Bundle: %s", s.cfg.BundlePath)
	s.logger.Printf("  API:    %s/api/v1/", url)
	s.logger.Printf("  Press Ctrl+C to stop")

	// Pre-warm the bundle cache in the background so the first API call
	// (triggered by page load) hits the cache instead of cold-loading.
	go func() {
		if _, err := s.loader(); err != nil {
			s.logger.Printf("  Warning: preload bundle failed: %v", err)
		} else {
			s.logger.Printf("  Bundle preloaded and cached")
		}
	}()

	if s.cfg.OpenBrowser {
		go openBrowser(url)
	}

	if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("dashboard server error: %w", err)
	}
	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	return s.httpSrv.Close()
}

// withLogging wraps an http.Handler with request logging.
func withLogging(next http.Handler, logger *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		// Only log API requests to keep output clean for asset loads.
		if strings.HasPrefix(r.URL.Path, "/api/") {
			logger.Printf("  %s %s (%s)", r.Method, r.URL.Path, time.Since(start))
		}
	})
}

// openBrowser attempts to open the default web browser at the given URL.
// Errors are logged but do not prevent the server from starting.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	if err := cmd.Start(); err != nil {
		log.Printf("dashboard: could not open browser: %v", err)
	}
}
