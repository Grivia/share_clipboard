package config

import "testing"

func TestMaxUsers(t *testing.T) {
	t.Setenv("FASTCOPY_DATABASE_URL", "postgres://example")
	t.Setenv("FASTCOPY_MAX_USERS", "1")
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if config.MaxUsers != 1 {
		t.Fatalf("MaxUsers = %d, want 1", config.MaxUsers)
	}
}

func TestMaxUsersRejectsNegativeValue(t *testing.T) {
	t.Setenv("FASTCOPY_DATABASE_URL", "postgres://example")
	t.Setenv("FASTCOPY_MAX_USERS", "-1")
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted a negative user limit")
	}
}
