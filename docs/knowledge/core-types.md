---
type: documentation
title: OKF Core Types (v0.2)
description: Core type system for OKF v0.2 including Concept, KnowledgeBundle, provenance, trust tiers, and lifecycle status.
tags: [okf, types, v0.2, core, architecture]
status: stable
---

# OKF Core Types (v0.2)

## Concept

The `Concept` struct represents a single unit of knowledge, corresponding to one Markdown file with YAML frontmatter.

### v0.1 Fields (carried forward)
- `Type` - Category of the concept (REQUIRED)
- `Title` - Human-readable name
- `Description` - Brief summary
- `Resource` - Reference to the actual resource
- `Tags` - Arbitrary labels for categorization
- `Timestamp` - LEGACY field, superseded by `Generated.At`

### v0.2 New Fields
- `Sources` - Materials the concept derives from (spec §5.1)
- `UsageWindow` - Date range framing usage_count
- `Generated` - How the content was produced (by, at)
- `Verified` - Verification events list
- `Status` - Lifecycle: draft | stable | deprecated
- `StaleAfter` - Absolute date (YYYY-MM-DD) after which concept is stale

### Attested Computation Fields (spec §10)
- `Runtime` - How to run the computation
- `Parameters` - Typed, named holes the agent may fill
- `Computation` - Path to computation file
- `Executor` - How the computation is run
- `Attester` - Deterministic check reference

## KnowledgeBundle

A collection of concepts loaded from a directory tree.

### Key Methods
- `Stats()` - Returns BundleStats with type counts, trust tiers, status counts
- `FilterConcepts(predicate)` - Filter concepts by function
- `RelatedConcepts(concept)` - Find concepts sharing tags or resources

## TrustTier

Three-tier trust model:
- `TrustUnverified` (0) - No verified key
- `TrustMachineConfirmed` (1) - Verified by non-human actors only
- `TrustHumanReviewed` (2) - Verified by a human actor

## ConceptStatus

Lifecycle statuses:
- `StatusDraft` - Work in progress
- `StatusStable` - Default, production-ready
- `StatusDeprecated` - Should not be used for new work
