package tool

import (
	stdctx "context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/superops-team/okf/pkg/identity"
	"github.com/superops-team/okf/pkg/memorydefense"
	"github.com/superops-team/okf/pkg/memorymeta"
	"github.com/superops-team/okf/pkg/okf"
	"github.com/superops-team/okf/pkg/parser"
)

const (
	OperationWrite            = "write"
	ErrIdempotencyConflict    = "idempotency_conflict"
	ErrKnowledgePathOutside   = "path_outside_root"
	ErrMemoryDefenseBlocked   = "memory_defense_blocked"
	ErrRedactionEmpty         = "redaction_empty"
	maxKnowledgeContentBytes  = 256 * 1024
	maxKnowledgeMetadataBytes = 16 * 1024
	maxKnowledgeTags          = 64
	maxKnowledgeTagBytes      = 128
	maxIdempotencyKeyBytes    = 256
)

var writeKnowledgeLocks sync.Map

// reRedactionPlaceholder matches [REDACTED:label] markers to detect degenerate
// writes where the entire content was secrets.
var reRedactionPlaceholder = regexp.MustCompile(`\[REDACTED:[^\]]*\]`)

// WriteKnowledgeRequest describes an explicit durable knowledge write.
type WriteKnowledgeRequest struct {
	Kind           string         `json:"kind"`
	Content        string         `json:"content"`
	Project        string         `json:"project,omitempty"`
	Tags           []string       `json:"tags,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	IdempotencyKey string         `json:"idempotency_key"`
	EvidenceRefs   []string       `json:"evidence_refs,omitempty"`
	// Temporal memory fields (design §5). When all are absent the write is a
	// legacy durable write whose serialized output is byte-for-byte unchanged.
	//
	// MemoryState defaults to approved when omitted. proposed requires a finite
	// [0,1] MemoryConfidence and at least one evidence_ref; approved must not
	// carry a confidence. declined cannot be created directly.
	MemoryState string `json:"memory_state,omitempty"`
	// MemoryConfidence is the proposal confidence. It is only meaningful (and
	// only accepted) for a proposed write.
	MemoryConfidence *float64 `json:"memory_confidence,omitempty"`
	// MemoryRelationKind is "updates" (exactly 1 target) or "extends" (1-8
	// targets). Empty means no temporal relation.
	MemoryRelationKind string `json:"memory_relation_kind,omitempty"`
	// MemoryRelationTargets are stable ids (bare okf id or okf://concept/<id>).
	MemoryRelationTargets []string `json:"memory_relation_targets,omitempty"`
}

// WriteKnowledgeResult identifies the durable concept written or reused.
type WriteKnowledgeResult struct {
	ConceptID   string `json:"concept_id"`
	ConceptPath string `json:"concept_path"`
	Created     bool   `json:"created"`
	// OKFID is the canonical stable okf id assigned to (or reused for) the
	// concept. Ref is its canonical okf://concept/<id> URI.
	OKFID string `json:"okf_id,omitempty"`
	Ref   string `json:"ref,omitempty"`
	// Redactions lists memory-defense detectors that fired on this write.
	Redactions []memorydefense.Hit `json:"redactions,omitempty"`
}

type writeKnowledgePayload struct {
	Kind         string         `json:"kind"`
	Content      string         `json:"content"`
	Project      string         `json:"project,omitempty"`
	Tags         []string       `json:"tags,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	EvidenceRefs []string       `json:"evidence_refs,omitempty"`
	// Temporal fields participate in the payload hash (S21): an identical retry
	// reuses the concept, while a changed state/confidence/relation under the
	// same key surfaces as an idempotency_conflict. All are omitempty so legacy
	// writes keep their exact serialized hash.
	MemoryState           string   `json:"memory_state,omitempty"`
	MemoryConfidence      *float64 `json:"memory_confidence,omitempty"`
	MemoryRelationKind    string   `json:"memory_relation_kind,omitempty"`
	MemoryRelationTargets []string `json:"memory_relation_targets,omitempty"`
}

