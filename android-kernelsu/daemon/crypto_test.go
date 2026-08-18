package main

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	eventID, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	upload, err := encryptText("跨设备文本", base64.StdEncoding.EncodeToString(key), eventID)
	if err != nil {
		t.Fatal(err)
	}
	text, err := decryptText(ClipEvent{
		ClientEventID: upload.ClientEventID,
		ContentType:   upload.ContentType,
		Algorithm:     upload.Algorithm,
		Nonce:         upload.Nonce,
		Ciphertext:    upload.Ciphertext,
	}, base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	if text != "跨设备文本" {
		t.Fatalf("unexpected plaintext %q", text)
	}
}

func TestKeyDerivationProtocolVector(t *testing.T) {
	key := deriveSharedKey("alice", "correct horse battery staple")
	if key != "dpMRWwaHgnInWXwAZC2TxG3GuJZGNbWhYCGNP5T6I2g=" {
		t.Fatalf("derived key = %q", key)
	}
}

func TestNewUUIDShape(t *testing.T) {
	value, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	if len(value) != 36 || strings.Count(value, "-") != 4 {
		t.Fatalf("unexpected UUID %q", value)
	}
}
