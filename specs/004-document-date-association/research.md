# Research: Document Date Association

**Feature**: 004-document-date-association  
**Date**: 2026-04-25

---

## Decision 1: Storage format for partial dates

**Decision**: Three separate nullable INTEGER columns on the `documents` table: `assoc_year`, `assoc_month`, `assoc_day`.

**Rationale**:
- Each field is independently queryable and sortable without string parsing.
- Validation rules ("day requires month") map directly to NULL checks in SQL.
- The project's existing migration pattern (adding nullable columns via `ALTER TABLE` in `migrate.go`) applies without change.
- Avoids encoding/decoding overhead compared to a JSON string column.

**Alternatives considered**:

| Alternative | Rejected because |
|---|---|
| Single TEXT column ("2024", "2024-04", "2024-04-25") | Requires string parsing to sort/query; partial ordering is ambiguous |
| Single JSON TEXT column (`{"year":2024,"month":4}`) | Same parsing burden; no benefit over 3 columns in SQLite |
| Single nullable DATE column | Cannot represent partial dates (year-only or month+year) |

**Validation rules expressible as SQL constraints**:
- `assoc_day IS NOT NULL → assoc_month IS NOT NULL` (day requires month)
- `assoc_month IS NOT NULL → assoc_year IS NOT NULL` (month requires year)
- Enforced in the application layer (Go handler), not as DB constraints (consistent with existing codebase style).

---

## Decision 2: Sorting algorithm for partial dates

**Decision**: Treat partial dates as the start of their period for ordering purposes.

**Sorting expression** (ascending, NULLs last):

```sql
CASE
  WHEN assoc_year IS NULL THEN NULL
  ELSE (assoc_year * 10000)
     + (COALESCE(assoc_month, 1) * 100)
     + COALESCE(assoc_day, 1)
END ASC NULLS LAST
```

**Rationale**: Year-only "2024" sorts as 2024-01-01; "April 2024" sorts as 2024-04-01. This is the standard approach used in library catalogues and historical records (Dublin Core). The spec confirmed this behaviour (clarification Q5).

---

## Decision 3: New API endpoint vs. extending existing update handler

**Decision**: New dedicated endpoint `PATCH /api/documents/{id}/associated-date`.

**Rationale**:
- The existing `PUT /api/documents/{id}` handler performs **version-checked optimistic locking** for document content (title, body, icon). Associated date is metadata — it should be independently settable without triggering a version conflict or incrementing `version`.
- Consistent with project pattern of separate focused handlers (e.g., `handleToggleFavorite`, `handleSetFavorite`).
- Keeps `handleUpdateDocument` unchanged, reducing regression surface.

**Request body**:
```json
{ "year": 2024, "month": 4, "day": 25 }
```
All fields are optional (omitted = null = clear that field). See `contracts/associated-date.md`.

---

## Decision 4: Year range in the frontend selector

**Decision**: 1900 to `new Date().getFullYear() + 10`.

**Rationale**: Confirmed in clarification Q1. Supports both historical records and near-future planning (meetings, deadlines).

---

## Decision 5: Cascade behaviour when month is cleared

**Decision**: Clearing the month dropdown also clears the day dropdown immediately (client-side reactive cascade).

**Rationale**: Prevents the user from accidentally saving an invalid state (day without month). Confirmed in clarification Q4. Implemented as a Svelte reactive statement: `$: if (!selectedMonth) selectedDay = null`.

---

## Migration strategy for existing documents

**Decision**: Backfill `assoc_year`, `assoc_month`, `assoc_day` from the document's `created_at` timestamp at startup (one-time data migration in `migrate.go`, idempotent via `WHERE assoc_year IS NULL`).

**SQL**:
```sql
UPDATE documents
SET
  assoc_year  = CAST(strftime('%Y', created_at) AS INTEGER),
  assoc_month = CAST(strftime('%m', created_at) AS INTEGER),
  assoc_day   = CAST(strftime('%d', created_at) AS INTEGER)
WHERE assoc_year IS NULL;
```

This runs at every startup but only touches rows where `assoc_year` is still NULL (i.e., newly added rows from the ALTER TABLE migration).
