# API Contract: Associated Date

**Feature**: 004-document-date-association  
**Date**: 2026-04-25

---

## PATCH /api/documents/{id}/associated-date

Sets or clears the user-editable associated date on a document. Does **not** perform version checking and does **not** increment the document's `version` field (associated date is metadata, not content).

### Authentication

Requires valid session cookie (same as all other document endpoints).

### Path Parameters

| Parameter | Type | Description |
|---|---|---|
| `id` | integer | Document ID |

### Request Body

```json
{
  "year":  2024,
  "month": 4,
  "day":   25
}
```

All fields are optional. Omitting a field (or passing `null`) clears that component of the date.

| Field | Type | Constraints |
|---|---|---|
| `year` | integer \| null | 1900–(current year + 10) |
| `month` | integer \| null | 1–12; requires `year` |
| `day` | integer \| null | 1–31 (valid for given month/year); requires `month` |

### Validation rules

- `day` present → `month` must also be present → 400 Bad Request
- `month` present → `year` must also be present → 400 Bad Request
- `day` out of range for the given month/year → 400 Bad Request
- Document not found or trashed → 404 Not Found

### Success Response — 200 OK

Returns the full updated document object (same shape as `GET /api/documents/{id}`), with `assoc_year`, `assoc_month`, `assoc_day` reflecting the new values.

```json
{
  "id": 42,
  "title": "My Note",
  "assoc_year": 2024,
  "assoc_month": 4,
  "assoc_day": 25,
  "created_at": "2025-01-15T10:30:00.000Z",
  "updated_at": "2026-04-25T14:00:00.000Z",
  "..."
}
```

### Clear all date fields

Send all fields as null (or omit all):

```json
{}
```

### Error Responses

| Status | Condition |
|---|---|
| 400 | Invalid combination (day without month, month without year, day out of range) |
| 400 | Non-integer value for year/month/day |
| 404 | Document not found or trashed |
| 401 | Not authenticated |

---

## Frontend integration

The Svelte component sends this request whenever the user changes any date dropdown or clicks "Limpar data":

```javascript
await fetch(`/api/documents/${doc.id}/associated-date`, {
  method: 'PATCH',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    year:  selectedYear  ?? null,
    month: selectedMonth ?? null,
    day:   selectedDay   ?? null,
  })
});
```

No optimistic locking needed — this endpoint never conflicts with concurrent content edits.
