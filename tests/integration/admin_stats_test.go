package integration_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

type tagStatDTO struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	Active   int64  `json:"active"`
	Archived int64  `json:"archived"`
}

type rootStatDTO struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Icon     string `json:"icon,omitempty"`
	Active   int64  `json:"active"`
	Archived int64  `json:"archived"`
}

type adminStatsResponse struct {
	DocCount         int64         `json:"doc_count"`
	DocCountActive   int64         `json:"doc_count_active"`
	DocCountArchived int64         `json:"doc_count_archived"`
	FileCount        int64         `json:"file_count"`
	LinkCount        int64         `json:"link_count"`
	TagCount         int64         `json:"tag_count"`
	TagStats         []tagStatDTO  `json:"tag_stats"`
	RootStats        []rootStatDTO `json:"root_stats"`
}

func getAdminStats(t *testing.T, client *http.Client) adminStatsResponse {
	t.Helper()
	resp, err := client.Get(ts.URL + "/api/admin/stats")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin stats: want 200, got %d", resp.StatusCode)
	}
	var st adminStatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal("decode admin stats:", err)
	}
	return st
}

func findRootStat(stats []rootStatDTO, id int64) *rootStatDTO {
	for i := range stats {
		if stats[i].ID == id {
			return &stats[i]
		}
	}
	return nil
}

func findTagStat(stats []tagStatDTO, name string) *tagStatDTO {
	for i := range stats {
		if stats[i].Name == name {
			return &stats[i]
		}
	}
	return nil
}

func TestAdminStats(t *testing.T) {
	client := loginClient(t)

	baseline := getAdminStats(t, client)

	// Root document A
	resp := apiPost(t, client, "/api/documents", map[string]interface{}{
		"title": "Stats Root A",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create A: want 201, got %d", resp.StatusCode)
	}
	docA := decodeDoc(t, resp)
	idA := int64(docA["id"].(float64))

	// Child document B under A
	resp = apiPost(t, client, "/api/documents", map[string]interface{}{
		"parent_id": idA,
		"title":     "Stats Child B",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create B: want 201, got %d", resp.StatusCode)
	}
	docB := decodeDoc(t, resp)
	idB := int64(docB["id"].(float64))

	// Independent root document C
	resp = apiPost(t, client, "/api/documents", map[string]interface{}{
		"title": "Stats Root C",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create C: want 201, got %d", resp.StatusCode)
	}
	docC := decodeDoc(t, resp)
	idC := int64(docC["id"].(float64))

	// Archive B
	resp = apiPost(t, client, "/api/documents/"+itoa(idB)+"/archive", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("archive B: want 200, got %d", resp.StatusCode)
	}

	// Tags: A gets alpha; B gets alpha + beta
	resp = apiPut(t, client, "/api/documents/"+itoa(idA)+"/tags", map[string]interface{}{
		"tags": []string{"alpha"},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("tag A: want 204, got %d", resp.StatusCode)
	}

	resp = apiPut(t, client, "/api/documents/"+itoa(idB)+"/tags", map[string]interface{}{
		"tags": []string{"alpha", "beta"},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("tag B: want 204, got %d", resp.StatusCode)
	}

	after := getAdminStats(t, client)

	if delta := after.DocCountActive - baseline.DocCountActive; delta != 2 {
		t.Errorf("doc_count_active delta: want 2, got %d", delta)
	}
	if delta := after.DocCountArchived - baseline.DocCountArchived; delta != 1 {
		t.Errorf("doc_count_archived delta: want 1, got %d", delta)
	}

	rootA := findRootStat(after.RootStats, idA)
	if rootA == nil {
		t.Fatalf("root_stats: missing entry for root A (id %d)", idA)
	}
	if rootA.Active != 1 || rootA.Archived != 1 {
		t.Errorf("root_stats[A]: want active=1 archived=1, got active=%d archived=%d", rootA.Active, rootA.Archived)
	}

	rootC := findRootStat(after.RootStats, idC)
	if rootC == nil {
		t.Fatalf("root_stats: missing entry for root C (id %d)", idC)
	}
	if rootC.Active != 1 || rootC.Archived != 0 {
		t.Errorf("root_stats[C]: want active=1 archived=0, got active=%d archived=%d", rootC.Active, rootC.Archived)
	}

	alpha := findTagStat(after.TagStats, "alpha")
	if alpha == nil {
		t.Fatalf("tag_stats: missing entry for 'alpha'")
	}
	if alpha.Active != 1 || alpha.Archived != 1 {
		t.Errorf("tag_stats[alpha]: want active=1 archived=1, got active=%d archived=%d", alpha.Active, alpha.Archived)
	}

	beta := findTagStat(after.TagStats, "beta")
	if beta == nil {
		t.Fatalf("tag_stats: missing entry for 'beta'")
	}
	if beta.Active != 0 || beta.Archived != 1 {
		t.Errorf("tag_stats[beta]: want active=0 archived=1, got active=%d archived=%d", beta.Active, beta.Archived)
	}
}
