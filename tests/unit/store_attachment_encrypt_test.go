package unit_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/edalcin/pkd/internal/security"
	"github.com/edalcin/pkd/internal/storage"
	"github.com/edalcin/pkd/internal/store"
)

// TestAttachmentEncryptRoundTrip exercises the at-rest cipher applied to the
// files of a protected document: after EncryptDocumentFiles the bytes sitting
// in the storage backend must no longer be the plaintext, ReadPlaintext must
// still return the original content, and DecryptDocumentFiles must restore the
// backend blob. size_bytes and content_sha256 keep describing the plaintext
// throughout, because backup/migration verification depends on that.
func TestAttachmentEncryptRoundTrip(t *testing.T) {
	db, err := store.Open("file:store_attachment_encrypt_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	docs := store.NewDocumentStore(db)
	doc, err := docs.Create(nil, "Doc Com Anexo Protegido")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	dir := t.TempDir()
	local := storage.NewLocal(dir)
	atts := store.NewAttachmentStore(db, local, nil)

	ctx := context.Background()
	plain := []byte("conteudo secreto do anexo\x00\x01binario")
	att, err := atts.CreateFile(ctx, local, doc.ID, "segredo.bin", "application/octet-stream", "", bytes.NewReader(plain), 1<<20)
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}
	sizeBefore, shaBefore := att.SizeBytes, att.ContentSHA256

	onDisk := filepath.Join(dir, filepath.FromSlash(att.StoredFilename))
	if raw, err := os.ReadFile(onDisk); err != nil {
		t.Fatalf("read stored file: %v", err)
	} else if !bytes.Equal(raw, plain) {
		t.Fatalf("plaintext upload should be stored verbatim")
	}

	key := security.DeriveDocKey("master-password")

	n, err := atts.EncryptDocumentFiles(ctx, doc.ID, key)
	if err != nil || n != 1 {
		t.Fatalf("EncryptDocumentFiles = (%d, %v), want (1, nil)", n, err)
	}

	raw, err := os.ReadFile(onDisk)
	if err != nil {
		t.Fatalf("read encrypted file: %v", err)
	}
	if bytes.Contains(raw, plain) {
		t.Fatalf("stored blob still contains plaintext after encryption")
	}

	fresh, err := atts.GetByID(att.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !fresh.Encrypted {
		t.Fatalf("attachment row not flagged encrypted")
	}
	if fresh.SizeBytes != sizeBefore || fresh.ContentSHA256 != shaBefore {
		t.Fatalf("size/sha must keep describing plaintext: got (%d, %s), want (%d, %s)",
			fresh.SizeBytes, fresh.ContentSHA256, sizeBefore, shaBefore)
	}

	got, err := atts.ReadPlaintext(ctx, fresh, key)
	if err != nil {
		t.Fatalf("ReadPlaintext: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("ReadPlaintext = %q, want %q", got, plain)
	}

	if _, err := atts.ReadPlaintext(ctx, fresh, security.DeriveDocKey("senha-errada")); err == nil {
		t.Fatalf("ReadPlaintext with the wrong key must fail")
	}

	// Encrypting twice is a no-op: the blob must stay decryptable.
	if _, err := atts.EncryptDocumentFiles(ctx, doc.ID, key); err != nil {
		t.Fatalf("second EncryptDocumentFiles: %v", err)
	}
	again, _ := atts.GetByID(att.ID)
	if got, err := atts.ReadPlaintext(ctx, again, key); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("double encryption corrupted the blob: %v", err)
	}

	if n, err := atts.DecryptDocumentFiles(ctx, doc.ID, key); err != nil || n != 1 {
		t.Fatalf("DecryptDocumentFiles = (%d, %v), want (1, nil)", n, err)
	}
	raw, err = os.ReadFile(onDisk)
	if err != nil {
		t.Fatalf("read decrypted file: %v", err)
	}
	if !bytes.Equal(raw, plain) {
		t.Fatalf("decrypted blob = %q, want %q", raw, plain)
	}
	final, _ := atts.GetByID(att.ID)
	if final.Encrypted {
		t.Fatalf("attachment row still flagged encrypted after decryption")
	}
}
