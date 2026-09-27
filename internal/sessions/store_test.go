package sessions

import "testing"

// TestDeleteAllExcept proves "Encerrar as outras sessões": with 3 sessions,
// deleting all but one leaves only that one in the store, still reachable
// via Get (i.e. still authenticated).
func TestDeleteAllExcept(t *testing.T) {
	s := New(60)
	a := s.Create("127.0.0.1")
	b := s.Create("127.0.0.1")
	c := s.Create("127.0.0.1")

	revoked := s.DeleteAllExcept(a.ID)
	if revoked != 2 {
		t.Fatalf("revoked = %d, want 2", revoked)
	}
	if _, ok := s.Get(a.ID); !ok {
		t.Fatalf("session A must still exist (caller stays authenticated)")
	}
	if _, ok := s.Get(b.ID); ok {
		t.Fatalf("session B must have been revoked")
	}
	if _, ok := s.Get(c.ID); ok {
		t.Fatalf("session C must have been revoked")
	}
}
