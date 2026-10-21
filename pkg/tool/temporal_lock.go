package tool

import (
	"path/filepath"
	"sync"
)

// Repository-scoped temporal lock.
//
// The per-path knowledgeWriteLock already serializes writes to one file. The
// temporal-aware operations need a coarser repo-root-scoped mutex because an
// approved relation write (and the ReviewMemory CAS mutation) must observe the
// WHOLE bundle: it loads every concept, builds the identity registry and the
// memorymeta view, runs the relation preflight, and only then performs the
// atomic write plus the persisted parse-back verify. That whole critical section
// must not interleave with another relation write or review against the same
// repo, otherwise two writers could both pass preflight and create an ambiguous
// or non-current update head.
//
// Lock order is fixed and must never be inverted:
//
//	repositoryTemporalLock(knowledgeDir)  // outer, coarse
//	knowledgeWriteLock(fullPath)          // inner, per-file
//
// Plain durable writes that carry no temporal fields take ONLY the per-path
// lock (S24): they must not be serialized behind the repo lock.

var repositoryTemporalLocks sync.Map

// repositoryTemporalLock returns the repo-scoped mutex for the given resolved
// repo/knowledge root. The key is cleaned so that identical logical roots map to
// the same mutex regardless of trailing separators or "." segments.
func repositoryTemporalLock(key string) *sync.Mutex {
	cleaned := filepath.Clean(key)
	lock, _ := repositoryTemporalLocks.LoadOrStore(cleaned, &sync.Mutex{})
	return lock.(*sync.Mutex)
}
