### Task 3: Handlers — criação e página pública

**Files:**
- Modify: `internal/server/handlers_share.go`

**Interfaces:**
- Consumes: `ShareStore.Create(docID int64, includeChildren bool)` (Task 2)
- Consumes: `model.ShareLink.IncludeChildren bool` (Task 1)

---

- [ ] **Step 1: Atualizar `handleCreateShare()` para ler `include_children` do body**

Substituir a função `handleCreateShare` (linhas 59–91 do arquivo atual):

```go
func (s *Server) handleCreateShare() http.HandlerFunc {
	type createShareRequest struct {
		IncludeChildren *bool `json:"include_children"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		docID, err := parseID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if _, err := s.docs.GetByID(docID); errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		var req createShareRequest
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
		}
		// Default: include children (backward-compatible).
		includeChildren := true
		if req.IncludeChildren != nil {
			includeChildren = *req.IncludeChildren
		}

		plaintext, share, err := s.shares.Create(docID, includeChildren)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Auto-create shares for all descendants only when recursive.
		if includeChildren {
			for _, id := range s.collectDescendantIDs(docID) {
				_ = s.shares.CreateAuto(id)
			}
		}

		base := s.baseURL(r)
		shareURL := base + "public/" + plaintext

		writeJSON(w, http.StatusCreated, model.ShareCreateResponse{
			Token:    plaintext,
			URL:      shareURL,
			RevokeID: share.ID,
		})
	}
}
```

Adicionar `"encoding/json"` ao bloco de imports se ainda não estiver presente (verificar o import atual do arquivo).

- [ ] **Step 2: Atualizar `handlePublicShare()` para respeitar `include_children`**

Na função `handlePublicShare`, após a linha que busca o `doc` (aproximadamente linha 175), inserir a lógica de short-circuit para filhos:

Localizar o bloco que começa com `// Fetch children and ensure each has an active public share.` (linhas ~174–196) e substituí-lo por:

```go
// Fetch children only if the share was created with include_children=true.
var childData []shareChildData
if shareLink.IncludeChildren {
	children, err := s.docs.ListChildren(doc.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	for _, child := range children {
		childToken, cerr := s.shares.GetActiveShareForDocument(child.ID)
		if cerr != nil || childToken == "" {
			continue // only list children that already have an explicit share
		}
		childIcon := child.Icon
		if childIcon == "" {
			childIcon = "📄"
		}
		childData = append(childData, shareChildData{
			Title:         child.Title,
			Icon:          childIcon,
			IconIsBoxicon: isBoxicon(childIcon),
			URL:           base + "public/" + childToken,
		})
	}
}
```

A variável `childData` não é mais declarada antes deste bloco — remover a declaração `var childData []shareChildData` original se existir separada.

- [ ] **Step 3: Verificar compilação**

```bash
cd D:/git/pkd && go build ./...
```

Esperado: sem erros.

- [ ] **Step 4: Commit**

```bash
cd D:/git/pkd && git add internal/server/handlers_share.go
git commit -m "feat(share): handleCreateShare reads include_children; public page respects flag"
```

---