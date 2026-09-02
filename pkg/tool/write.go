package tool

import (
	stdctx "context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/superops-team/okf/pkg/okf"
	"github.com/superops-team/okf/pkg/parser"
)

const (
	OperationWrite            = "write"
	ErrIdempotencyConflict    = "idempotency_conflict"
	ErrKnowledgePathOutside   = "path_outside_root"
	maxKnowledgeContentBytes  = 256 * 1024
	maxKnowledgeMetadataBytes = 16 * 1024
	maxKnowledgeTags          = 64
	maxKnowledgeTagBytes      = 128
	maxIdempotencyKeyBytes    = 256
)

var writeKnowledgeLocks sync.Map

// WriteKnowledgeRequest describes an explicit durable knowledge write.
type WriteKnowledgeRequest struct {
	Kind           string         `json:"kind"`
	Content        string         `json:"content"`
	Project        string         `json:"project,omitempty"`
	Tags           []string       `json:"tags,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	IdempotencyKey string         `json:"idempotency_key"`
	EvidenceRefs   []string       `json:"evidence_refs,omitempty"`
}

// WriteKnowledgeResult identifies the durable concept written or reused.
type WriteKnowledgeResult struct {
	ConceptID   string `json:"concept_id"`
	ConceptPath string `json:"concept_path"`
	Created     bool   `json:"created"`
}

type writeKnowledgePayload struct {
	Kind         string         `json:"kind"`
	Content      string         `json:"content"`
	Project      string         `json:"project,omitempty"`
	Tags         []string       `json:"tags,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	EvidenceRefs []string       `json:"evidence_refs,omitempty"`
}

// WriteKnowledge validates and atomically persists one note, event, or feedback concept.
func (s *Service) WriteKnowledge(ctx stdctx.Context, req WriteKnowledgeRequest) ToolEnvelope {
	resolved, err := s.resolve()
	if err != nil {
		return failure(OperationWrite, "", "", nil, err)
	}
	if err := checkContext(ctx); err != nil {
		return failure(OperationWrite, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), err)
	}
	payload, err := normalizeWriteKnowledgeRequest(req)
	if err != nil {
		return failure(OperationWrite, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), err)
	}
	if err := validateKnowledgeWriteRoot(resolved.knowledgeDir); err != nil {
		return failure(OperationWrite, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), err)
	}

	conceptID := stableKnowledgeID(resolved.repoRoot, payload.Kind, strings.TrimSpace(req.IdempotencyKey))
	relPath := filepath.Join(payload.Kind+"s", conceptID+".md")
	fullPath := filepath.Join(resolved.knowledgeDir, relPath)
	lock := knowledgeWriteLock(fullPath)
	lock.Lock()
	defer lock.Unlock()

	payloadHash, err := hashKnowledgePayload(payload)
	if err != nil {
		return failure(OperationWrite, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), err)
	}
	if existing, err := loadExistingKnowledge(fullPath); err != nil {
		return failure(OperationWrite, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), err)
	} else if existing != nil {
		existingHash, _ := existing.CustomFields["payload_hash"].(string)
		if existingHash != payloadHash {
			return failure(OperationWrite, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), toolError{
				code:        ErrIdempotencyConflict,
				message:     "idempotency key is already associated with different knowledge content",
				remediation: "Reuse the key only for an identical retry or choose a new idempotency key.",
			})
		}
		return writeKnowledgeSuccess(resolved, conceptID, relPath, false)
	}

	concept := buildKnowledgeConcept(payload, strings.TrimSpace(req.IdempotencyKey), conceptID, relPath, payloadHash)
	data, err := serializeKnowledgeConcept(concept)
	if err != nil {
		return failure(OperationWrite, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), err)
	}
	if err := s.writeKnowledgeFile(fullPath, data); err != nil {
		// The persistence hook may fail after rename (for example when the
		// containing directory cannot be synced). Treat that as an
		// uncommitted write from the caller's perspective and remove any
		// file that may have become visible before returning the error.
		if rollbackErr := rollbackKnowledgeFile(fullPath); rollbackErr != nil {
			err = fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
		}
		return failure(OperationWrite, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), err)
	}
	persisted, err := parser.ParseConcept(fullPath)
	if err != nil || persisted.CustomFields["payload_hash"] != payloadHash {
		if err == nil {
			err = fmt.Errorf("persisted payload hash mismatch")
		}
		verifyErr := fmt.Errorf("verify persisted knowledge: %w", err)
		if rollbackErr := rollbackKnowledgeFile(fullPath); rollbackErr != nil {
			verifyErr = fmt.Errorf("%w; rollback failed: %v", verifyErr, rollbackErr)
		}
		return failure(OperationWrite, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), verifyErr)
	}
	return writeKnowledgeSuccess(resolved, conceptID, relPath, true)
}