// hasTemporal reports whether this write carries any temporal memory metadata and
// therefore must serialize behind the repo-scoped temporal lock (S24).
func (p writeKnowledgePayload) hasTemporal() bool {
	return p.MemoryState == string(memorymeta.MemoryProposed) || p.MemoryRelationKind != ""
}

// hasApprovedRelation reports whether this is an APPROVED relation write, which
// requires the full bundle preflight before the first byte is written (S22).
// Proposed relation metadata is stored inactive and preflighted again at review.
func (p writeKnowledgePayload) hasApprovedRelation() bool {
	return p.MemoryState != string(memorymeta.MemoryProposed) && p.MemoryRelationKind != ""
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

	// Memory Defense: scan content body (the existing hasCredentialField gate
	// only inspects metadata field names). Screen runs before payload hashing
	// so redacted content is what gets persisted and idempotency-hashed.
	pol, err := memorydefense.LoadPolicy(resolved.repoRoot)
	if err != nil {
		return failure(OperationWrite, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved),
			toolError{code: ErrInvalidRequest, message: err.Error(), remediation: "Fix memory_defense config in .okf/config.yaml."})
	}
	redacted, defenseHits, err := memorydefense.Screen(payload.Content, pol)
	if err != nil {
		var blocked *memorydefense.ErrBlocked
		if errors.As(err, &blocked) {
			return failure(OperationWrite, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved),
				toolError{code: ErrMemoryDefenseBlocked,
					message:     "write blocked by memory defense: high-severity secret detected",
					remediation: "Remove the secret from content before writing; detector=" + blocked.DetectorID})
		}
		return failure(OperationWrite, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), err)
	}
	payload.Content = redacted
	// Reject degenerate writes where the entire content was secrets (after
	// stripping redaction placeholders, nothing meaningful remains).
	stripped := reRedactionPlaceholder.ReplaceAllString(payload.Content, "")
	if strings.TrimSpace(stripped) == "" {
		return failure(OperationWrite, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved),
			toolError{code: ErrRedactionEmpty,
				message:     "content became empty after memory-defense redaction",
				remediation: "Provide non-secret content alongside any redacted material."})
	}
	writeRedactions := defenseHits

	conceptID := stableKnowledgeID(resolved.repoRoot, payload.Kind, strings.TrimSpace(req.IdempotencyKey))
	relPath := filepath.Join(payload.Kind+"s", conceptID+".md")
	fullPath := filepath.Join(resolved.knowledgeDir, relPath)

	// Temporal writes observe the whole bundle (registry + memorymeta view) and
	// must serialize behind the repo-scoped lock. Legacy writes keep the existing
	// per-path-only critical section and never take the repo lock (S24). Lock
	// order is fixed: repo lock first, then the per-path lock.
	if payload.hasTemporal() {
		repoLock := repositoryTemporalLock(resolved.knowledgeDir)
		repoLock.Lock()
		defer repoLock.Unlock()
	}

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
		// Identical retry: reuse the on-disk concept (preserving any review it has
		// received) and return its current stable identity (S21).
		return writeKnowledgeSuccess(resolved, conceptID, relPath, false, okfIDFromCustomFields(existing.CustomFields), nil)
	}

	// An APPROVED relation write validates the whole bundle topology before the
	// first byte is persisted. Invalid topology aborts with zero files on disk
	// (S22). Proposed relation metadata is stored inactive and validated at review.
	if payload.hasApprovedRelation() {
		if err := preflightWriteRelation(resolved.knowledgeDir, payload); err != nil {
			return failure(OperationWrite, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), err)
		}
	}

	concept := buildKnowledgeConcept(payload, strings.TrimSpace(req.IdempotencyKey), conceptID, relPath, payloadHash)
	// Additive stable ref: durable capture keeps its deterministic concept_id as
	// the idempotency/path handle, and additionally receives a stable okf_id.
	// The two are never substituted for one another.
	okfID, err := identity.EnsureFinalID(concept, fullPath)
	if err != nil {
		return failure(OperationWrite, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), err)
	}
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
	return writeKnowledgeSuccess(resolved, conceptID, relPath, true, okfID, writeRedactions)
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
	evidenceRefs := uniqueNonEmptyStrings(req.EvidenceRefs)

	state, confidence, relationKind, relationTargets, err := normalizeTemporalFields(req, evidenceRefs)
	if err != nil {
		return writeKnowledgePayload{}, err
	}

	return writeKnowledgePayload{
		Kind:         kind,
		Content:      content,
		Project:      strings.TrimSpace(req.Project),
		Tags:         tags,
		Metadata:     req.Metadata,
		EvidenceRefs: evidenceRefs,
		MemoryState:  state, MemoryConfidence: confidence,
		MemoryRelationKind: relationKind, MemoryRelationTargets: relationTargets,
	}, nil
}

