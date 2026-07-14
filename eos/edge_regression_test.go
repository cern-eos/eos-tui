package eos

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUnmarshalEOSJSONRejectsLeadingErrorAnnotation(t *testing.T) {
	input := []byte("* error: authorization denied\n{\"result\":[],\"retc\":\"0\"}\n")
	var payload map[string]any
	if err := unmarshalEOSJSON(input, &payload); err == nil || !strings.Contains(err.Error(), "authorization denied") {
		t.Fatalf("unmarshalEOSJSON() error = %v, want leading EOS error", err)
	}
}

func TestUnmarshalEOSJSONRejectsLeadingErrorEvenWithNonEmptyPayload(t *testing.T) {
	input := []byte("* error: authorization denied\n{\"result\":[{\"name\":\"should-not-be-trusted\"}],\"retc\":\"0\"}\n")
	var payload map[string]any
	if err := unmarshalEOSJSON(input, &payload); err == nil || !strings.Contains(err.Error(), "authorization denied") {
		t.Fatalf("unmarshalEOSJSON() error = %v, want leading authorization error", err)
	}
}

func TestUnmarshalEOSJSONTrustsNonEmptyResultAfterNoisyLeadingError(t *testing.T) {
	input := []byte("* error: cannot connect to localhost\n{\"result\":[{\"hostport\":\"fst01:1095\"}],\"retc\":\"0\"}\n")
	var payload struct {
		Result []struct {
			HostPort string `json:"hostport"`
		} `json:"result"`
	}
	if err := unmarshalEOSJSON(input, &payload); err != nil {
		t.Fatalf("unmarshalEOSJSON() rejected valid fallback result: %v", err)
	}
	if len(payload.Result) != 1 || payload.Result[0].HostPort != "fst01:1095" {
		t.Fatalf("unmarshalEOSJSON() payload = %+v", payload)
	}
}

func TestUnmarshalEOSJSONTrustsEmptyResultAfterKnownFallbackNoise(t *testing.T) {
	input := []byte("* error: cannot connect to localhost\n{\"result\":[],\"retc\":\"0\"}\n")
	var payload struct {
		Result []json.RawMessage `json:"result"`
	}
	if err := unmarshalEOSJSON(input, &payload); err != nil {
		t.Fatalf("unmarshalEOSJSON() rejected valid empty fallback result: %v", err)
	}
	if payload.Result == nil || len(payload.Result) != 0 {
		t.Fatalf("unmarshalEOSJSON() payload = %+v", payload)
	}
}

func TestUnmarshalEOSJSONFallbackNoiseRequiresExplicitSuccessStatus(t *testing.T) {
	input := []byte("* error: cannot connect to localhost\n{}\n")
	var payload map[string]any
	if err := unmarshalEOSJSON(input, &payload); err == nil || !strings.Contains(err.Error(), "cannot connect") {
		t.Fatalf("unmarshalEOSJSON() error = %v, want missing-retc fallback error", err)
	}
}

func TestUnmarshalEOSJSONAllowsInformationalAnnotations(t *testing.T) {
	input := []byte("* info: using cached view\n{\"result\":[{\"name\":\"default\"}],\"retc\":0}\n* info: completed\n")
	var payload struct {
		Result []struct {
			Name string `json:"name"`
		} `json:"result"`
	}
	if err := unmarshalEOSJSON(input, &payload); err != nil {
		t.Fatalf("unmarshalEOSJSON() error = %v", err)
	}
	if len(payload.Result) != 1 || payload.Result[0].Name != "default" {
		t.Fatalf("unmarshalEOSJSON() payload = %+v", payload)
	}
}

