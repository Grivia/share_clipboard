package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"
)

type BridgeEventKind int

const (
	BridgeReady BridgeEventKind = iota
	BridgeInitial
	BridgeClipboard
)

type BridgeEvent struct {
	Kind BridgeEventKind
	Text string
}

var errClipboardUnavailable = errors.New("Android clipboard is temporarily unavailable")

type Bridge struct {
	jarPath      string
	events       chan BridgeEvent
	mu           sync.Mutex
	commandMu    sync.Mutex
	writer       *bufio.Writer
	setResults   chan error
	launchers    []appProcessLauncher
	nextLauncher int
}

func NewBridge(jarPath string) *Bridge {
	return &Bridge{
		jarPath:    jarPath,
		events:     make(chan BridgeEvent, 16),
		setResults: make(chan error, 1),
		launchers:  clipboardAppProcessLaunchers(),
	}
}

func (b *Bridge) Events() <-chan BridgeEvent {
	return b.events
}

func (b *Bridge) Run(ctx context.Context) {
	for ctx.Err() == nil {
		launcher := b.launchers[b.nextLauncher]
		ready, err := b.runProcess(ctx, launcher)
		if ctx.Err() != nil {
			return
		}
		log.Printf(
			"clipboard bridge exited (launcher=%s ready=%t): %v",
			launcher.name,
			ready,
			err,
		)
		b.nextLauncher = (b.nextLauncher + 1) % len(b.launchers)
		retryDelay := 2 * time.Second
		if b.nextLauncher == 0 {
			retryDelay = 15 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(retryDelay):
		}
	}
}

func (b *Bridge) Set(text string) error {
	b.commandMu.Lock()
	defer b.commandMu.Unlock()
	select {
	case <-b.setResults:
	default:
	}

	b.mu.Lock()
	if b.writer == nil {
		b.mu.Unlock()
		return fmt.Errorf("%w: bridge is not ready", errClipboardUnavailable)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	if _, err := fmt.Fprintf(b.writer, "SET\t%s\n", encoded); err != nil {
		b.mu.Unlock()
		return fmt.Errorf("%w: %v", errClipboardUnavailable, err)
	}
	if err := b.writer.Flush(); err != nil {
		b.mu.Unlock()
		return fmt.Errorf("%w: %v", errClipboardUnavailable, err)
	}
	b.mu.Unlock()

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case result := <-b.setResults:
		return result
	case <-timer.C:
		return fmt.Errorf("%w: bridge did not confirm the write", errClipboardUnavailable)
	}
}

func (b *Bridge) runProcess(ctx context.Context, launcher appProcessLauncher) (bool, error) {
	command, err := launcher.command(
		ctx,
		b.jarPath,
		"hair.zhy.fastcopy.ClipboardBridge",
	)
	if err != nil {
		return false, err
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		return false, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return false, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return false, err
	}
	if err := command.Start(); err != nil {
		return false, err
	}
	log.Printf("clipboard bridge starting (launcher=%s)", launcher.name)

	go copyBridgeErrors(stderr)
	b.mu.Lock()
	b.writer = bufio.NewWriter(stdin)
	b.mu.Unlock()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 512*1024)
	ready := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "READY" {
			ready = true
			log.Printf("clipboard bridge ready (launcher=%s)", launcher.name)
			b.emit(BridgeEvent{Kind: BridgeReady})
			continue
		}
		if line == "SET_OK" {
			b.emitSetResult(nil)
			continue
		}
		if strings.HasPrefix(line, "SET_RETRY\t") {
			reason := strings.TrimPrefix(line, "SET_RETRY\t")
			b.emitSetResult(fmt.Errorf("%w: %s", errClipboardUnavailable, reason))
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 || (parts[0] != "INIT" && parts[0] != "CLIP") {
			log.Printf("clipboard bridge sent an invalid line")
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			log.Printf("decode clipboard bridge event: %v", err)
			continue
		}
		kind := BridgeClipboard
		if parts[0] == "INIT" {
			kind = BridgeInitial
		}
		b.emit(BridgeEvent{Kind: kind, Text: string(decoded)})
	}

	b.mu.Lock()
	b.writer = nil
	b.mu.Unlock()
	b.emitSetResult(fmt.Errorf("%w: bridge exited", errClipboardUnavailable))
	_ = stdin.Close()
	waitErr := command.Wait()
	if scanErr := scanner.Err(); scanErr != nil {
		return ready, scanErr
	}
	return ready, waitErr
}

func (b *Bridge) emitSetResult(result error) {
	select {
	case b.setResults <- result:
	default:
	}
}

func (b *Bridge) emit(event BridgeEvent) {
	select {
	case b.events <- event:
	default:
		log.Printf("clipboard bridge event queue is full")
	}
}

func copyBridgeErrors(reader io.Reader) {
	forwardBridgeErrors(reader, func(line string) {
		log.Printf("clipboard bridge: %s", line)
	})
}

func forwardBridgeErrors(reader io.Reader, emit func(string)) {
	scanner := bufio.NewScanner(reader)
	suppressVendorTrace := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(
			line,
			"FileNotFoundException: /data/system/theme_config/theme_compatibility.xml",
		) {
			emit("MIUI theme compatibility file is absent; continuing")
			suppressVendorTrace = true
			continue
		}
		if suppressVendorTrace && isStackTraceContinuation(line) {
			continue
		}
		suppressVendorTrace = false
		emit(line)
	}
}

func isStackTraceContinuation(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "at ") ||
		strings.HasPrefix(trimmed, "Caused by:") ||
		strings.HasPrefix(trimmed, "...")
}
