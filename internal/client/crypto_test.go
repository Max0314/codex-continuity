package client

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	source := filepath.Join(dir, "source.bin")
	encrypted := filepath.Join(dir, "encrypted.ccx")
	decrypted := filepath.Join(dir, "decrypted.bin")
	payload := bytes.Repeat([]byte("Codex 工作接力\n"), 400_000)
	if err := os.WriteFile(source, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{0x42}, 32)
	if err := EncryptFile(source, encrypted, key); err != nil {
		t.Fatal(err)
	}
	if err := DecryptFile(encrypted, decrypted, key); err != nil {
		t.Fatal(err)
	}
	result, err := os.ReadFile(decrypted)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, result) {
		t.Fatal("decrypted payload does not match source")
	}
}

func TestDecryptRejectsWrongKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	source := filepath.Join(dir, "source.bin")
	encrypted := filepath.Join(dir, "encrypted.ccx")
	decrypted := filepath.Join(dir, "decrypted.bin")
	if err := os.WriteFile(source, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EncryptFile(source, encrypted, bytes.Repeat([]byte{1}, 32)); err != nil {
		t.Fatal(err)
	}
	if err := DecryptFile(encrypted, decrypted, bytes.Repeat([]byte{2}, 32)); err == nil {
		t.Fatal("expected decryption to fail with the wrong key")
	}
}