// normalizeTemporalFields validates the optional temporal memory fields and
// returns their normalized forms. The returned state is always "approved" or
// "proposed" (an omitted state normalizes to approved).
func normalizeTemporalFields(req WriteKnowledgeRequest, evidenceRefs []string) (string, *float64, string, []string, error) {
	state := strings.ToLower(strings.TrimSpace(req.MemoryState))
	switch state {
	case "":
		state = string(memorymeta.MemoryApproved)
	case string(memorymeta.MemoryApproved), string(memorymeta.MemoryProposed):
	default:
		if state == string(memorymeta.MemoryDeclined) {
			return "", nil, "", nil, temporalError(ErrInvalidMemoryState,
				"declined memory state cannot be created directly",
				"Write the concept as proposed and decline it through memory_review.")
		}
		return "", nil, "", nil, temporalError(ErrInvalidMemoryState,
			fmt.Sprintf("invalid memory_state %q", req.MemoryState),
			"memory_state must be approved or proposed.")
	}

	var confidence *float64
	if req.MemoryConfidence != nil {
		if state == string(memorymeta.MemoryApproved) {
			return "", nil, "", nil, temporalError(ErrInvalidMemoryState,
				"approved memory must not carry memory_confidence",
				"Confidence is only recorded for proposed concepts.")
		}
		v := *req.MemoryConfidence
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return "", nil, "", nil, temporalError(ErrInvalidMemoryState,
				fmt.Sprintf("memory_confidence %v must be finite in [0,1]", v),
				"Provide a confidence between 0 and 1 for a proposed concept.")
		}
		confidence = &v
	}
	if state == string(memorymeta.MemoryProposed) {
		if confidence == nil {
			return "", nil, "", nil, temporalError(ErrInvalidMemoryState,
				"proposed memory requires memory_confidence",
				"Provide a finite [0,1] confidence for a proposed concept.")
		}
		if len(evidenceRefs) == 0 {
			return "", nil, "", nil, temporalError(ErrInvalidMemoryState,
				"proposed memory requires at least one evidence_ref",
				"Attach at least one evidence_ref to a proposed concept.")
		}
	}

	relationKind := strings.ToLower(strings.TrimSpace(req.MemoryRelationKind))
	var relationTargetsOut []string
	if relationKind != "" {
		switch memorymeta.RelationKind(relationKind) {
		case memorymeta.RelationUpdates, memorymeta.RelationExtends:
		default:
			return "", nil, "", nil, temporalError(ErrInvalidMemoryRelation,
				fmt.Sprintf("invalid memory_relation_kind %q", req.MemoryRelationKind),
				"memory_relation_kind must be updates or extends.")
		}
		rawTargets := uniqueNonEmptyStrings(req.MemoryRelationTargets)
		seen := make(map[string]bool, len(rawTargets))
		for _, raw := range rawTargets {
			target, err := normalizeRelationTarget(raw)
			if err != nil {
				return "", nil, "", nil, temporalError(ErrInvalidMemoryRelation,
					err.Error(),
					"Relation targets must be a bare okf id or okf://concept/<id>.")
			}
			if seen[target] {
				return "", nil, "", nil, temporalError(ErrInvalidMemoryRelation,
					fmt.Sprintf("duplicate relation target %q", target),
					"List each relation target exactly once.")
			}
			seen[target] = true
			relationTargetsOut = append(relationTargetsOut, target)
		}
		switch memorymeta.RelationKind(relationKind) {
		case memorymeta.RelationUpdates:
			if len(relationTargetsOut) != 1 {
				return "", nil, "", nil, temporalError(ErrInvalidMemoryRelation,
					fmt.Sprintf("updates requires exactly 1 target, got %d", len(relationTargetsOut)),
					"Point an updates relation at exactly one prior concept.")
			}
		case memorymeta.RelationExtends:
			if len(relationTargetsOut) < 1 || len(relationTargetsOut) > 8 {
				return "", nil, "", nil, temporalError(ErrInvalidMemoryRelation,
					fmt.Sprintf("extends requires 1-8 targets, got %d", len(relationTargetsOut)),
					"List between 1 and 8 targets for an extends relation.")
			}
		}
	}

	return state, confidence, relationKind, relationTargetsOut, nil
}

