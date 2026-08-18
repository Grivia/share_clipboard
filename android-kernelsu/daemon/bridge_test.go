package main

import (
	"bufio"
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBridgeSetWaitsForWriteConfirmation(t *testing.T) {
	bridge := NewBridge("bridge.jar")
	var output bytes.Buffer
	bridge.writer = bufio.NewWriter(&output)
	done := make(chan error, 1)
	go func() {
		done <- bridge.Set("hello")
	}()
	waitForBridgeCommand(t, bridge, &output, "SET\taGVsbG8=\n")
	bridge.emitSetResult(nil)
	if err := <-done; err != nil {
		t.Fatalf("Set returned %v", err)
	}
}

func TestBridgeSetReturnsRetryableClipboardError(t *testing.T) {
	bridge := NewBridge("bridge.jar")
	var output bytes.Buffer
	bridge.writer = bufio.NewWriter(&output)
	done := make(chan error, 1)
	go func() {
		done <- bridge.Set("hello")
	}()
	waitForBridgeCommand(t, bridge, &output, "SET\taGVsbG8=\n")
	bridge.emitSetResult(errClipboardUnavailable)
	if err := <-done; !errors.Is(err, errClipboardUnavailable) {
		t.Fatalf("Set returned %v, want errClipboardUnavailable", err)
	}
}

func waitForBridgeCommand(
	t *testing.T,
	bridge *Bridge,
	output *bytes.Buffer,
	want string,
) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		bridge.mu.Lock()
		got := output.String()
		bridge.mu.Unlock()
		if got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for bridge command")
}

func TestForwardBridgeErrorsCollapsesKnownMIUITrace(t *testing.T) {
	input := strings.Join([]string{
		"java.io.FileNotFoundException: /data/system/theme_config/theme_compatibility.xml: ENOENT",
		"\tat libcore.io.IoBridge.open(IoBridge.java:574)",
		"Caused by: android.system.ErrnoException: ENOENT",
		"\t... 21 more",
		"clipboard identity: uid=2000 package=com.android.shell",
	}, "\n")
	var got []string
	forwardBridgeErrors(strings.NewReader(input), func(line string) {
		got = append(got, line)
	})
	want := []string{
		"MIUI theme compatibility file is absent; continuing",
		"clipboard identity: uid=2000 package=com.android.shell",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("forwarded lines = %#v, want %#v", got, want)
	}
}

func TestForwardBridgeErrorsPreservesUnknownTrace(t *testing.T) {
	input := "fatal: unknown\n\tat example.Main.main(Main.java:1)\n"
	var got []string
	forwardBridgeErrors(strings.NewReader(input), func(line string) {
		got = append(got, line)
	})
	want := []string{"fatal: unknown", "\tat example.Main.main(Main.java:1)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("forwarded lines = %#v, want %#v", got, want)
	}
}
