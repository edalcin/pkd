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
	geminiEmbedURL       = "https://generativelanguage.googleapis.com/v1beta/models/gemini-embedding-001:batchEmbedContents"
	geminiEmbedModel     = "models/gemini-embedding-001"
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

// GetSemanticEdges returns semantic similarity edges between active documents,
// using Gemini embeddings cached in SQLite.
// ponytail: O(n²·d) full scan, fine for personal KB; swap to ANN if doc count makes activation slow.
func (s *LinkStore) GetSemanticEdges(ctx context.Context, apiKey string) ([]model.GraphEdge, error) {
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
		return nil, fmt.Errorf("semantic: load docs: %w", err)
	}
	var docs []doc
	for rows.Next() {
		var id int64
		var title, body string
		if err := rows.Scan(&id, &title, &body); err != nil {
			rows.Close()
			return nil, fmt.Errorf("semantic: scan doc: %w", err)
		}
		docs = append(docs, doc{id: id, text: title + "\n" + body})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("semantic: docs rows: %w", err)
	}
	if len(docs) < 2 {
		return nonNilEdges(nil), nil
	}

	// 2. Content hashes (over the already-truncated text).
	hashes := make([]string, len(docs))
	for i, d := range docs {
		sum := sha256.Sum256([]byte(d.text))
		hashes[i] = hex.EncodeToString(sum[:])
	}

	// 3. Read cache.
	type cached struct {
		hash string
		vec  []float32
	}
	cache := make(map[int64]cached)
	crows, err := s.db.QueryContext(ctx, `SELECT document_id, content_hash, embedding FROM document_embeddings`)
	if err != nil {
		return nil, fmt.Errorf("semantic: read cache: %w", err)
	}
	for crows.Next() {
		var id int64
		var hash string
		var blob []byte
		if err := crows.Scan(&id, &hash, &blob); err != nil {
			crows.Close()
			return nil, fmt.Errorf("semantic: scan cache: %w", err)
		}
		cache[id] = cached{hash: hash, vec: decodeEmbedding(blob)}
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return nil, fmt.Errorf("semantic: cache rows: %w", err)
	}

	// 4. Determine stale set.
	type stale struct {
		idx  int
		text string
	}
	var stales []stale
	for i, d := range docs {
		c, ok := cache[d.id]
		if !ok || c.hash != hashes[i] {
			stales = append(stales, stale{idx: i, text: d.text})
		}
	}

	// 5. Call Gemini in batches for stale docs.
	if len(stales) > 0 {
		client := &http.Client{Timeout: 30 * time.Second}
		for start := 0; start < len(stales); start += semanticBatchSize {
			end := start + semanticBatchSize
			if end > len(stales) {
				end = len(stales)
			}
			batch := stales[start:end]
			texts := make([]string, len(batch))
			for i, s := range batch {
				texts[i] = s.text
			}
			vecs, err := embedBatch(ctx, client, apiKey, texts)
			if err != nil {
				return nil, fmt.Errorf("semantic: embedBatch: %w", err)
			}
			for i, item := range batch {
				d := docs[item.idx]
				blob := encodeEmbedding(vecs[i])
				_, err := s.db.ExecContext(ctx, `
					INSERT INTO document_embeddings (document_id, content_hash, embedding, created_at)
					VALUES (?, ?, ?, datetime('now'))
					ON CONFLICT(document_id) DO UPDATE SET
					  content_hash=excluded.content_hash,
					  embedding=excluded.embedding,
					  created_at=excluded.created_at`,
					d.id, hashes[item.idx], blob)
				if err != nil {
					return nil, fmt.Errorf("semantic: upsert embedding: %w", err)
				}
				cache[d.id] = cached{hash: hashes[item.idx], vec: vecs[i]}
			}
		}
	}

	// 6. Normalize + compute pairwise cosine similarity, top-8 per node.
	// ponytail: O(n²·d) full scan, fine for personal KB; swap to ANN if doc count makes activation slow.
	topK := make(map[int64][]semCandidate, len(docs))
	norms := make([][]float32, len(docs))
	for i, d := range docs {
		c, ok := cache[d.id]
		if !ok {
			continue
		}
		norms[i] = normalize(c.vec)
	}

	for i := 0; i < len(docs); i++ {
		if norms[i] == nil {
			continue
		}
		for j := i + 1; j < len(docs); j++ {
			if norms[j] == nil {
				continue
			}
			sim := dot(norms[i], norms[j])
			if sim < semanticSimThreshold {
				continue
			}
			idA, idB := docs[i].id, docs[j].id
			c := semCandidate{a: idA, b: idB, sim: sim}
			topK[idA] = insertTopK(topK[idA], c, semanticMaxNeighbors)
			topK[idB] = insertTopK(topK[idB], c, semanticMaxNeighbors)
		}
	}

	// 7. Collect unique pairs from top-k sets.
	type pair [2]int64
	seen := make(map[pair]float32)
	for _, cands := range topK {
		for _, c := range cands {
			lo, hi := c.a, c.b
			if lo > hi {
				lo, hi = hi, lo
			}
			k := pair{lo, hi}
			if _, ok := seen[k]; !ok {
				seen[k] = c.sim
			}
		}
	}

	var edges []model.GraphEdge
	for k, sim := range seen {
		edges = append(edges, model.GraphEdge{
			Source:   k[0],
			Target:   k[1],
			EdgeType: "semantic",
			Weight:   float64(sim),
		})
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
func embedBatch(ctx context.Context, client *http.Client, apiKey string, texts []string) ([][]float32, error) {
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
			Model:   geminiEmbedModel,
			Content: embContent{Parts: []embPart{{Text: t}}},
		}
	}
	body, err := json.Marshal(reqBody{Requests: reqs})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		geminiEmbedURL+"?key="+url.QueryEscape(apiKey),
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
