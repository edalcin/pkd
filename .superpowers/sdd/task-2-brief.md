### Task 2: Store — `Create()` e `LookupByToken()`

**Files:**
- Modify: `internal/store/shares.go`

**Interfaces:**
- Consumes: `model.ShareLink.IncludeChildren bool` (Task 1)
- Produces: `ShareStore.Create(docID int64, includeChildren bool)` — consumido pela Task 3.
- Produces: `LookupByToken()` retorna `ShareLink` com `IncludeChildren` preenchido — consumido pela Task 3.

---

- [ ] **Step 1: Atualizar `Create()` para aceitar e persistir `includeChildren`**

Substituir a função `Create` (linhas 26–53 do arquivo atual):

```go
// Create generates a share link for docID. Returns the plaintext token (shown
// once) and the ShareLink record containing the row ID needed for revocation.
// The token is NOT stored — only its SHA-256 hash is persisted.
// includeChildren controls whether sub-document auto-shares are created.
func (s *ShareStore) Create(docID int64, includeChildren bool) (plaintext string, share *model.ShareLink, err error) {
	plaintext = security.NewToken(32) // 32 bytes → 43 chars base64url
	hash := security.HashSHA256(plaintext)

	ic := 0
	if includeChildren {
		ic = 1
	}

	now := time.Now().UTC()
	var id int64
	err = WithTx(s.db, func(tx *sql.Tx) error {
		res, err := tx.Exec(`
			INSERT INTO share_links (document_id, token_hash, token_plain, include_children, created_at)
			VALUES (?, ?, ?, ?, ?)`,
			docID, hash, plaintext, ic, now.Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	share = &model.ShareLink{
		ID:              id,
		DocumentID:      docID,
		IncludeChildren: includeChildren,
		TokenHash:       hash,
		CreatedAt:       now,
	}
	return plaintext, share, nil
}
```

- [ ] **Step 2: Atualizar `LookupByToken()` para incluir `include_children` no SELECT**

Substituir a função `LookupByToken` (linhas 57–86 do arquivo atual):

```go
// LookupByToken finds an active (non-revoked) share link by plaintext token.
// Returns ErrNotFound if the token does not match any active link.
func (s *ShareStore) LookupByToken(plaintext string) (*model.ShareLink, error) {
	hash := security.HashSHA256(plaintext)

	rows, err := s.db.Query(`
		SELECT id, document_id, token_hash, include_children, created_at
		FROM share_links
		WHERE revoked_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var sl model.ShareLink
		var tokenHash []byte
		var createdStr string
		var ic int
		if err := rows.Scan(&sl.ID, &sl.DocumentID, &tokenHash, &ic, &createdStr); err != nil {
			return nil, err
		}
		if subtle.ConstantTimeCompare(hash, tokenHash) == 1 {
			sl.TokenHash = tokenHash
			sl.IncludeChildren = ic == 1
			sl.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
			return &sl, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nil, ErrNotFound
}
```

- [ ] **Step 3: Verificar compilação**

```bash
cd D:/git/pkd && go build ./...
```

Esperado: sem erros de compilação. Pode haver erro de compilação em `handlers_share.go` se ele chama `shares.Create(docID)` — será corrigido na Task 3.

- [ ] **Step 4: Commit**

```bash
cd D:/git/pkd && git add internal/store/shares.go
git commit -m "feat(share): update Create/LookupByToken to handle include_children"
```

---