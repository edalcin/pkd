# Data Model: Document Date Association

**Feature**: 004-document-date-association  
**Date**: 2026-04-25

---

## Schema changes — `documents` table

Three nullable INTEGER columns are added via idempotent `ALTER TABLE` migrations in `migrate.go`:

```sql
ALTER TABLE documents ADD COLUMN assoc_year  INTEGER;
ALTER TABLE documents ADD COLUMN assoc_month INTEGER;
ALTER TABLE documents ADD COLUMN assoc_day   INTEGER;
```

### Valid states

| assoc_year | assoc_month | assoc_day | Meaning |
|---|---|---|---|
| NULL | NULL | NULL | No associated date |
| 2024 | NULL | NULL | Year only ("2024") |
| 2024 | 4 | NULL | Month + year ("Abril/2024") |
| 2024 | 4 | 25 | Full date ("25/04/2024") |
| NULL | 4 | — | **INVALID** — month requires year |
| 2024 | NULL | 25 | **INVALID** — day requires month |
| NULL | NULL | 25 | **INVALID** — day requires year and month |

Validation is enforced in the Go handler layer, not as a DB constraint.

### Sorting expression (ascending, documents with no date last)

```sql
CASE
  WHEN assoc_year IS NULL THEN NULL
  ELSE (assoc_year * 10000)
     + (COALESCE(assoc_month, 1) * 100)
     + COALESCE(assoc_day, 1)
END ASC NULLS LAST
```

---

## Go model — `internal/model/document.go`

Add three pointer fields to the `Document` struct (pointer = nullable):

```go
AssocYear  *int `json:"assoc_year,omitempty"`
AssocMonth *int `json:"assoc_month,omitempty"`
AssocDay   *int `json:"assoc_day,omitempty"`
```

---

## Store changes — `internal/store/documents.go`

### `scanDocRow` — add 3 columns to SELECT and Scan

The SELECT in `scanDoc` and `scanDocFromTx` must include the new columns:

```sql
SELECT id, parent_id, title, body_html, body_text, icon, position, version,
       is_favorite, created_at, updated_at,
       assoc_year, assoc_month, assoc_day          -- NEW
FROM documents WHERE id = ? AND trashed_at IS NULL
```

`scanDocRow` scan targets:

```go
var assocYear, assocMonth, assocDay sql.NullInt64   // NEW
err := row.Scan(
    &doc.ID, &parentID, &doc.Title,
    &bodyHTML, &bodyText, &icon,
    &doc.Position, &doc.Version, &doc.IsFavorite,
    &createdStr, &updatedStr,
    &assocYear, &assocMonth, &assocDay,             // NEW
)
// after scan:
if assocYear.Valid  { v := int(assocYear.Int64);  doc.AssocYear  = &v }
if assocMonth.Valid { v := int(assocMonth.Int64); doc.AssocMonth = &v }
if assocDay.Valid   { v := int(assocDay.Int64);   doc.AssocDay   = &v }
```

`scanDocRows` (used in list queries) requires the same column additions.

### New method — `UpdateAssocDate`

```go
// UpdateAssocDate sets the user-editable associated date fields on a document.
// year/month/day are nullable — pass nil to clear a field.
// Does NOT bump version (associated date is metadata, not document content).
func (s *DocumentStore) UpdateAssocDate(id int64, year, month, day *int) (*model.Document, error) {
    _, err := s.db.Exec(
        `UPDATE documents SET assoc_year = ?, assoc_month = ?, assoc_day = ?
         WHERE id = ? AND trashed_at IS NULL`,
        year, month, day, id)
    if err != nil {
        return nil, err
    }
    return s.GetByID(id)
}
```

---

## Migration strategy — `internal/store/migrate.go`

### Column migrations (added to `colMigrations` slice)

```go
{`ALTER TABLE documents ADD COLUMN assoc_year  INTEGER`, "alter documents assoc_year"},
{`ALTER TABLE documents ADD COLUMN assoc_month INTEGER`, "alter documents assoc_month"},
{`ALTER TABLE documents ADD COLUMN assoc_day   INTEGER`, "alter documents assoc_day"},
```

### Data backfill (added after the existing icon data migration)

```go
if _, err := db.Exec(`
    UPDATE documents
    SET
        assoc_year  = CAST(strftime('%Y', created_at) AS INTEGER),
        assoc_month = CAST(strftime('%m', created_at) AS INTEGER),
        assoc_day   = CAST(strftime('%d', created_at) AS INTEGER)
    WHERE assoc_year IS NULL
`); err != nil {
    db.Close()
    return nil, fmt.Errorf("store.Open assoc_date backfill: %w", err)
}
```

Runs every startup but only touches rows where `assoc_year IS NULL` — idempotent.

---

## No new tables or indexes required

The three columns on `documents` are sufficient. No foreign keys, join tables, or FTS changes needed.
