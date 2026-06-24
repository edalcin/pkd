package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"time"

	"github.com/edalcin/pkd/internal/model"
)

const (
	semanticBodyChars    = 800
	semanticBatchSize    = 100
	semanticMaxNeighbors = 8
	semanticSimThreshold = 0.60
)

// semCandidate is a scored edge candidate used during similarity computation.
type semCandidate struct {
	a, b int64
	sim  float32
}

// EmbedStaleDocs (re)embeds every active document whose stored embedding is
// absent or stale (content or model changed) and upserts into document_embeddings.
// Returns count of (re)embedded docs. No-op (0, nil) when apiKey == "".
// Serialized by s.embedMu to avoid concurrent worker+graph-fetch API calls.
// ponytail: O(n) per sweep; batch of 100 to API. Fine for personal KB.
func (s *LinkStore) EmbedStaleDocs(ctx context.Context, apiKey string) (int, error) {
	if apiKey == "" {
		return 0, nil
	}
	s.embedMu.Lock()
	defer s.embedMu.Unlock()

	// 1. Load active docs.
	type doc struct {
		id   int64
		text string
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, SUBSTR(body_text, 1, 800) AS body
		FROM documents
		WHERE trashed_at IS NULL AND archived_at IS NULL
		ORDER BY id`)
	if err != nil {
		return 0, fmt.Errorf("embed: load docs: %w", err)
	}
	var docs []doc
	for rows.Next() {
		var id int64
		var title, body string
		if err := rows.Scan(&id, &title, &body); err != nil {
			rows.Close()
			return 0, fmt.Errorf("embed: scan doc: %w", err)
		}
		docs = append(docs, doc{id: id, text: title + "\n" + body})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("embed: docs rows: %w", err)
	}
	if len(docs) == 0 {
		return 0, nil
	}

	// 2. Hash includes model name: model change invalidates all, forces re-embed.
	// ponytail: fold model into hash (1 line) instead of new column — avoids
	// mixed-dimension vectors if model changes in future.
	hashes := make([]string, len(docs))
	for i, d := range docs {
		sum := sha256.Sum256([]byte(s.embedModel + "\x00" + d.text))
		hashes[i] = hex.EncodeToString(sum[:])
	}

	// 3. Read cached hashes (no need to fetch the blob here).
	cache := make(map[int64]string)
	crows, err := s.db.QueryContext(ctx, `SELECT document_id, content_hash FROM document_embeddings`)
	if err != nil {
		return 0, fmt.Errorf("embed: read cache: %w", err)
	}
	for crows.Next() {
		var id int64
		var h string
		if err := crows.Scan(&id, &h); err != nil {
			crows.Close()
			return 0, fmt.Errorf("embed: scan cache: %w", err)
		}
		cache[id] = h
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return 0, fmt.Errorf("embed: cache rows: %w", err)
	}

	// 4. Collect stale set.
	type stale struct {
		idx  int
		text string
	}
	var stales []stale
	for i, d := range docs {
		if h, ok := cache[d.id]; !ok || h != hashes[i] {
			stales = append(stales, stale{idx: i, text: d.text})
		}
	}
	// Prune embeddings for trashed/archived docs — excluded from graph, wasted space.
	// ponytail: single DELETE per sweep; always runs, non-fatal.
	_, _ = s.db.ExecContext(ctx, `
		DELETE FROM document_embeddings
		WHERE document_id NOT IN (
			SELECT id FROM documents WHERE trashed_at IS NULL AND archived_at IS NULL
		)`)
	if len(stales) == 0 {
		return 0, nil
	}

	// 5. Embed in batches and upsert.
	client := &http.Client{Timeout: 30 * time.Second}
	for start := 0; start < len(stales); start += semanticBatchSize {
		end := start + semanticBatchSize
		if end > len(stales) {
			end = len(stales)
		}
		batch := stales[start:end]
		texts := make([]string, len(batch))
		for i, st := range batch {
			texts[i] = st.text
		}
		vecs, err := embedBatch(ctx, client, apiKey, texts, s.embedModel)
		if err != nil {
			return 0, fmt.Errorf("embed: embedBatch: %w", err)
		}
		for i, item := range batch {
			blob := encodeEmbedding(vecs[i])
			if _, err := s.db.ExecContext(ctx, `
				INSERT INTO document_embeddings (document_id, content_hash, embedding, created_at)
				VALUES (?, ?, ?, datetime('now'))
				ON CONFLICT(document_id) DO UPDATE SET
				  content_hash=excluded.content_hash,
				  embedding=excluded.embedding,
				  created_at=excluded.created_at`,
				docs[item.idx].id, hashes[item.idx], blob); err != nil {
				return 0, fmt.Errorf("embed: upsert: %w", err)
			}
		}
	}
	return len(stales), nil
}

// GetSemanticEdges returns semantic similarity edges between active documents,
// using Gemini embeddings cached in SQLite.
// ponytail: O(n²·d) full scan, fine for personal KB; swap to ANN if doc count makes activation slow.
func (s *LinkStore) GetSemanticEdges(ctx context.Context, apiKey string) ([]model.GraphEdge, error) {
	// Fallback: ensure stale docs are embedded even if worker hasn't run yet.
	if _, err := s.EmbedStaleDocs(ctx, apiKey); err != nil {
		return nil, err
	}

	// Load (id, vector) for active docs that have an embedding.
	type vec struct {
		id   int64
		norm []float32
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.document_id, e.embedding
		FROM document_embeddings e
		JOIN documents d ON d.id = e.document_id
		WHERE d.trashed_at IS NULL AND d.archived_at IS NULL
		ORDER BY e.document_id`)
	if err != nil {
		return nil, fmt.Errorf("semantic: load vectors: %w", err)
	}
	var vecs []vec
	for rows.Next() {
		var id int64
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			rows.Close()
			return nil, fmt.Errorf("semantic: scan vector: %w", err)
		}
		n := normalize(decodeEmbedding(blob))
		if n == nil {
			continue
		}
		vecs = append(vecs, vec{id: id, norm: n})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("semantic: vector rows: %w", err)
	}
	if len(vecs) < 2 {
		return nonNilEdges(nil), nil
	}

	// Pairwise cosine similarity, top-8 per node, dedup by canonical pair.
	// ponytail: O(n²·d) full scan, fine for personal KB.
	topK := make(map[int64][]semCandidate, len(vecs))
	for i := 0; i < len(vecs); i++ {
		for j := i + 1; j < len(vecs); j++ {
			sim := dot(vecs[i].norm, vecs[j].norm)
			if sim < semanticSimThreshold {
				continue
			}
			c := semCandidate{a: vecs[i].id, b: vecs[j].id, sim: sim}
			topK[vecs[i].id] = insertTopK(topK[vecs[i].id], c, semanticMaxNeighbors)
			topK[vecs[j].id] = insertTopK(topK[vecs[j].id], c, semanticMaxNeighbors)
		}
	}
	type pair [2]int64
	seen := make(map[pair]float32)
	for _, cands := range topK {
		for _, c := range cands {
			lo, hi := c.a, c.b
			if lo > hi {
				lo, hi = hi, lo
			}
			if _, ok := seen[pair{lo, hi}]; !ok {
				seen[pair{lo, hi}] = c.sim
			}
		}
	}
	var edges []model.GraphEdge
	for k, sim := range seen {
		edges = append(edges, model.GraphEdge{Source: k[0], Target: k[1], EdgeType: "semantic", Weight: float64(sim)})
	}
	return nonNilEdges(edges), nil
}

