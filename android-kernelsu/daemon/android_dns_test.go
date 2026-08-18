package main

import "testing"

func TestParseResolvedIPs(t *testing.T) {
	addresses := parseResolvedIPs("203.0.113.10\n2001:db8::10\nwarning\n")
	if len(addresses) != 2 {
		t.Fatalf("parsed %d addresses, want 2", len(addresses))
	}
	if addresses[0].String() != "203.0.113.10" || addresses[1].String() != "2001:db8::10" {
		t.Fatalf("unexpected addresses: %v", addresses)
	}
}
