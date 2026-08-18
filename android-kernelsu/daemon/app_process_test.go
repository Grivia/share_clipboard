package main

import "testing"

func TestClipboardLaunchersPreferShellAndKeepSystemFallback(t *testing.T) {
	launchers := clipboardAppProcessLaunchers()
	want := []string{
		"shell-credential",
		"shell-su-uid",
		"shell-su-name",
		"system-credential",
		"system-su-uid",
	}
	if len(launchers) != len(want) {
		t.Fatalf("got %d launchers, want %d", len(launchers), len(want))
	}
	for index, name := range want {
		if launchers[index].name != name {
			t.Fatalf("launcher %d = %q, want %q", index, launchers[index].name, name)
		}
	}
}

func TestShellQuote(t *testing.T) {
	got := shellQuote("a'b c")
	if got != "'a'\\''b c'" {
		t.Fatalf("shellQuote returned %q", got)
	}
}
