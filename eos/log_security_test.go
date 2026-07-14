package eos

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestInitSessionLogCreatesPrivateUniqueFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	baseDir := filepath.Join(home, ".eos-tui")
	sessionsDir := filepath.Join(baseDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Chmod(baseDir, 0755); err != nil {
		t.Fatalf("Chmod(baseDir) error = %v", err)
	}
	if err := os.Chmod(sessionsDir, 0755); err != nil {
		t.Fatalf("Chmod(sessionsDir) error = %v", err)
	}

	first := initSessionLog()
	second := initSessionLog()
	if first == "" || second == "" {
		t.Fatalf("initSessionLog() returned empty path: first=%q second=%q", first, second)
	}
	if first == second {
		t.Fatalf("session paths collided: %q", first)
	}

	assertPathPermissions(t, baseDir, 0700)
	assertPathPermissions(t, sessionsDir, 0700)
	assertPathPermissions(t, first, 0600)
	assertPathPermissions(t, second, 0600)

	latestTarget, err := os.Readlink(filepath.Join(baseDir, "latest.log"))
	if err != nil {
		t.Fatalf("Readlink(latest.log) error = %v", err)
	}
	wantTarget := filepath.Join("sessions", filepath.Base(second))
	if latestTarget != wantTarget {
		t.Fatalf("latest.log target = %q, want %q", latestTarget, wantTarget)
	}
}

func TestSessionLogWritesEscapeControlCharacters(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "session.log")
	if err := os.WriteFile(logPath, nil, 0666); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chmod(logPath, 0666); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}

	client := &Client{sessionLogPath: logPath}
	client.LogCommand([]string{"eos", "attr", "line1\n[2099-01-01 00:00:00] forged\r\t\x1b\u2028\u202e\u2066"})
	client.logResponse(
		[]string{"eos", "bad\n[2099-01-01 00:00:01] forged"},
		[]byte("remote output\n[2099-01-01 00:00:02] forged\x00"),
		errors.New("exit failure\n[2099-01-01 00:00:03] forged"),
	)

	assertPathPermissions(t, logPath, 0600)
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(raw)
	for _, forbidden := range []string{"\r", "\t", "\x00", "\x1b", "\u2028", "\u202e", "\u2066"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("log contains unescaped control character %q: %q", forbidden, text)
		}
	}

	physicalLines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if len(physicalLines) != 3 {
		t.Fatalf("log contains %d physical lines, want 3: %q", len(physicalLines), text)
	}
	for _, line := range physicalLines {
		if strings.HasPrefix(line, "[2099-") {
			t.Fatalf("forged command-history row found: %q", line)
		}
	}
	if !strings.Contains(text, `\n[2099-01-01`) || !strings.Contains(text, `\u001b`) || !strings.Contains(text, `\u202e`) {
		t.Fatalf("expected escaped control characters in log: %q", text)
	}

	commands, err := client.SessionCommands(10)
	if err != nil {
		t.Fatalf("SessionCommands() error = %v", err)
	}
	if len(commands) != 1 {
		t.Fatalf("SessionCommands() returned %d rows, want 1: %q", len(commands), commands)
	}
}

func TestSessionLogConcurrentWritesRemainDistinct(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "session.log")
	if err := os.WriteFile(logPath, nil, 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &Client{sessionLogPath: logPath}
	const writes = 64
	var wg sync.WaitGroup
	for i := 0; i < writes; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			client.LogCommand([]string{"eos", fmt.Sprintf("command-%d", i)})
		}(i)
	}
	wg.Wait()

	commands, err := client.SessionCommands(writes + 1)
	if err != nil {
		t.Fatalf("SessionCommands() error = %v", err)
	}
	if len(commands) != writes {
		t.Fatalf("SessionCommands() returned %d rows, want %d", len(commands), writes)
	}

	seen := make(map[string]bool, writes)
	for _, command := range commands {
		for i := 0; i < writes; i++ {
			marker := fmt.Sprintf("command-%d", i)
			if strings.HasSuffix(command, marker) {
				seen[marker] = true
				break
			}
		}
	}
	if len(seen) != writes {
		t.Fatalf("found %d distinct commands, want %d", len(seen), writes)
	}
}

func assertPathPermissions(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("permissions for %q = %04o, want %04o", path, got, want)
	}
}
