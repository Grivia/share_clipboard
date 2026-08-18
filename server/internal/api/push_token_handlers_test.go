package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateAPNsToken(t *testing.T) {
	request := apnsTokenRequest{
		Token:       " ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789 ",
		Environment: "SANDBOX",
	}
	recorder := httptest.NewRecorder()
	if !validateAPNsToken(recorder, &request) {
		t.Fatalf("valid token rejected: %s", recorder.Body.String())
	}
	if request.Environment != "sandbox" || request.Token != strings.ToLower(strings.TrimSpace(request.Token)) {
		t.Fatalf("request was not normalized: %+v", request)
	}
}

func TestValidateAPNsTokenRejectsNonHex(t *testing.T) {
	request := apnsTokenRequest{Token: strings.Repeat("z", 64), Environment: "production"}
	recorder := httptest.NewRecorder()
	if validateAPNsToken(recorder, &request) {
		t.Fatal("non-hex token was accepted")
	}
}