// insertTopK inserts c into a top-k slice ordered descending by sim and prunes to maxK.
func insertTopK(s []semCandidate, c semCandidate, maxK int) []semCandidate {
	s = append(s, c)
	// Insertion sort (small k).
	for i := len(s) - 1; i > 0 && s[i].sim > s[i-1].sim; i-- {
		s[i], s[i-1] = s[i-1], s[i]
	}
	if len(s) > maxK {
		s = s[:maxK]
	}
	return s
}

// embedBatch calls the Gemini batchEmbedContents API and returns one vector per text.
func embedBatch(ctx context.Context, client *http.Client, apiKey string, texts []string, model string) ([][]float32, error) {
	type embPart struct {
		Text string `json:"text"`
	}
	type embContent struct {
		Parts []embPart `json:"parts"`
	}
	type embReq struct {
		Model   string     `json:"model"`
		Content embContent `json:"content"`
	}
	type reqBody struct {
		Requests []embReq `json:"requests"`
	}
	reqs := make([]embReq, len(texts))
	for i, t := range texts {
		reqs[i] = embReq{
			Model:   model,
			Content: embContent{Parts: []embPart{{Text: t}}},
		}
	}
	body, err := json.Marshal(reqBody{Requests: reqs})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://generativelanguage.googleapis.com/v1beta/"+model+":batchEmbedContents?key="+url.QueryEscape(apiKey),
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("gemini embed: status %d: %s", resp.StatusCode, snippet)
	}
	var out struct {
		Embeddings []struct {
			Values []float32 `json:"values"`
		} `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("gemini embed: decode: %w", err)
	}
	if len(out.Embeddings) != len(texts) {
		return nil, fmt.Errorf("gemini embed: got %d embeddings for %d texts", len(out.Embeddings), len(texts))
	}
	vecs := make([][]float32, len(texts))
	for i, e := range out.Embeddings {
		vecs[i] = e.Values
	}
	return vecs, nil
}

// encodeEmbedding serializes a float32 slice as little-endian bytes.
func encodeEmbedding(v []float32) []byte {
	var buf bytes.Buffer
	for _, f := range v {
		_ = binary.Write(&buf, binary.LittleEndian, f)
	}
	return buf.Bytes()
}

// decodeEmbedding deserializes a little-endian byte slice into float32 slice.
func decodeEmbedding(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	_ = binary.Read(bytes.NewReader(b), binary.LittleEndian, v)
	return v
}

// normalize returns a unit-length copy of v. Returns nil if norm is zero.
func normalize(v []float32) []float32 {
	var sum float64
	for _, f := range v {
		sum += float64(f) * float64(f)
	}
	if sum == 0 {
		return nil
	}
	inv := float32(1.0 / math.Sqrt(sum))
	out := make([]float32, len(v))
	for i, f := range v {
		out[i] = f * inv
	}
	return out
}

// dot computes the dot product of two equal-length float32 slices.
func dot(a, b []float32) float32 {
	var s float32
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}
