package store

import (
	"bytes"
	"testing"

	"fastcopy/server/internal/model"
)

func TestClipRequestDigest(t *testing.T) {
	upload := model.ClipUpload{
		ClientEventID: "55b5196d-6db5-47c4-9978-f24edccf4643",
		ContentType:   "text/plain",
		Algorithm:     "AES-256-GCM",
		Nonce:         bytes.Repeat([]byte{1}, 12),
		Ciphertext:    bytes.Repeat([]byte{2}, 32),
	}
	first := ClipRequestDigest(upload)
	second := ClipRequestDigest(upload)
	if !bytes.Equal(first, second) {
		t.Fatal("same upload produced different digests")
	}
	upload.Ciphertext[0] = 3
	if bytes.Equal(first, ClipRequestDigest(upload)) {
		t.Fatal("different upload produced the same digest")
	}
}
