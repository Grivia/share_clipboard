package api

import (
	"net/http/httptest"
	"testing"

	"fastcopy/server/internal/model"
)

func TestValidateAuthRequestAcceptsBroadAccountAndPassword(t *testing.T) {
	request := authRequest{
		Account:  "  用户 Name+1  ",
		Password: "短 密码",
		Device: model.DeviceInput{
			InstallationID: "55b5196d-6db5-47c4-9978-f24edccf4643",
			ReportedName:   "Test device",
			Platform:       "macos",
		},
	}
	recorder := httptest.NewRecorder()
	if !validateAuthRequest(recorder, &request) {
		t.Fatalf("request was rejected: %s", recorder.Body.String())
	}
	if request.Account != "用户 Name+1" {
		t.Fatalf("account was normalized to %q", request.Account)
	}
}

func TestValidateAuthRequestRejectsControlCharacterInAccount(t *testing.T) {
	request := authRequest{
		Account:  "user\nname",
		Password: "password",
		Device: model.DeviceInput{
			InstallationID: "55b5196d-6db5-47c4-9978-f24edccf4643",
			ReportedName:   "Test device",
			Platform:       "android",
		},
	}
	recorder := httptest.NewRecorder()
	if validateAuthRequest(recorder, &request) {
		t.Fatal("account containing a control character was accepted")
	}
	if recorder.Code != 400 {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}
