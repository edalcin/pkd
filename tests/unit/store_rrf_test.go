package unit_test

import (
	"reflect"
	"testing"

	"github.com/edalcin/pkd/internal/store"
)

// TestFuseRRF covers Reciprocal Rank Fusion's contract: score-sum across
// legs, ascending-ID tiebreak (required for determinism against Go's
// randomized map iteration order), and the limit cutoff.
func TestFuseRRF(t *testing.T) {
	cases := []struct {
		name     string
		lexical  []int64
		semantic []int64
		limit    int
		want     []int64
	}{
		{
			name:     "shared hit outranks single-leg hits, tie broken by ascending id",
			lexical:  []int64{1, 2, 3},
			semantic: []int64{3, 4},
			limit:    10,
			want:     []int64{3, 1, 2, 4},
		},
		{
			name:     "empty semantic leg degrades to exact lexical order",
			lexical:  []int64{5, 6, 7},
			semantic: nil,
			limit:    10,
			want:     []int64{5, 6, 7},
		},
		{
			name:     "limit truncates the fused ranking",
			lexical:  []int64{1, 2, 3},
			semantic: []int64{3, 4},
			limit:    2,
			want:     []int64{3, 1},
		},
		{
			name:     "both legs empty yields empty result",
			lexical:  nil,
			semantic: nil,
			limit:    10,
			want:     []int64{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := store.FuseRRF(tc.lexical, tc.semantic, tc.limit)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("FuseRRF(%v, %v, %d) = %v, want %v", tc.lexical, tc.semantic, tc.limit, got, tc.want)
			}
		})
	}
}
