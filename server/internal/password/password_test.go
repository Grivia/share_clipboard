package password

import "testing"

func TestHashAndVerify(t *testing.T) {
	hash, err := Hash("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !Verify(hash, "correct horse battery staple") {
		t.Fatal("correct password was rejected")
	}
	if Verify(hash, "wrong password") {
		t.Fatal("wrong password was accepted")
	}
}

func TestVerifyRejectsInvalidEncoding(t *testing.T) {
	if Verify("not-a-password-hash", "anything") {
		t.Fatal("invalid hash was accepted")
	}
}