func TestRunCommandOnHostKeepsOriginalGatewayAfterDiscovery(t *testing.T) {
	runner := &recordingRunner{out: []byte("OK\n")}
	client := &Client{
		sshTarget:         "cluster-gateway",
		resolvedSSHTarget: "root@private-mgm.cern.ch",
		timeout:           time.Second,
		runner:            runner,
	}

	if _, err := client.runCommandOnHost(context.Background(), "qdb01.cern.ch", "redis-cli", "PING"); err != nil {
		t.Fatalf("runCommandOnHost() error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.calls))
	}
	args := strings.Join(runner.calls[0].args, " ")
	if !strings.Contains(args, "-J cluster-gateway") || !strings.Contains(args, "root@qdb01.cern.ch") {
		t.Fatalf("command route = %v, want destination through original gateway", runner.calls[0].args)
	}
	if strings.Contains(args, "-J root@private-mgm.cern.ch") {
		t.Fatalf("command incorrectly routes through discovered MGM: %v", runner.calls[0].args)
	}
}

func TestTailLogOnHostKeepsOriginalGatewayAfterDiscovery(t *testing.T) {
	runner := &recordingRunner{out: []byte("line\n")}
	client := &Client{
		sshTarget:         "cluster-gateway",
		resolvedSSHTarget: "root@private-mgm.cern.ch",
		timeout:           time.Second,
		runner:            runner,
	}

	if _, err := client.TailLogOnHost(context.Background(), "fst01.cern.ch", "/var/log/eos/fst/fst.log", 20); err != nil {
		t.Fatalf("TailLogOnHost() error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.calls))
	}
	args := strings.Join(runner.calls[0].args, " ")
	if !strings.Contains(args, "-J cluster-gateway") || !strings.Contains(args, "root@fst01.cern.ch") {
		t.Fatalf("tail route = %v, want FST through original gateway", runner.calls[0].args)
	}
}

func TestPeerCommandsLogTheirActualSSHRoute(t *testing.T) {
	for _, test := range []struct {
		name        string
		destination string
		run         func(*Client) error
	}{
		{
			name:        "command on QDB",
			destination: "root@qdb01.cern.ch",
			run: func(client *Client) error {
				_, err := client.runCommandOnHost(context.Background(), "qdb01.cern.ch", "redis-cli", "PING")
				return err
			},
		},
		{
			name:        "tail on FST",
			destination: "root@fst01.cern.ch",
			run: func(client *Client) error {
				_, err := client.TailLogOnHost(context.Background(), "fst01.cern.ch", "/var/log/eos/fst/fst.log", 20)
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "session.log")
			if err := os.WriteFile(logPath, nil, 0600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			client := &Client{
				sshTarget:         "cluster-gateway",
				resolvedSSHTarget: "root@private-mgm.cern.ch",
				sessionLogPath:    logPath,
				timeout:           time.Second,
				runner:            &recordingRunner{out: []byte("OK\n")},
			}

			if err := test.run(client); err != nil {
				t.Fatalf("peer command error = %v", err)
			}
			raw, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			logged := string(raw)
			if !strings.Contains(logged, "-J cluster-gateway") || !strings.Contains(logged, test.destination) {
				t.Fatalf("session log does not show actual peer route: %q", logged)
			}
			if strings.Contains(logged, "root@private-mgm.cern.ch") || strings.Contains(logged, "→") {
				t.Fatalf("session log shows misleading intermediate MGM route: %q", logged)
			}
		})
	}
}

func TestPostDiscoveryDoesNotJumpWhenOriginalTargetIsResolvedHost(t *testing.T) {
	runner := &recordingRunner{out: []byte("OK\n")}
	client := &Client{
		sshTarget:         "root@mgm01.cern.ch",
		resolvedSSHTarget: "root@mgm01.cern.ch",
		timeout:           time.Second,
		runner:            runner,
	}

	if _, err := client.runCommandContext(context.Background(), "eos", "version"); err != nil {
		t.Fatalf("runCommandContext() error = %v", err)
	}
	args := strings.Join(runner.calls[0].args, " ")
	if strings.Contains(args, " -J ") {
		t.Fatalf("same-host route contains self ProxyJump: %v", runner.calls[0].args)
	}
}

func TestPostDiscoveryDoesNotJumpFromShortNameToItsFQDN(t *testing.T) {
	runner := &recordingRunner{out: []byte("OK\n")}
	client := &Client{
		sshTarget:         "root@mgm01",
		resolvedSSHTarget: "root@mgm01.cern.ch",
		timeout:           time.Second,
		runner:            runner,
	}

	if _, err := client.runCommandContext(context.Background(), "eos", "version"); err != nil {
		t.Fatalf("runCommandContext() error = %v", err)
	}
	args := strings.Join(runner.calls[0].args, " ")
	if strings.Contains(args, " -J ") {
		t.Fatalf("short/FQDN same-host route contains self ProxyJump: %v", runner.calls[0].args)
	}
}

func TestQDBCoupArgsAndLegacyRedisErrors(t *testing.T) {
	wantArgs := "redis-cli -p 7777 raft-attempt-coup"
	if got := strings.Join(QDBCoupArgs(), " "); got != wantArgs {
		t.Fatalf("QDBCoupArgs() = %q, want %q", got, wantArgs)
	}

	for _, output := range []string{
		"(error) ERR quorum unavailable\n",
		"-ERR command failed\n",
		"NOAUTH Authentication required.\n",
		"CLUSTERDOWN The cluster is down\n",
		"MOVED 42 qdb02.cern.ch:7777\n",
	} {
		if got := redisCLIError([]byte(output)); got == "" {
			t.Errorf("redisCLIError(%q) did not detect server error", output)
		}
	}
	for _, output := range []string{"OK\n", "PONG\n", "leader elected\n"} {
		if got := redisCLIError([]byte(output)); got != "" {
			t.Errorf("redisCLIError(%q) = %q, want no error", output, got)
		}
	}
}

func TestInitSessionLogRejectsSymlinkedStateDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	external := filepath.Join(t.TempDir(), "external")
	if err := os.Mkdir(external, 0755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.Symlink(external, filepath.Join(home, ".eos-tui")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	if got := initSessionLog(); got != "" {
		t.Fatalf("initSessionLog() = %q, want logging disabled for symlinked state directory", got)
	}
	info, err := os.Stat(external)
	if err != nil {
		t.Fatalf("Stat(external) error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0755 {
		t.Fatalf("external directory permissions changed to %04o", got)
	}
}

func TestInitSessionLogAtomicallyReplacesStaleLatestFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	baseDir := filepath.Join(home, ".eos-tui")
	if err := os.Mkdir(baseDir, 0700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	latest := filepath.Join(baseDir, "latest.log")
	if err := os.WriteFile(latest, []byte("stale"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	sessionPath := initSessionLog()
	if sessionPath == "" {
		t.Fatal("initSessionLog() returned empty path")
	}
	target, err := os.Readlink(latest)
	if err != nil {
		t.Fatalf("Readlink(latest.log) error = %v", err)
	}
	want := filepath.Join("sessions", filepath.Base(sessionPath))
	if target != want {
		t.Fatalf("latest.log target = %q, want %q", target, want)
	}
}
