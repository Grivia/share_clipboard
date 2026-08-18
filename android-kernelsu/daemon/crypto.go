package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

const (
	keyDerivationVersion    = 1
	keyDerivationIterations = 210_000
)

func deriveSharedKey(account, password string) string {
	salt := sha256.Sum256([]byte("fastcopy:key-salt:v1|" + account))
	mac := hmac.New(sha256.New, []byte(password))
	_, _ = mac.Write(salt[:])
	_, _ = mac.Write([]byte{0, 0, 0, 1})
	u := mac.Sum(nil)
	key := append([]byte(nil), u...)

	for iteration := 1; iteration < keyDerivationIterations; iteration++ {
		mac.Reset()
		_, _ = mac.Write(u)
		u = mac.Sum(u[:0])
		for index := range key {
			key[index] ^= u[index]
		}
	}
	return base64.StdEncoding.EncodeToString(key)
}

func newUUID() (string, error) {
	value := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16],
	), nil
}

func encryptText(text, keyBase64, clientEventID string) (ClipUpload, error) {
	gcm, err := newGCM(keyBase64)
	if err != nil {
		return ClipUpload{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return ClipUpload{}, err
	}
	aad := []byte("fastcopy:v1|" + clientEventID + "|text/plain")
	ciphertext := gcm.Seal(nil, nonce, []byte(text), aad)
	return ClipUpload{
		ClientEventID: clientEventID,
		ContentType:   "text/plain",
		Algorithm:     "AES-256-GCM",
		Nonce:         base64.StdEncoding.EncodeToString(nonce),
		Ciphertext:    base64.StdEncoding.EncodeToString(ciphertext),
	}, nil
}

func decryptText(event ClipEvent, keyBase64 string) (string, error) {
	if event.ContentType != "text/plain" || event.Algorithm != "AES-256-GCM" {
		return "", fmt.Errorf("unsupported encryption envelope")
	}
	gcm, err := newGCM(keyBase64)
	if err != nil {
		return "", err
	}
	nonce, err := base64.StdEncoding.DecodeString(event.Nonce)
	if err != nil || len(nonce) != gcm.NonceSize() {
		return "", fmt.Errorf("invalid nonce")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(event.Ciphertext)
	if err != nil || len(ciphertext) < gcm.Overhead() {
		return "", fmt.Errorf("invalid ciphertext")
	}
	aad := []byte("fastcopy:v1|" + event.ClientEventID + "|text/plain")
	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return "", fmt.Errorf("decrypt clipboard event: %w", err)
	}
	return string(plaintext), nil
}

func newGCM(keyBase64 string) (cipher.AEAD, error) {
	key, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("shared key must be 32 bytes encoded as Base64")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func contentDigest(text string) [32]byte {
	return sha256.Sum256([]byte(text))
}
