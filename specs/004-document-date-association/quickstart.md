# Quickstart: Testing Document Date Association

**Feature**: 004-document-date-association  
**Date**: 2026-04-25

---

## Prerequisites

- Go 1.25 installed, `CGO_ENABLED=0`
- Node.js + npm for frontend build
- The database file (or a fresh one) at the path configured in the app

---

## Build and run

```bash
# Backend
cd D:/git/pkd
go build ./cmd/pkd && ./pkd

# Frontend (separate terminal)
cd frontend
npm install
npm run dev
```

---

## Manual test plan

### 1. Existing documents get creation date as associated date

1. Start the app with an existing database (documents already created).
2. Open any document → Associations panel.
3. **Expected**: The date section shows the document's creation date (day, month, year) pre-filled in the dropdowns.
4. Verify the "Data de criação" field shows the same date and is read-only (no dropdown/edit).

### 2. New document defaults to today's date

1. Create a new document.
2. Open Associations panel immediately.
3. **Expected**: Year, month, and day dropdowns show today's date.

### 3. Save year-only date

1. Open any document → Associations panel.
2. Clear the month dropdown → day dropdown should clear automatically.
3. Clear the month (if auto-cleared, skip).
4. Save.
5. **Expected**: Display shows only the year (e.g., "2024"). Month and day dropdowns are empty.

### 4. Save month + year date

1. Select a year and a month. Leave day empty.
2. Save.
3. **Expected**: Display shows "Abril/2024" (or equivalent). Day dropdown is empty.

### 5. Cascade: removing month clears day

1. Set a full date (day + month + year).
2. Change the month dropdown to empty.
3. **Expected**: Day dropdown clears automatically without user action.

### 6. Validation: cannot save day without month

1. Using the API directly (or attempting via UI if accessible):
   ```bash
   curl -X PATCH http://localhost:PORT/api/documents/1/associated-date \
     -H 'Content-Type: application/json' \
     -d '{"year":2024,"day":15}'
   ```
2. **Expected**: 400 Bad Request response.

### 7. Clear all date fields

1. Set a full date, then click "Limpar data".
2. Save.
3. **Expected**: All three dropdowns are empty. Document saves without associated date.

### 8. Creation date is immutable

1. Open Associations panel.
2. **Expected**: "Data de criação" is displayed as plain text — no editable controls.
3. Confirm via API: `PUT /api/documents/{id}` — `created_at` field is unchanged regardless of request body.

### 9. Year range

1. Open the year dropdown.
2. **Expected**: Years range from 1900 to (current year + 10, e.g., 2036 in 2026).

---

## API smoke test

```bash
# Set full date
curl -X PATCH http://localhost:PORT/api/documents/1/associated-date \
  -H 'Content-Type: application/json' \
  -d '{"year":2024,"month":4,"day":25}'

# Set year-only
curl -X PATCH http://localhost:PORT/api/documents/1/associated-date \
  -H 'Content-Type: application/json' \
  -d '{"year":2024}'

# Clear date
curl -X PATCH http://localhost:PORT/api/documents/1/associated-date \
  -H 'Content-Type: application/json' \
  -d '{}'
```