// normalizeRelationTarget strips the canonical okf://concept/ prefix and validates
// the bare okf id grammar.
func normalizeRelationTarget(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimPrefix(ref, identity.URIScheme)
	return identity.Parse(ref)
}

// okfIDFromCustomFields extracts and validates the stable okf id from a parsed
// concept's CustomFields.
func okfIDFromCustomFields(cf map[string]any) string {
	if cf == nil {
		return ""
	}
	id, _ := cf["okf_id"].(string)
	if _, err := identity.Parse(id); err != nil {
		return ""
	}
	return id
}

// preflightWriteRelation loads the bundle and validates an approved relation write
// against the current topology. It reads no files of its own; on failure nothing
// has been written (S22).
func preflightWriteRelation(knowledgeDir string, payload writeKnowledgePayload) error {
	bundle, err := okf.LoadBundle(knowledgeDir, okf.DefaultLoadOptions())
	if err != nil {
		return fmt.Errorf("load bundle for relation preflight: %w", err)
	}
	reg, err := identity.BuildRegistry(bundle.Concepts)
	if err != nil {
		return fmt.Errorf("build registry for relation preflight: %w", err)
	}
	view := memorymeta.BuildTemporalView(bundle.Concepts)
	rel := memorymeta.MemoryRelation{
		Kind:    memorymeta.RelationKind(payload.MemoryRelationKind),
		Targets: payload.MemoryRelationTargets,
	}
	return preflightApprovedRelation(reg, view, payload.Project, rel, "")
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
	concept := &okf.Concept{
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
	applyTemporalCustomFields(concept, payload)
	return concept
}

// applyTemporalCustomFields writes the memory_* temporal metadata onto a freshly
// built concept. An approved, relation-less concept leaves memory_state absent so
// that legacy serialized output is byte-for-byte unchanged (S18).
func applyTemporalCustomFields(concept *okf.Concept, payload writeKnowledgePayload) {
	if payload.MemoryState == string(memorymeta.MemoryProposed) {
		memorymeta.SetState(concept, memorymeta.MemoryProposed)
		if payload.MemoryConfidence != nil {
			concept.CustomFields["memory_confidence"] = *payload.MemoryConfidence
		}
	}
	if payload.MemoryRelationKind != "" {
		memorymeta.SetRelation(concept, memorymeta.MemoryRelation{
			Kind:    memorymeta.RelationKind(payload.MemoryRelationKind),
			Targets: payload.MemoryRelationTargets,
		})
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

func writeKnowledgeSuccess(resolved resolvedConfig, conceptID, conceptPath string, created bool, okfID string, redactions []memorydefense.Hit) ToolEnvelope {
	ref := ""
	if okfID != "" {
		ref = identity.CanonicalURI(okfID)
	}
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
			OKFID:       okfID,
			Ref:         ref,
			Redactions:  redactions,
		},
	}
}