func normalizeWriteKnowledgeRequest(req WriteKnowledgeRequest) (writeKnowledgePayload, error) {
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	switch kind {
	case "note", "event", "feedback":
	default:
		return writeKnowledgePayload{}, invalidWriteRequest("kind must be note, event, or feedback")
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return writeKnowledgePayload{}, invalidWriteRequest("content must not be empty")
	}
	if len(content) > maxKnowledgeContentBytes {
		return writeKnowledgePayload{}, invalidWriteRequest("content exceeds the maximum size")
	}
	key := strings.TrimSpace(req.IdempotencyKey)
	if key == "" {
		return writeKnowledgePayload{}, invalidWriteRequest("idempotency_key must not be empty")
	}
	if len(key) > maxIdempotencyKeyBytes {
		return writeKnowledgePayload{}, invalidWriteRequest("idempotency_key exceeds the maximum size")
	}
	if hasCredentialField(req.Metadata) {
		return writeKnowledgePayload{}, invalidWriteRequest("metadata contains a credential-like field")
	}
	metadataJSON, err := json.Marshal(req.Metadata)
	if err != nil {
		return writeKnowledgePayload{}, invalidWriteRequest("metadata must be valid JSON data")
	}
	if len(metadataJSON) > maxKnowledgeMetadataBytes {
		return writeKnowledgePayload{}, invalidWriteRequest("metadata exceeds the maximum size")
	}
	tags := normalizeLimitedStrings(req.Tags, maxKnowledgeTags, maxKnowledgeTagBytes)
	if len(tags) != len(uniqueNonEmptyStrings(req.Tags)) {
		return writeKnowledgePayload{}, invalidWriteRequest("tags contain empty or oversized values, or exceed the maximum count")
	}
	return writeKnowledgePayload{
		Kind:         kind,
		Content:      content,
		Project:      strings.TrimSpace(req.Project),
		Tags:         tags,
		Metadata:     req.Metadata,
		EvidenceRefs: uniqueNonEmptyStrings(req.EvidenceRefs),
	}, nil
}

func invalidWriteRequest(message string) error {
	return toolError{code: ErrInvalidRequest, message: message, remediation: "Correct the request and retry without sensitive credential fields."}
}

func normalizeCredentialFieldName(name string) string {
	runes := []rune(name)
	var b strings.Builder
	for i, r := range runes {
		if i > 0 && r >= 'A' && r <= 'Z' &&
			((runes[i-1] >= 'a' && runes[i-1] <= 'z') ||
				(i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z')) {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(strings.ToLower(b.String()))
}

func hasCredentialField(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			normalized := normalizeCredentialFieldName(key)
			for _, marker := range []string{"password", "passwd", "secret", "token", "api_key", "access_key", "private_key", "credential"} {
				if normalized == marker || strings.HasSuffix(normalized, "_"+marker) {
					return true
				}
			}
			if hasCredentialField(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if hasCredentialField(nested) {
				return true
			}
		}
	}
	return false
}

func normalizeLimitedStrings(values []string, maxCount, maxBytes int) []string {
	cleaned := uniqueNonEmptyStrings(values)
	if len(cleaned) > maxCount {
		return nil
	}
	for _, value := range cleaned {
		if len(value) > maxBytes {
			return nil
		}
	}
	sort.Strings(cleaned)
	return cleaned
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		cleaned = append(cleaned, value)
	}
	return cleaned
}

func stableKnowledgeID(repoRoot, kind, key string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(repoRoot) + "\x00" + kind + "\x00" + key))
	return hex.EncodeToString(sum[:])
}

