package ids

import "testing"

func TestUUID(t *testing.T) {
	first, err := UUID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := UUID()
	if err != nil {
		t.Fatal(err)
	}
	if !IsUUID(first) {
		t.Fatalf("generated invalid UUID %q", first)
	}
	if first == second {
		t.Fatal("generated duplicate UUIDs")
	}
}

func TestTokenDigest(t *testing.T) {
	token, digest, err := Token()
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 40 {
		t.Fatalf("token too short: %d", len(token))
	}
	if string(digest) != string(DigestString(token)) {
		t.Fatal("token digest does not match")
	}
}
