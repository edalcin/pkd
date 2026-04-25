# Implementation Plan: Document Date Association

**Branch**: `004-document-date-association` | **Date**: 2026-04-25 | **Spec**: [spec.md](./spec.md)  
**Input**: Feature specification from `/specs/004-document-date-association/spec.md`

## Summary

Add a user-editable "associated date" (partial: year-only, month+year, or full day+month+year) to the document Associations panel. Existing documents are back-filled with their creation date. The implementation touches the SQLite schema (3 new nullable columns), the Go data model and store layer, one new API endpoint, and the `Editor.svelte` Associations panel UI.

## Technical Context

**Language/Version**: Go 1.25 (backend, CGO disabled), JavaScript/Svelte 5 (frontend)  
**Primary Dependencies**: chi v5 (router), modernc.org/sqlite v1.48.2, Svelte 5 + Vite  
**Storage**: SQLite — single file, ISO-8601 strings for timestamps, integer columns for partial dates  
**Testing**: Go standard `testing` package  
**Target Platform**: Self-hosted web application (single user)  
**Project Type**: Web application — Go JSON API backend + Svelte SPA frontend  
**Performance Goals**: Personal app — no specific throughput targets  
**Constraints**: CGO disabled (pure Go); SQLite single-writer; ALTER TABLE migrations must be idempotent  
**Scale/Scope**: Single user; documents table grows incrementally

## Constitution Check

Constitution file is an unfilled template — no project-specific gates apply. No violations to track.

## Project Structure

### Documentation (this feature)

```text
specs/004-document-date-association/
├── plan.md              # This file
├── research.md          # Phase 0 — storage & sorting decisions
├── data-model.md        # Phase 1 — schema + Go struct changes
├── contracts/
│   └── associated-date.md   # PATCH /api/documents/{id}/associated-date
├── quickstart.md        # Phase 1 — how to test the feature end-to-end
└── tasks.md             # Phase 2 output (created by /speckit.tasks)
```

### Source Code (affected files)

```text
internal/
├── model/
│   └── document.go          # Add AssocYear, AssocMonth, AssocDay *int fields
├── store/
│   ├── migrate.go           # Add 3 ALTER TABLE + backfill data migration
│   └── documents.go         # Update scan helpers + add UpdateAssocDate method
└── server/
    ├── server.go             # Register new route PATCH /api/documents/{id}/associated-date
    └── handlers_documents.go # Add handleUpdateAssocDate() handler

frontend/src/lib/components/
└── Editor.svelte            # Add date section to .assoc-area
```

## Complexity Tracking

No constitution violations — section not applicable.
