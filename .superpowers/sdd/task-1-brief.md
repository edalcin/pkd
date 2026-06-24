### Task 1: Migration e Model

**Files:**
- Modify: `internal/store/migrate.go:85-109`
- Modify: `internal/model/share.go:1-31`

**Interfaces:**
- Produces: `model.ShareLink.IncludeChildren bool` — usado pelas Tasks 2 e 3.

---

- [ ] **Step 1: Adicionar migration em `migrate.go`**

No slice `colMigrations` (linhas 85–101), adicionar entrada após a última:

```go
{`ALTER TABLE share_links ADD COLUMN include_children INTEGER NOT NULL DEFAULT 1`, "alter share_links include_children"},
```

O bloco completo fica:
```go
colMigrations := []struct {
    sql string
    ctx string
}{
    {`ALTER TABLE documents ADD COLUMN is_favorite INTEGER NOT NULL DEFAULT 0`, "alter documents is_favorite"},
    {`ALTER TABLE document_links ADD COLUMN manual INTEGER NOT NULL DEFAULT 0`, "alter document_links"},
    {`ALTER TABLE tags ADD COLUMN color TEXT NOT NULL DEFAULT ''`, "alter tags color"},
    {`ALTER TABLE share_links ADD COLUMN token_plain TEXT NOT NULL DEFAULT ''`, "alter share_links token_plain"},
    {`ALTER TABLE share_links ADD COLUMN is_auto INTEGER NOT NULL DEFAULT 0`, "alter share_links is_auto"},
    {`ALTER TABLE documents ADD COLUMN assoc_year  INTEGER`, "alter documents assoc_year"},
    {`ALTER TABLE documents ADD COLUMN assoc_month INTEGER`, "alter documents assoc_month"},
    {`ALTER TABLE documents ADD COLUMN assoc_day   INTEGER`, "alter documents assoc_day"},
    {`ALTER TABLE documents ADD COLUMN locked      INTEGER NOT NULL DEFAULT 0`, "alter documents locked"},
    {`ALTER TABLE documents ADD COLUMN archived_at TEXT`, "alter documents archived_at"},
    {`ALTER TABLE attachments ADD COLUMN storage_location TEXT NOT NULL DEFAULT 'local'`, "alter attachments storage_location"},
    {`ALTER TABLE attachments ADD COLUMN content_sha256 TEXT`, "alter attachments content_sha256"},
    {`ALTER TABLE share_links ADD COLUMN include_children INTEGER NOT NULL DEFAULT 1`, "alter share_links include_children"},
}
```

- [ ] **Step 2: Adicionar campo `IncludeChildren` ao model**

Substituir o conteúdo de `internal/model/share.go`:

```go
package model

import "time"

// ShareLink mirrors the share_links table.
type ShareLink struct {
	ID              int64      `json:"id"`
	DocumentID      int64      `json:"document_id"`
	IncludeChildren bool       `json:"include_children"`
	TokenHash       []byte     `json:"-"` // never sent to clients
	CreatedAt       time.Time  `json:"created_at"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
}

// ShareCreateResponse is returned once when a share link is created.
// The plaintext token is shown exactly once and never stored.
type ShareCreateResponse struct {
	Token    string `json:"token"`
	URL      string `json:"url"`
	RevokeID int64  `json:"revoke_id"`
}

// ShareWithDoc is returned by the admin list-shares endpoint.
// It enriches a ShareLink with the associated document title and the full public URL.
type ShareWithDoc struct {
	ID            int64     `json:"id"`
	DocumentID    int64     `json:"document_id"`
	DocumentTitle string    `json:"document_title"`
	CreatedAt     time.Time `json:"created_at"`
	Token         string    `json:"token"`         // plaintext token; empty for shares created before this field was added
	URL           string    `json:"url,omitempty"` // full public URL, populated by the handler
}
```

- [ ] **Step 3: Verificar compilação**

```bash
cd D:/git/pkd && go build ./...
```

Esperado: sem erros.

- [ ] **Step 4: Commit**

```bash
cd D:/git/pkd && git add internal/store/migrate.go internal/model/share.go
git commit -m "feat(share): add include_children column and model field"
```

---