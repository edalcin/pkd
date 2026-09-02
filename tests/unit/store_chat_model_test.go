package unit_test

import (
	"testing"

	"github.com/edalcin/pkd/internal/store"
)

// TestSettingsStore_ChatModel covers the three branches of the Chat model
// lookup that ADR-006 D9 and ADR-004 D7 pin down: absent key falls back to the
// compiled default, a whitelisted value round-trips, and a value persisted by
// an older build whose model left the whitelist falls back WITHOUT being
// rewritten in the DB (silently overwriting the administrator's setting is
// worse than ignoring it).
func TestSettingsStore_ChatModel(t *testing.T) {
	db, err := store.Open("file:store_chat_model_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	settings := store.NewSettingsStore(db)

	got, err := settings.ChatModel()
	if err != nil {
		t.Fatalf("ChatModel on empty store: %v", err)
	}
	if got != store.DefaultChatModel {
		t.Errorf("absent key: got %q, want default %q", got, store.DefaultChatModel)
	}

	if err := settings.SetChatModel(store.ChatModelPro); err != nil {
		t.Fatalf("SetChatModel: %v", err)
	}
	if got, _ := settings.ChatModel(); got != store.ChatModelPro {
		t.Errorf("round-trip: got %q, want %q", got, store.ChatModelPro)
	}

	// A model that left the whitelist, as models/gemini-1.5-flash did.
	if err := settings.Set("chat.model", "models/gemini-1.5-flash"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got, _ := settings.ChatModel(); got != store.DefaultChatModel {
		t.Errorf("stale model: got %q, want default %q", got, store.DefaultChatModel)
	}
	if raw, _ := settings.Get("chat.model"); raw != "models/gemini-1.5-flash" {
		t.Errorf("fallback must not rewrite the stored value, got %q", raw)
	}

	if store.IsValidChatModel("models/gemini-1.5-flash") {
		t.Error("IsValidChatModel accepted a dead model")
	}
}