func hashKnowledgePayload(payload writeKnowledgePayload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal knowledge payload: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func buildKnowledgeConcept(payload writeKnowledgePayload, key, conceptID, relPath, payloadHash string) *okf.Concept {
	provenance := map[string]any{"evidence_refs": payload.EvidenceRefs}
	if category, ok := payload.Metadata["category"]; ok {
		provenance["category"] = category
	}
	if principle, ok := payload.Metadata["principle"]; ok {
		provenance["principle"] = principle
	}
	return &okf.Concept{
		Type:        payload.Kind,
		Title:       knowledgeTitle(payload.Content),
		Description: knowledgeDescription(payload.Content),
		Tags:        payload.Tags,
		Content:     payload.Content,
		FilePath:    relPath,
		Generated:   &okf.GeneratedInfo{By: "okf-mcp", At: time.Now().UTC().Format(time.RFC3339Nano)},
		Status:      okf.StatusStable,
		CustomFields: map[string]any{
			"concept_id":      conceptID,
			"idempotency_key": key,
			"payload_hash":    payloadHash,
			"project":         payload.Project,
			"metadata":        payload.Metadata,
			"provenance":      provenance,
		},
	}
}

func knowledgeTitle(content string) string {
	line, _, _ := strings.Cut(content, "\n")
	return truncateUTF8Bytes(strings.TrimSpace(line), 96)
}

func knowledgeDescription(content string) string {
	return truncateUTF8Bytes(content, 240)
}

func truncateUTF8Bytes(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	end := maxBytes
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}

func serializeKnowledgeConcept(concept *okf.Concept) ([]byte, error) {
	return parser.SerializeConcept(&parser.Concept{
		Type:         concept.Type,
		Title:        concept.Title,
		Description:  concept.Description,
		Tags:         concept.Tags,
		Content:      concept.Content,
		FilePath:     concept.FilePath,
		CustomFields: concept.CustomFields,
		Status:       string(concept.Status),
		Generated:    &parser.GeneratedInfo{By: concept.Generated.By, At: concept.Generated.At},
	}, true)
}

func loadExistingKnowledge(path string) (*parser.Concept, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat existing knowledge: %w", err)
	}
	concept, err := parser.ParseConcept(path)
	if err != nil {
		return nil, fmt.Errorf("load existing knowledge: %w", err)
	}
	return concept, nil
}

func knowledgeWriteLock(path string) *sync.Mutex {
	lock, _ := writeKnowledgeLocks.LoadOrStore(filepath.Clean(path), &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func validateKnowledgeWriteRoot(root string) error {
	cleanRoot := filepath.Clean(root)
	for current := cleanRoot; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return toolError{code: ErrKnowledgePathOutside, message: "knowledge path contains a symbolic link", remediation: "Use a real repository or knowledge directory path."}
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return nil
}

func rollbackKnowledgeFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove unverified knowledge file: %w", err)
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open knowledge directory for rollback sync: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync knowledge directory after rollback: %w", err)
	}
	return nil
}

func atomicWriteKnowledgeFile(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create knowledge directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".okf-write-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary knowledge file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if err := tmp.Chmod(0o644); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temporary knowledge file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temporary knowledge file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary knowledge file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("commit knowledge file: %w", err)
	}
	directory, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open knowledge directory for sync: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync knowledge directory: %w", err)
	}
	return nil
}

func writeKnowledgeSuccess(resolved resolvedConfig, conceptID, conceptPath string, created bool) ToolEnvelope {
	return ToolEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     OperationWrite,
		OK:            true,
		Mutating:      true,
		RepoRoot:      resolved.repoRoot,
		KnowledgeDir:  resolved.knowledgeDir,
		Freshness:     readFreshness(resolved),
		Warnings:      []string{},
		Result: WriteKnowledgeResult{
			ConceptID:   conceptID,
			ConceptPath: conceptPath,
			Created:     created,
		},
	}
}
