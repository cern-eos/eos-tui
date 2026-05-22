package eos

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type runnerCall struct {
	name string
	args []string
}

type recordingRunner struct {
	out   []byte
	err   error
	calls []runnerCall
}

func (r *recordingRunner) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.calls = append(r.calls, runnerCall{name: name, args: append([]string(nil), args...)})
	if r.err != nil {
		return r.out, r.err
	}
	return r.out, nil
}

type scriptedResponse struct {
	out []byte
	err error
}

type scriptedRunner struct {
	responses []scriptedResponse
	calls     []runnerCall
}

func (r *scriptedRunner) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.calls = append(r.calls, runnerCall{name: name, args: append([]string(nil), args...)})
	if len(r.responses) == 0 {
		return nil, errors.New("unexpected command")
	}
	response := r.responses[0]
	r.responses = r.responses[1:]
	return response.out, response.err
}

type deadlineRunner struct{}

func (deadlineRunner) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	<-ctx.Done()
	return nil, errors.New("signal: killed")
}

type contextErrRunner struct{}

func (contextErrRunner) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestParseLabeledValues(t *testing.T) {
	input := `
ALL      Files                            78 [booted] (0s)
ALL      Directories                      19
ALL      current file id                  7661
ALL      memory resident                  586.30 MB
`
	values := parseLabeledValues(input)

	if got := parseUint(values["Files"]); got != 78 {
		t.Fatalf("expected files=78, got %d", got)
	}
	if got := parseUint(values["Directories"]); got != 19 {
		t.Fatalf("expected directories=19, got %d", got)
	}
	if got := parseUint(values["current file id"]); got != 7661 {
		t.Fatalf("expected current file id=7661, got %d", got)
	}
	if got := parseHumanBytes(values["memory resident"]); got == 0 {
		t.Fatalf("expected parsed memory resident bytes, got 0")
	}
}

func TestEntryFromCLIContainer(t *testing.T) {
	entry := entryFromCLI(cliFileInfo{
		Name:           "eos",
		Path:           "/eos/",
		ID:             2,
		PID:            1,
		Inode:          2,
		UID:            0,
		GID:            0,
		Mode:           16893,
		TreeFiles:      78,
		TreeContainers: 17,
		TreeSize:       4907360263,
	})

	if entry.Kind != EntryKindContainer {
		t.Fatalf("expected container kind, got %q", entry.Kind)
	}
	if entry.Path != "/eos" {
		t.Fatalf("expected cleaned path /eos, got %q", entry.Path)
	}
	if entry.Files != 78 || entry.Containers != 17 {
		t.Fatalf("unexpected tree counts: files=%d containers=%d", entry.Files, entry.Containers)
	}
}

func TestEntryFromCLIFile(t *testing.T) {
	entry := entryFromCLI(cliFileInfo{
		Name:      "hola",
		Path:      "/eos/dev/test/hola",
		ID:        14,
		PID:       17,
		Inode:     9223372036854775822,
		UID:       0,
		GID:       0,
		Mode:      493,
		Size:      12,
		Locations: []cliLocation{{FSID: 3}},
	})

	if entry.Kind != EntryKindFile {
		t.Fatalf("expected file kind, got %q", entry.Kind)
	}
	if entry.Size != 12 || entry.Locations != 1 {
		t.Fatalf("unexpected file metadata: size=%d locations=%d", entry.Size, entry.Locations)
	}
}

func TestParseSpaceStatusLegacy(t *testing.T) {
	input := `
groupbalancer.threshold          := 5
groupmod                         := 24
lru                              := on
tgc.totalbytes                   := 1000000000000000000
`
	records := parseSpaceStatusLegacy([]byte(input))

	if len(records) != 4 {
		t.Fatalf("expected 4 records, got %d", len(records))
	}

	if records[0].Key != "groupbalancer.threshold" || records[0].Value != "5" {
		t.Fatalf("unexpected record 0: %+v", records[0])
	}
	if records[3].Key != "tgc.totalbytes" || records[3].Value != "1000000000000000000" {
		t.Fatalf("unexpected record 3: %+v", records[3])
	}
}

func TestParseSpaceStatusJSON(t *testing.T) {
	input := `{
  "result": [
    {
      "groupbalancer": {
        "threshold": 5
      },
      "groupmod": 24,
      "lru": {
        "status": "on"
      },
      "inspector": {
        "status": "off"
      },
      "tgc": {
        "totalbytes": 1000000000000000000
      }
    }
  ],
  "retc": "0"
}`

	records, err := parseSpaceStatusJSON([]byte(input))
	if err != nil {
		t.Fatalf("parseSpaceStatusJSON() error = %v", err)
	}

	got := make(map[string]string, len(records))
	for _, record := range records {
		got[record.Key] = record.Value
	}

	want := map[string]string{
		"groupbalancer.threshold": "5",
		"groupmod":                "24",
		"inspector.status":        "off",
		"lru.status":              "on",
		"tgc.totalbytes":          "1000000000000000000",
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d flattened records, got %d: %+v", len(want), len(got), got)
	}
	for key, wantValue := range want {
		if got[key] != wantValue {
			t.Fatalf("record %q = %q, want %q", key, got[key], wantValue)
		}
	}
}

func TestParseSpaceStatusJSONWithPreamble(t *testing.T) {
	input := "* info: connected\n" + `{
  "result": [
    {
      "space": {
        "converter": {
          "status": "on"
        }
      }
    }
  ]
}`

	records, err := parseSpaceStatusJSON([]byte(input))
	if err != nil {
		t.Fatalf("parseSpaceStatusJSON() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Key != "space.converter.status" || records[0].Value != "on" {
		t.Fatalf("unexpected record: %+v", records[0])
	}
}

func TestNamespaceStatsJSONConflict(t *testing.T) {
	// Simulated JSON with conflicting types for 'files' and 'directories'
	input := `
{
	"result": [
		{
			"ns": {
				"total": {
					"files": 78,
					"directories": 19
				}
			}
		},
		{
			"ns": {
				"total": {
					"files": {
						"changelog": { "size": 0 }
					}
				}
			}
		}
	],
	"retc": "0"
}
`
	var payload struct {
		Result []struct {
			NS struct {
				Total struct {
					Files       any `json:"files"`
					Directories any `json:"directories"`
				} `json:"total"`
			} `json:"ns"`
		} `json:"result"`
	}

	if err := json.Unmarshal([]byte(input), &payload); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(payload.Result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(payload.Result))
	}

	if val := toUint64(payload.Result[0].NS.Total.Files); val != 78 {
		t.Fatalf("expected result[0] files=78, got %v", val)
	}

	if val := toUint64(payload.Result[1].NS.Total.Files); val != 0 {
		t.Fatalf("expected result[1] files=0 (ignored object), got %v", val)
	}
}

func TestParseRaftInfoSingleNode(t *testing.T) {
	// Output from a single-node cluster (lobis-eos-dev / lobisapa-dev.cern.ch).
	input := `TERM 6
LOG-START 0
LOG-SIZE 1710314
LEADER lobisapa-dev.cern.ch:7777
CLUSTER-ID eosdev
COMMIT-INDEX 1710313
LAST-APPLIED 1710313
BLOCKED-WRITES 0
LAST-STATE-CHANGE 2302767 (26 days, 15 hours, 39 minutes, 27 seconds)
----------
MYSELF lobisapa-dev.cern.ch:7777
VERSION 5.4.0.1
STATUS LEADER
NODE-HEALTH GREEN
JOURNAL-FSYNC-POLICY sync-important-updates
----------
MEMBERSHIP-EPOCH 0
NODES lobisapa-dev.cern.ch:7777
OBSERVERS
QUORUM-SIZE 1
`
	info := parseRaftInfo([]byte(input))

	if info.Leader != "lobisapa-dev.cern.ch:7777" {
		t.Errorf("Leader: got %q, want lobisapa-dev.cern.ch:7777", info.Leader)
	}
	if info.Myself != "lobisapa-dev.cern.ch:7777" {
		t.Errorf("Myself: got %q, want lobisapa-dev.cern.ch:7777", info.Myself)
	}
	if info.MyRole != "leader" {
		t.Errorf("MyRole: got %q, want leader", info.MyRole)
	}
	if info.MyVersion != "5.4.0.1" {
		t.Errorf("MyVersion: got %q, want 5.4.0.1", info.MyVersion)
	}
	if info.MyHealth != "GREEN" {
		t.Errorf("MyHealth: got %q, want GREEN", info.MyHealth)
	}
	if len(info.Nodes) != 1 || info.Nodes[0] != "lobisapa-dev.cern.ch:7777" {
		t.Errorf("Nodes: got %v, want [lobisapa-dev.cern.ch:7777]", info.Nodes)
	}
	if len(info.Replicas) != 0 {
		t.Errorf("Replicas: got %d, want 0", len(info.Replicas))
	}
}

func TestParseRaftInfoSingleNodeNoLeaderLine(t *testing.T) {
	// Some single-node QDB deployments (e.g. the CI kind cluster) omit the
	// LEADER line entirely; the leader must be inferred from MYSELF+STATUS.
	input := `TERM 1
LOG-START 0
LOG-SIZE 42
CLUSTER-ID eos-ci
COMMIT-INDEX 41
LAST-APPLIED 41
BLOCKED-WRITES 0
LAST-STATE-CHANGE 100 (0 days, 0 hours, 1 minutes, 40 seconds)
----------
MYSELF eos-qdb-0.eos-qdb.default.svc.cluster.local:7777
VERSION 5.4.1
STATUS LEADER
NODE-HEALTH GREEN
----------
MEMBERSHIP-EPOCH 0
NODES eos-qdb-0.eos-qdb.default.svc.cluster.local:7777
OBSERVERS
QUORUM-SIZE 1
`
	info := parseRaftInfo([]byte(input))

	want := "eos-qdb-0.eos-qdb.default.svc.cluster.local:7777"
	if info.Leader != want {
		t.Errorf("Leader: got %q, want %q (should be inferred from MYSELF)", info.Leader, want)
	}
	if info.MyRole != "leader" {
		t.Errorf("MyRole: got %q, want leader", info.MyRole)
	}
	if info.Myself != want {
		t.Errorf("Myself: got %q, want %q", info.Myself, want)
	}
}

func TestParseRaftInfoMultiNode(t *testing.T) {
	// Output from a multi-node cluster (eospilot).
	input := `TERM 214
LOG-START 13282600000
LOG-SIZE 13332650191
LEADER eospilot-ns-02.cern.ch:7777
CLUSTER-ID 9cd69709-1dac-475e-bee6-86e4c9e1f286
COMMIT-INDEX 13332650190
LAST-APPLIED 13332650190
BLOCKED-WRITES 0
LAST-STATE-CHANGE 3431852 (1 months, 9 days, 17 hours, 17 minutes, 32 seconds)
----------
MYSELF eospilot-ns-02.cern.ch:7777
VERSION 5.3.29.1
STATUS LEADER
NODE-HEALTH GREEN
JOURNAL-FSYNC-POLICY sync-important-updates
----------
MEMBERSHIP-EPOCH 13091436169
NODES eospilot-ns-ip700.cern.ch:7777,eospilot-ns-01.cern.ch:7777,eospilot-ns-02.cern.ch:7777
OBSERVERS
QUORUM-SIZE 2
----------
REPLICA eospilot-ns-01.cern.ch:7777 | ONLINE | UP-TO-DATE | LOG-SIZE 13332650191 | VERSION 5.3.29.1
REPLICA eospilot-ns-ip700.cern.ch:7777 | ONLINE | UP-TO-DATE | LOG-SIZE 13332650191 | VERSION 5.3.29.1
`
	info := parseRaftInfo([]byte(input))

	if info.Leader != "eospilot-ns-02.cern.ch:7777" {
		t.Errorf("Leader: got %q, want eospilot-ns-02.cern.ch:7777", info.Leader)
	}
	if info.MyRole != "leader" {
		t.Errorf("MyRole: got %q, want leader", info.MyRole)
	}
	if info.MyVersion != "5.3.29.1" {
		t.Errorf("MyVersion: got %q, want 5.3.29.1", info.MyVersion)
	}
	if len(info.Nodes) != 3 {
		t.Errorf("Nodes count: got %d, want 3", len(info.Nodes))
	}
	if len(info.Replicas) != 2 {
		t.Fatalf("Replicas count: got %d, want 2", len(info.Replicas))
	}

	// Replicas should have the follower nodes (not the leader).
	rep0 := info.Replicas[0]
	if rep0.Host != "eospilot-ns-01.cern.ch:7777" {
		t.Errorf("Replicas[0].Host: got %q", rep0.Host)
	}
	if rep0.Status != "ONLINE" {
		t.Errorf("Replicas[0].Status: got %q, want ONLINE", rep0.Status)
	}
	if rep0.Version != "5.3.29.1" {
		t.Errorf("Replicas[0].Version: got %q, want 5.3.29.1", rep0.Version)
	}
}

func TestParseRaftInfoBuildsMGMRecords(t *testing.T) {
	// Verify that the raftInfo → MgmRecord mapping produces correct roles/versions.
	input := `LEADER eospilot-ns-02.cern.ch:7777
----------
MYSELF eospilot-ns-02.cern.ch:7777
VERSION 5.3.29.1
STATUS LEADER
NODE-HEALTH GREEN
----------
NODES eospilot-ns-ip700.cern.ch:7777,eospilot-ns-01.cern.ch:7777,eospilot-ns-02.cern.ch:7777
----------
REPLICA eospilot-ns-01.cern.ch:7777 | ONLINE | UP-TO-DATE | LOG-SIZE 100 | VERSION 5.3.29.1
REPLICA eospilot-ns-ip700.cern.ch:7777 | OFFLINE | LAGGING | LOG-SIZE 99 | VERSION 5.3.29.0
`
	info := parseRaftInfo([]byte(input))

	leaderHost := hostOnly(info.Leader)
	replicaMap := make(map[string]raftReplica)
	for _, r := range info.Replicas {
		replicaMap[hostOnly(r.Host)] = r
	}

	type want struct {
		role    string
		status  string
		version string
	}
	expectations := map[string]want{
		"eospilot-ns-02.cern.ch":    {role: "leader", status: "online", version: "5.3.29.1"},
		"eospilot-ns-01.cern.ch":    {role: "follower", status: "online", version: "5.3.29.1"},
		"eospilot-ns-ip700.cern.ch": {role: "follower", status: "offline", version: "5.3.29.0"},
	}

	for _, node := range info.Nodes {
		host := hostOnly(node)
		exp, ok := expectations[host]
		if !ok {
			t.Errorf("unexpected node %q", host)
			continue
		}

		role := "follower"
		if host == leaderHost {
			role = "leader"
		}
		if role != exp.role {
			t.Errorf("%s role: got %q, want %q", host, role, exp.role)
		}

		status := "online"
		version := ""
		if r, found := replicaMap[host]; found {
			if strings.ToUpper(r.Status) == "OFFLINE" {
				status = "offline"
			}
			version = r.Version
		}
		if host == leaderHost && info.MyVersion != "" {
			version = info.MyVersion
		}

		if status != exp.status {
			t.Errorf("%s status: got %q, want %q", host, status, exp.status)
		}
		if version != exp.version {
			t.Errorf("%s version: got %q, want %q", host, version, exp.version)
		}
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"eos -j node ls", "'eos -j node ls'"},
		{"it's a test", `'it'\''s a test'`},
		{"simple", "'simple'"},
	}
	for _, tt := range tests {
		if got := shellQuote(tt.input); got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestToUint64(t *testing.T) {
	tests := []struct {
		input any
		want  uint64
	}{
		{float64(78), 78},
		{uint64(100), 100},
		{int64(50), 50},
		{int(10), 10},
		{"not a number", 0},
		{map[string]any{"foo": "bar"}, 0},
	}

	for _, tt := range tests {
		if got := toUint64(tt.input); got != tt.want {
			t.Errorf("toUint64(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// ---- stripEOSPreamble tests ------------------------------------------------

func TestStripEOSPreambleNoPreamble(t *testing.T) {
	input := `{"result":[]}`
	got := string(stripEOSPreamble([]byte(input)))
	if got != input {
		t.Errorf("expected no change, got %q", got)
	}
}

func TestStripEOSPreambleArrayNoPreamble(t *testing.T) {
	input := `[{"id":"app1"}]`
	got := string(stripEOSPreamble([]byte(input)))
	if got != input {
		t.Errorf("expected no change for array, got %q", got)
	}
}

func TestStripEOSPreambleSingleStarLine(t *testing.T) {
	input := "* warning: something\n{\"result\":[]}"
	got := string(stripEOSPreamble([]byte(input)))
	if got != `{"result":[]}` {
		t.Errorf("expected preamble stripped, got %q", got)
	}
}

func TestStripEOSPreambleMultipleStarLines(t *testing.T) {
	input := "* info: connecting\n* warning: slow\n[{\"id\":\"x\"}]"
	got := string(stripEOSPreamble([]byte(input)))
	if got != `[{"id":"x"}]` {
		t.Errorf("expected all preamble lines stripped, got %q", got)
	}
}

func TestStripEOSPreamblePreservesTrailingContent(t *testing.T) {
	// A trailing * line after JSON should not truncate JSON.
	input := "{\"result\":[{\"name\":\"default\"}]}"
	got := string(stripEOSPreamble([]byte(input)))
	if !strings.Contains(got, "default") {
		t.Errorf("expected JSON content preserved, got %q", got)
	}
}

func TestStripEOSPreambleEmptyInput(t *testing.T) {
	got := stripEOSPreamble([]byte{})
	if len(got) != 0 {
		t.Errorf("expected empty output for empty input, got %q", got)
	}
}

// ---- JSON parsing with EOS preamble ----------------------------------------

func TestNodesParseWithPreamble(t *testing.T) {
	// Simulate EOS emitting a `* <msg>` line before the JSON payload.
	raw := `* error: cannot connect to localhost
{
  "errormsg": "",
  "result": [
    {
      "type": "nodesview",
      "hostport": "fst01:1095",
      "status": "online",
      "heartbeatdelta": 1,
      "nofs": 5,
      "cfg": {"status": "on", "stat": {"geotag": "local", "sys": {"kernel": "5.x", "rss": 1024, "threads": 50, "uptime": "10h", "vsize": 2048, "eos": {"version": "5.3.27"}}}},
      "avg": {"stat": {"disk": {"load": 0.1}}},
      "sum": {"stat": {"disk": {"readratemb": 1.5, "writeratemb": 2.0}, "statfs": {"capacity": 1000000, "freebytes": 500000, "usedbytes": 500000}, "usedfiles": 100}}
    }
  ]
}`
	var payload struct {
		Result []struct {
			HostPort string `json:"hostport"`
			NoFS     int    `json:"nofs"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stripEOSPreamble([]byte(raw)), &payload); err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(payload.Result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(payload.Result))
	}
	if payload.Result[0].HostPort != "fst01:1095" {
		t.Errorf("expected hostport fst01:1095, got %q", payload.Result[0].HostPort)
	}
}

func TestFileSystemsParseWithPreamble(t *testing.T) {
	raw := "* warning: fallback mode\n" + `{
  "errormsg": "",
  "result": [
    {
      "host": "fst01", "port": 1095, "id": 3,
      "path": "/data/fst.1/01",
      "schedgroup": "default.0",
      "configstatus": "rw",
      "local": {"drain": {"status": "nodrain"}},
      "stat": {"active": "online", "boot": "booted", "geotag": "local",
               "health": {"status": "ok"},
               "disk": {"bw": 0.0, "iops": 0.0, "readratemb": 0.0, "writeratemb": 0.0},
               "statfs": {"capacity": 1000, "freebytes": 500, "usedbytes": 500},
               "usedfiles": 10}
    }
  ]
}`
	var payload struct {
		Result []struct {
			ID   uint64 `json:"id"`
			Host string `json:"host"`
			Path string `json:"path"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stripEOSPreamble([]byte(raw)), &payload); err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(payload.Result) != 1 || payload.Result[0].Path != "/data/fst.1/01" {
		t.Errorf("unexpected result: %+v", payload.Result)
	}
}

func TestSpacesParseWithPreamble(t *testing.T) {
	raw := "* info: connected\n" + `{
  "errormsg": "",
  "result": [
    {
      "name": "default",
      "type": "groupbalancer",
      "cfg": {"groupsize": 24},
      "sum": {"n_rw": 2, "stat": {"statfs": {"capacity": 1000, "usedbytes": 400, "freebytes": 600, "files": 78}}}
    }
  ]
}`
	var payload struct {
		Result []struct {
			Name string `json:"name"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stripEOSPreamble([]byte(raw)), &payload); err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(payload.Result) != 1 || payload.Result[0].Name != "default" {
		t.Errorf("unexpected result: %+v", payload.Result)
	}
}

func TestSessionCommandsReturnsOnlyCommandLines(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "session.log")
	content := strings.Join([]string{
		"[2026-04-09 10:00:00] eos -j node ls",
		"[2026-04-09 10:00:01] ERROR (node ls): boom",
		"[2026-04-09 10:00:01]   output: failed",
		"[2026-04-09 10:00:02] ssh -o BatchMode=yes root@host 'eos -j fs ls'",
	}, "\n") + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	client := &Client{sessionLogPath: logPath}
	lines, err := client.SessionCommands(10)
	if err != nil {
		t.Fatalf("SessionCommands error: %v", err)
	}

	if got := strings.Join(lines, "\n"); strings.Contains(got, "ERROR") || strings.Contains(got, "output:") {
		t.Fatalf("expected errors/output lines to be filtered out, got:\n%s", got)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 command lines, got %d", len(lines))
	}
}

func TestSessionCommandsKeepsLastNEntries(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "session.log")
	content := strings.Join([]string{
		"[2026-04-09 10:00:00] cmd-1",
		"[2026-04-09 10:00:01] cmd-2",
		"[2026-04-09 10:00:02] cmd-3",
	}, "\n") + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	client := &Client{sessionLogPath: logPath}
	lines, err := client.SessionCommands(2)
	if err != nil {
		t.Fatalf("SessionCommands error: %v", err)
	}
	if got := strings.Join(lines, ","); got != "[2026-04-09 10:00:01] cmd-2,[2026-04-09 10:00:02] cmd-3" {
		t.Fatalf("unexpected tail result: %s", got)
	}
}

func TestSessionCommandsReadsAppendsIncrementally(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "session.log")
	if err := os.WriteFile(logPath, []byte("[2026-04-09 10:00:00] cmd-1\n"), 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	client := &Client{sessionLogPath: logPath}
	lines, err := client.SessionCommands(10)
	if err != nil {
		t.Fatalf("SessionCommands initial error: %v", err)
	}
	if got := strings.Join(lines, ","); got != "[2026-04-09 10:00:00] cmd-1" {
		t.Fatalf("unexpected initial lines: %s", got)
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open append: %v", err)
	}
	if _, err := f.WriteString("[2026-04-09 10:00:01] ERROR (cmd): boom\n[2026-04-09 10:00:02] cmd-2\n"); err != nil {
		_ = f.Close()
		t.Fatalf("append log: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close append: %v", err)
	}

	lines, err = client.SessionCommands(10)
	if err != nil {
		t.Fatalf("SessionCommands append error: %v", err)
	}
	if got := strings.Join(lines, ","); got != "[2026-04-09 10:00:00] cmd-1,[2026-04-09 10:00:02] cmd-2" {
		t.Fatalf("unexpected appended lines: %s", got)
	}
}

func TestRunCommandContextUsesCallerCancellation(t *testing.T) {
	runner := &recordingRunner{out: []byte("EOS_SERVER_VERSION=5.4.0\n")}
	client := &Client{timeout: time.Minute, runner: runner}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.EOSVersion(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner should not record completed command after cancellation, got %#v", runner.calls)
	}
}

func TestClientCloseCancelsRunningCommand(t *testing.T) {
	client, err := New(context.Background(), Config{Timeout: time.Minute})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	client.runner = contextErrRunner{}

	errCh := make(chan error, 1)
	go func() {
		_, err := client.runCommandContext(context.Background(), "eos", "version")
		errCh <- err
	}()

	client.Close()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("command did not stop after client close")
	}
}

func TestRunCommandContextUsesInjectedRunner(t *testing.T) {
	runner := &recordingRunner{out: []byte("EOS_SERVER_VERSION=5.4.0\n")}
	client := &Client{timeout: time.Minute, runner: runner}

	version, err := client.EOSVersion(context.Background())
	if err != nil {
		t.Fatalf("EOSVersion error: %v", err)
	}
	if version != "5.4.0" {
		t.Fatalf("version = %q, want 5.4.0", version)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 runner call, got %d", len(runner.calls))
	}
	if runner.calls[0].name != "eos" || strings.Join(runner.calls[0].args, " ") != "version" {
		t.Fatalf("unexpected runner call: %#v", runner.calls[0])
	}
}

func TestNamespaceStatsParseWithPreamble(t *testing.T) {
	raw := "* msg: ns booted\n" + `{
  "errormsg": "",
  "result": [
    {
      "master_id": "mgm01:1094",
      "ns": {
        "total": {"files": 78, "directories": 19},
        "current": {"fid": 7661, "cid": 1000},
        "generated": {"fid": 7662, "cid": 1001},
        "contention": {"read": 0.1, "write": 0.2},
        "cache": {
          "files": {"maxsize": 1000, "occupancy": 500, "requests": 200, "hits": 180},
          "containers": {"maxsize": 500, "occupancy": 250, "requests": 100, "hits": 90}
        }
      }
    }
  ]
}`
	var payload struct {
		Result []struct {
			Master string `json:"master_id"`
			NS     struct {
				Total struct {
					Files any `json:"files"`
				} `json:"total"`
			} `json:"ns"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stripEOSPreamble([]byte(raw)), &payload); err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(payload.Result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(payload.Result))
	}
	if payload.Result[0].Master != "mgm01:1094" {
		t.Errorf("expected master mgm01:1094, got %q", payload.Result[0].Master)
	}
	if v := toUint64(payload.Result[0].NS.Total.Files); v != 78 {
		t.Errorf("expected files=78, got %d", v)
	}
}

func TestParseNamespaceAttrs(t *testing.T) {
	input := `
* attr listing
sys.forced.layout="replica"
user.comment = "hello world"
sys.mask=755
`

	attrs := parseNamespaceAttrs([]byte(input))
	if len(attrs) != 3 {
		t.Fatalf("expected 3 attrs, got %d", len(attrs))
	}
	if attrs[0].Key != "sys.forced.layout" || attrs[0].Value != "replica" {
		t.Fatalf("unexpected first attr: %+v", attrs[0])
	}
	if attrs[1].Key != "sys.mask" || attrs[1].Value != "755" {
		t.Fatalf("unexpected second attr: %+v", attrs[1])
	}
	if attrs[2].Key != "user.comment" || attrs[2].Value != "hello world" {
		t.Fatalf("unexpected third attr: %+v", attrs[2])
	}
}

func TestShellJoinQuotesEveryArgument(t *testing.T) {
	got := shellJoin([]string{"eos", "attr", "set", "user.comment=hello world", "/eos/dev/file"})
	want := "'eos' 'attr' 'set' 'user.comment=hello world' '/eos/dev/file'"
	if got != want {
		t.Fatalf("shellJoin() = %q, want %q", got, want)
	}
}

func TestShellDisplayJoinKeepsSimpleArgsReadable(t *testing.T) {
	got := shellDisplayJoin([]string{"eos", "-j", "-b", "space", "ls"})
	want := "eos -j -b space ls"
	if got != want {
		t.Fatalf("shellDisplayJoin() = %q, want %q", got, want)
	}
}

func TestShellDisplayJoinQuotesOnlyUnsafeArgs(t *testing.T) {
	got := shellDisplayJoin([]string{"eos", "attr", "set", "user.comment=hello world", "/eos/dev/file"})
	want := "eos attr set 'user.comment=hello world' /eos/dev/file"
	if got != want {
		t.Fatalf("shellDisplayJoin() = %q, want %q", got, want)
	}
}

func TestSetAttrCommand(t *testing.T) {
	tests := []struct {
		name      string
		recursive bool
		want      string
	}{
		{
			name:      "non-recursive",
			recursive: false,
			want:      "eos attr set 'user.comment=hello world' /eos/dev/file",
		},
		{
			name:      "recursive",
			recursive: true,
			want:      "eos attr -r set 'user.comment=hello world' /eos/dev/file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellDisplayJoin(attrSetArgs("/eos/dev/file", "user.comment", "hello world", tt.recursive))
			if got != tt.want {
				t.Fatalf("SetAttr command = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAttrSetArgs(t *testing.T) {
	tests := []struct {
		name      string
		recursive bool
		want      []string
	}{
		{
			name:      "non-recursive",
			recursive: false,
			want:      []string{"eos", "attr", "set", "user.comment=hello world", "/eos/dev/file"},
		},
		{
			name:      "recursive",
			recursive: true,
			want:      []string{"eos", "attr", "-r", "set", "user.comment=hello world", "/eos/dev/file"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := attrSetArgs("/eos/dev/file", "user.comment", "hello world", tt.recursive)
			if strings.Join(got, "\x00") != strings.Join(tt.want, "\x00") {
				t.Fatalf("attrSetArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestMkdirRunsEOSMkdir(t *testing.T) {
	runner := &recordingRunner{}
	c := &Client{timeout: time.Second, runner: runner}

	if err := c.Mkdir(context.Background(), "/eos/test/new-dir"); err != nil {
		t.Fatalf("Mkdir() error: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one command, got %d", len(runner.calls))
	}
	call := runner.calls[0]
	if call.name != "eos" || strings.Join(call.args, " ") != "mkdir /eos/test/new-dir" {
		t.Fatalf("expected eos mkdir command, got %+v", call)
	}
}

func TestRTLogCommand(t *testing.T) {
	got := shellDisplayJoin([]string{"eos", "rtlog", "/eos/fst01.cern.ch:1095/fst", "600", "info"})
	want := "eos rtlog /eos/fst01.cern.ch:1095/fst 600 info"
	if got != want {
		t.Fatalf("shellDisplayJoin() = %q, want %q", got, want)
	}
}

func TestNormalizeClusterInstance(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "eospublic", want: "eospublic"},
		{input: "root@eospublic.cern.ch", want: "eospublic"},
		{input: "root@eospilot-ns-02.cern.ch", want: "eospilot"},
		{input: "eoshome-mgm", want: "eoshome"},
		{input: "local eos cli", want: ""},
		{input: "", want: ""},
	}

	for _, tt := range tests {
		if got := NormalizeClusterInstance(tt.input); got != tt.want {
			t.Fatalf("NormalizeClusterInstance(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseEOSServerVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "standard output",
			input: "EOS_INSTANCE=eosdev\nEOS_SERVER_VERSION=5.3.27 EOS_SERVER_RELEASE=unknown\nEOS_CLIENT_VERSION=5.3.27 EOS_CLIENT_RELEASE=unknown\n",
			want:  "5.3.27",
		},
		{
			name:  "with preamble star line",
			input: "* info: something\nEOS_SERVER_VERSION=5.4.1 EOS_SERVER_RELEASE=1\n",
			want:  "5.4.1",
		},
		{
			name:  "no version line",
			input: "EOS_INSTANCE=eosdev\n",
			want:  "",
		},
		{
			name:  "dash dash version output",
			input: "EOS 5.4.2 (2026)\n\nDeveloped by the CERN IT Storage Group\n",
			want:  "5.4.2",
		},
		{
			name:  "dash dash version older output",
			input: "EOS 5.4.0 (2020)\n\nDeveloped by the CERN IT storage group\n",
			want:  "5.4.0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEOSServerVersion([]byte(tt.input))
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseEOSServerBuildVersionIncludesRelease(t *testing.T) {
	input := "EOS_INSTANCE=eospilot\nEOS_SERVER_VERSION=5.4.2 EOS_SERVER_RELEASE=20260507225351gite07497239\n"
	if got := parseEOSServerBuildVersion([]byte(input)); got != "5.4.2-20260507225351gite07497239" {
		t.Fatalf("expected full build version, got %q", got)
	}
}

func TestParseEOSServerPackageVersionStripsRPMPlatformSuffix(t *testing.T) {
	input := "5.4.2-20260429174732git29f83ccc8.el9\n"
	if got := parseEOSServerPackageVersion([]byte(input)); got != "5.4.2-20260429174732git29f83ccc8" {
		t.Fatalf("expected package build version, got %q", got)
	}
}

func TestParseEOSServerPackageVersionSkipsSSHWarning(t *testing.T) {
	input := "* WARNING: connection is not using a post-quantum key exchange algorithm.\n" +
		"This session may be vulnerable to \"store now, decrypt later\" attacks.\n" +
		"5.4.2-20260429174732git29f83ccc8.el9\n"
	if got := parseEOSServerPackageVersion([]byte(input)); got != "5.4.2-20260429174732git29f83ccc8" {
		t.Fatalf("expected package build version after warning, got %q", got)
	}
}

func TestEOSVersionOnHostUsesPackageVersionFirst(t *testing.T) {
	runner := &scriptedRunner{responses: []scriptedResponse{
		{out: []byte("5.4.2-20260429174732git29f83ccc8.el9\n")},
	}}
	c := &Client{timeout: time.Second, runner: runner}

	got, err := c.eosVersionOnHost(context.Background(), "")
	if err != nil {
		t.Fatalf("eosVersionOnHost() error: %v", err)
	}
	if got != "5.4.2-20260429174732git29f83ccc8" {
		t.Fatalf("expected package build version, got %q", got)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one command, got %d", len(runner.calls))
	}
	if runner.calls[0].name != "rpm" || strings.Join(runner.calls[0].args, " ") != "-q --qf %{VERSION}-%{RELEASE}\\n eos-server" {
		t.Fatalf("expected rpm package probe first, got %+v", runner.calls[0])
	}
}

func TestEOSVersionOnHostFallsBackToEOSVersion(t *testing.T) {
	runner := &scriptedRunner{responses: []scriptedResponse{
		{out: []byte("package eos-server is not installed\n"), err: errors.New("exit status 1")},
		{out: []byte("EOS_INSTANCE=eospilot\nEOS_SERVER_VERSION=5.4.2 EOS_SERVER_RELEASE=20260507225351gite07497239\n")},
	}}
	c := &Client{timeout: time.Second, runner: runner}

	got, err := c.eosVersionOnHost(context.Background(), "")
	if err != nil {
		t.Fatalf("eosVersionOnHost() error: %v", err)
	}
	if got != "5.4.2-20260507225351gite07497239" {
		t.Fatalf("expected eos version build fallback, got %q", got)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("expected two commands, got %d", len(runner.calls))
	}
	if runner.calls[1].name != "eos" || strings.Join(runner.calls[1].args, " ") != "version" {
		t.Fatalf("expected eos version fallback, got %+v", runner.calls[1])
	}
}

func TestEOSVersionOnHostFallsBackToDashVersion(t *testing.T) {
	runner := &scriptedRunner{responses: []scriptedResponse{
		{out: []byte("package eos-server is not installed\n"), err: errors.New("exit status 1")},
		{out: []byte("error: MGM root://localhost not online/reachable\n"), err: errors.New("exit status 64")},
		{out: []byte("EOS 5.4.2 (2026)\n\nDeveloped by the CERN IT Storage Group\n")},
	}}
	c := &Client{timeout: time.Second, runner: runner}

	got, err := c.eosVersionOnHost(context.Background(), "")
	if err != nil {
		t.Fatalf("eosVersionOnHost() error: %v", err)
	}
	if got != "5.4.2" {
		t.Fatalf("expected short fallback version, got %q", got)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("expected three commands, got %d", len(runner.calls))
	}
	if runner.calls[2].name != "eos" || strings.Join(runner.calls[2].args, " ") != "--version" {
		t.Fatalf("expected eos --version final fallback, got %+v", runner.calls[2])
	}
}

func TestQDBVersionOnHostUsesRaftInfoVersion(t *testing.T) {
	runner := &scriptedRunner{responses: []scriptedResponse{
		{out: []byte(`LEADER eospilot-ns-02.cern.ch:7777
----------
MYSELF eospilot-ns-02.cern.ch:7777
VERSION 5.4.2.1
STATUS LEADER
----------
NODES eospilot-ns-02.cern.ch:7777
`)},
	}}
	c := &Client{timeout: time.Second, runner: runner}

	got, err := c.qdbVersionOnHost(context.Background(), "")
	if err != nil {
		t.Fatalf("qdbVersionOnHost() error: %v", err)
	}
	if got != "5.4.2.1" {
		t.Fatalf("expected raft-info QDB version, got %q", got)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one raft-info command, got %d commands", len(runner.calls))
	}
	if runner.calls[0].name != "redis-cli" || strings.Join(runner.calls[0].args, " ") != "-p 7777 raft-info" {
		t.Fatalf("expected redis-cli raft-info, got %+v", runner.calls[0])
	}
}

func TestSSHTargetForHostLocal(t *testing.T) {
	// No SSH target configured (running locally).
	c := &Client{}

	// No host selected — should return empty strings (open local shell).
	target, jump := c.SSHTargetForHost("")
	if target != "" || jump != "" {
		t.Errorf("expected empty target/jump for local+no-host, got target=%q jump=%q", target, jump)
	}

	// Specific host selected — should SSH directly.
	target, jump = c.SSHTargetForHost("fst01.cern.ch")
	if target != "root@fst01.cern.ch" {
		t.Errorf("expected root@fst01.cern.ch, got %q", target)
	}
	if jump != "" {
		t.Errorf("expected no jump for local client, got %q", jump)
	}
}

func TestSSHTargetForHostLocalSameHostUsesLocalShell(t *testing.T) {
	prev := hostnameFunc
	hostnameFunc = func() (string, error) { return "eospilot-ns-02.cern.ch", nil }
	defer func() { hostnameFunc = prev }()

	c := &Client{}

	target, jump := c.SSHTargetForHost("eospilot-ns-02.cern.ch")
	if target != "" || jump != "" {
		t.Fatalf("expected local self host to stay local, got target=%q jump=%q", target, jump)
	}

	target, jump = c.SSHTargetForHost("eospilot-ns-02")
	if target != "" || jump != "" {
		t.Fatalf("expected short local self host to stay local, got target=%q jump=%q", target, jump)
	}
}

func TestSSHTargetForHostRemoteSameHost(t *testing.T) {
	// Running via SSH to mgm01.cern.ch; selected host IS the gateway.
	c := &Client{resolvedSSHTarget: "root@mgm01.cern.ch"}

	target, jump := c.SSHTargetForHost("mgm01.cern.ch")
	if target != "root@mgm01.cern.ch" {
		t.Errorf("expected direct target root@mgm01.cern.ch, got %q", target)
	}
	if jump != "" {
		t.Errorf("expected no jump when selected host IS the gateway, got %q", jump)
	}
}

func TestSSHTargetForHostRemoteDifferentHost(t *testing.T) {
	// Running via SSH to mgm01.cern.ch; selected host is a different FST.
	c := &Client{resolvedSSHTarget: "root@mgm01.cern.ch"}

	target, jump := c.SSHTargetForHost("fst01.cern.ch")
	if target != "root@fst01.cern.ch" {
		t.Errorf("expected root@fst01.cern.ch, got %q", target)
	}
	if jump != "root@mgm01.cern.ch" {
		t.Errorf("expected jump root@mgm01.cern.ch, got %q", jump)
	}
}

func TestSSHArgsDefault(t *testing.T) {
	c := &Client{}

	got := c.SSHArgs(true, "root@host", "hostname")
	want := []string{"-o", "LogLevel=ERROR", "-o", "BatchMode=yes", "root@host", "hostname"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected SSH args: got %v want %v", got, want)
	}
}

func TestSSHArgsAcceptNewHostKeys(t *testing.T) {
	c := &Client{acceptNewHostKeys: true}

	got := c.SSHArgs(false, "-t", "root@host")
	want := []string{"-o", "LogLevel=ERROR", "-o", "BatchMode=no", "-o", "StrictHostKeyChecking=accept-new", "-t", "root@host"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected SSH args: got %v want %v", got, want)
	}
}

func TestHostOnly(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"eospilot-ns-02.cern.ch:7777", "eospilot-ns-02.cern.ch"},
		{"lobisapa-dev.cern.ch:7777", "lobisapa-dev.cern.ch"},
		{"hostname", "hostname"},
		{"", ""},
	}
	for _, tt := range tests {
		got := hostOnly(tt.input)
		if got != tt.want {
			t.Errorf("hostOnly(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMatchesLocalHost(t *testing.T) {
	prev := hostnameFunc
	hostnameFunc = func() (string, error) { return "eospilot-ns-02.cern.ch", nil }
	defer func() { hostnameFunc = prev }()

	for _, host := range []string{
		"eospilot-ns-02.cern.ch",
		"eospilot-ns-02",
		"localhost",
		"127.0.0.1",
		"::1",
	} {
		if !matchesLocalHost(host) {
			t.Fatalf("expected %q to match local host", host)
		}
	}

	for _, host := range []string{
		"eospilot-ns-01.cern.ch",
		"fst01.cern.ch",
	} {
		if matchesLocalHost(host) {
			t.Fatalf("did not expect %q to match local host", host)
		}
	}
}

func TestIOShapingPolicySetArgsForApp(t *testing.T) {
	got, err := ioShapingPolicySetArgs(IOShapingPolicyUpdate{
		Mode:                        IOShapingApps,
		ID:                          "test2",
		Enabled:                     true,
		LimitReadBytesPerSec:        0,
		LimitWriteBytesPerSec:       15000000,
		ReservationReadBytesPerSec:  2000,
		ReservationWriteBytesPerSec: 3000,
	})
	if err != nil {
		t.Fatalf("expected io shaping args to build, got error %v", err)
	}

	want := []string{
		"eos", "io", "shaping", "policy", "set",
		"--app", "test2",
		"--enable",
		"--limit-read", "0",
		"--limit-write", "15000000",
		"--reservation-read", "2000",
		"--reservation-write", "3000",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected io shaping policy args: got %v want %v", got, want)
	}
}

func TestIOShapingPolicySetArgsForGroupDisable(t *testing.T) {
	got, err := ioShapingPolicySetArgs(IOShapingPolicyUpdate{
		Mode:    IOShapingGroups,
		ID:      "1234",
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("expected io shaping args to build, got error %v", err)
	}
	if !strings.Contains(strings.Join(got, " "), "--gid 1234 --disable") {
		t.Fatalf("expected group policy args to use --gid and --disable, got %v", got)
	}
}

func TestIOShapingNodesUsesNodesFlag(t *testing.T) {
	runner := &recordingRunner{out: []byte(`[{"id":"node1","type":"node","window_sec":5,"read_rate_bps":1000,"write_rate_bps":2000,"read_iops":3,"write_iops":4}]`)}
	client := &Client{timeout: time.Minute, runner: runner}

	records, err := client.IOShaping(context.Background(), IOShapingNodes)
	if err != nil {
		t.Fatalf("IOShaping(nodes) error: %v", err)
	}
	if len(records) != 1 || records[0].ID != "node1" || records[0].ReadBps != 1000 {
		t.Fatalf("unexpected node shaping records: %+v", records)
	}
	got := append([]string{runner.calls[0].name}, runner.calls[0].args...)
	want := []string{"eos", "io", "shaping", "ls", "--nodes", "--json", "--window", "5"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected io shaping nodes args: got %v want %v", got, want)
	}
}

func TestIOShapingPressureParsesRecords(t *testing.T) {
	runner := &recordingRunner{out: []byte(`[{
		"type":"app_node_pressure",
		"app":"bench",
		"node_id":"node1:1095",
		"node_io_pressure":0.25,
		"has_node_io_pressure":true,
		"read_rate_bps":1000,
		"write_rate_bps":2000,
		"global_read_rate_bps":3000,
		"global_write_rate_bps":4000,
		"reservation_read_bytes_per_sec":5000,
		"reservation_write_bytes_per_sec":6000,
		"read_reservation_deficit_bps":7000,
		"write_reservation_deficit_bps":8000,
		"read_pressure_active":true,
		"write_pressure_active":false,
		"read_reservation_deficit_active":true,
		"write_reservation_deficit_active":false,
		"read_triggers_competitor_throttling":false,
		"write_triggers_competitor_throttling":true,
		"node_has_pressured_read_reservation":true,
		"node_has_pressured_write_reservation":false
	}]`)}
	client := &Client{timeout: time.Minute, runner: runner}

	records, err := client.IOShapingPressure(context.Background())
	if err != nil {
		t.Fatalf("IOShapingPressure() error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one pressure record, got %d", len(records))
	}
	gotRecord := records[0]
	if gotRecord.App != "bench" || gotRecord.NodeID != "node1:1095" || gotRecord.NodeIOPressure != 0.25 {
		t.Fatalf("unexpected pressure record identity: %+v", gotRecord)
	}
	if !gotRecord.ReadPressureActive || !gotRecord.ReadReservationDeficitActive || !gotRecord.WriteTriggersCompetitorThrottling || !gotRecord.NodeHasPressuredReadReservation {
		t.Fatalf("expected pressure booleans to parse, got %+v", gotRecord)
	}
	got := append([]string{runner.calls[0].name}, runner.calls[0].args...)
	want := []string{"eos", "io", "shaping", "pressure", "ls", "--json"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected io shaping pressure args: got %v want %v", got, want)
	}
}

func TestIOShapingConfigParsesLimitsEnabled(t *testing.T) {
	runner := &recordingRunner{out: []byte(`{"limits_enabled":true}`)}
	client := &Client{timeout: time.Minute, runner: runner}

	config, err := client.IOShapingConfig(context.Background())
	if err != nil {
		t.Fatalf("IOShapingConfig() error: %v", err)
	}
	if !config.LimitsEnabled {
		t.Fatalf("expected LimitsEnabled=true")
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one command, got %d", len(runner.calls))
	}
	got := append([]string{runner.calls[0].name}, runner.calls[0].args...)
	want := []string{"eos", "io", "shaping", "config", "ls", "--json"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected io shaping config args: got %v want %v", got, want)
	}
}

func TestSetIOShapingLimitsEnabledArgs(t *testing.T) {
	for _, tc := range []struct {
		enabled bool
		want    string
	}{
		{enabled: true, want: "enabled"},
		{enabled: false, want: "disabled"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			runner := &recordingRunner{}
			client := &Client{timeout: time.Minute, runner: runner}

			if err := client.SetIOShapingLimitsEnabled(context.Background(), tc.enabled); err != nil {
				t.Fatalf("SetIOShapingLimitsEnabled() error: %v", err)
			}
			if len(runner.calls) != 1 {
				t.Fatalf("expected one command, got %d", len(runner.calls))
			}
			got := append([]string{runner.calls[0].name}, runner.calls[0].args...)
			want := []string{"eos", "io", "shaping", "config", "set", "--limits", tc.want}
			if strings.Join(got, "|") != strings.Join(want, "|") {
				t.Fatalf("unexpected io shaping limits args: got %v want %v", got, want)
			}
		})
	}
}

func TestGroupSetArgsForDrain(t *testing.T) {
	got, err := groupSetArgs("default.1", "drain")
	if err != nil {
		t.Fatalf("expected group set args to build, got error %v", err)
	}

	want := []string{"eos", "-b", "group", "set", "default.1", "drain"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected group set args: got %v want %v", got, want)
	}
}

func TestGroupSetArgsRejectsInvalidStatus(t *testing.T) {
	_, err := groupSetArgs("default.1", "paused")
	if err == nil {
		t.Fatal("expected invalid group status to fail")
	}
}

func TestIOShapingPolicyRemoveArgsForUser(t *testing.T) {
	got, err := ioShapingPolicyRemoveArgs(IOShapingUsers, "1001")
	if err != nil {
		t.Fatalf("expected io shaping remove args to build, got error %v", err)
	}

	want := []string{
		"eos", "io", "shaping", "policy", "rm",
		"--uid", "1001",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected io shaping policy rm args: got %v want %v", got, want)
	}
}

// --- splitHostPort ---

func TestSplitHostPort(t *testing.T) {
	t.Run("host:port", func(t *testing.T) {
		host, port := splitHostPort("host:1234")
		if host != "host" || port != 1234 {
			t.Fatalf("expected (host, 1234), got (%q, %d)", host, port)
		}
	})
	t.Run("host only", func(t *testing.T) {
		host, port := splitHostPort("host")
		if host != "host" || port != 0 {
			t.Fatalf("expected (host, 0), got (%q, %d)", host, port)
		}
	})
	t.Run("host:abc", func(t *testing.T) {
		host, port := splitHostPort("host:abc")
		if host != "host" || port != 0 {
			t.Fatalf("expected (host, 0), got (%q, %d)", host, port)
		}
	})
}

// --- cleanPath ---

func TestCleanPathEmpty(t *testing.T) {
	if got := cleanPath(""); got != "/" {
		t.Fatalf("expected %q, got %q", "/", got)
	}
}

func TestCleanPathRelative(t *testing.T) {
	if got := cleanPath("foo/bar"); got != "/foo/bar" {
		t.Fatalf("expected %q, got %q", "/foo/bar", got)
	}
}

func TestCleanPathTrailingSlash(t *testing.T) {
	if got := cleanPath("/eos/"); got != "/eos" {
		t.Fatalf("expected %q, got %q", "/eos", got)
	}
}

func TestCleanPathAlreadyClean(t *testing.T) {
	if got := cleanPath("/eos/dev"); got != "/eos/dev" {
		t.Fatalf("expected %q, got %q", "/eos/dev", got)
	}
}

// --- parseHumanBytes ---

func TestParseHumanBytesAllUnits(t *testing.T) {
	tests := []struct {
		input string
		want  uint64
	}{
		{"10 KB", 10 * 1024},
		{"2 MB", 2 * 1024 * 1024},
		{"3 GB", 3 * 1024 * 1024 * 1024},
		{"1 TB", 1024 * 1024 * 1024 * 1024},
		{"512 B", 512},
		{"", 0},
		{"42", 0}, // single field, missing unit
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := parseHumanBytes(tt.input); got != tt.want {
				t.Fatalf("parseHumanBytes(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// --- parseUint ---

func TestParseUintEmpty(t *testing.T) {
	if got := parseUint(""); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestParseUintMultipleFields(t *testing.T) {
	if got := parseUint("123 [booted]"); got != 123 {
		t.Fatalf("expected 123, got %d", got)
	}
}

// --- toUint64 ---

func TestToUint64AdditionalTypes(t *testing.T) {
	t.Run("string returns 0", func(t *testing.T) {
		if got := toUint64("hello"); got != 0 {
			t.Fatalf("expected 0, got %d", got)
		}
	})
	t.Run("int(5) returns 5", func(t *testing.T) {
		if got := toUint64(int(5)); got != 5 {
			t.Fatalf("expected 5, got %d", got)
		}
	})
	t.Run("int64(10) returns 10", func(t *testing.T) {
		if got := toUint64(int64(10)); got != 10 {
			t.Fatalf("expected 10, got %d", got)
		}
	})
}

// --- ensureRootPrefix ---

func TestEnsureRootPrefixAlreadyPresent(t *testing.T) {
	if got := ensureRootPrefix("root@host"); got != "root@host" {
		t.Fatalf("expected %q, got %q", "root@host", got)
	}
}

func TestEnsureRootPrefixNotPresent(t *testing.T) {
	if got := ensureRootPrefix("host"); got != "root@host" {
		t.Fatalf("expected %q, got %q", "root@host", got)
	}
}

// --- ShellJoin / ShellDisplayJoin ---

func TestShellJoinExported(t *testing.T) {
	got := ShellJoin([]string{"echo", "hello world"})
	if !strings.Contains(got, "hello world") {
		t.Fatalf("expected quoted arg in result, got %q", got)
	}
}

func TestShellDisplayJoinExported(t *testing.T) {
	got := ShellDisplayJoin([]string{"ls", "-la", "/tmp/my dir"})
	if !strings.Contains(got, "ls") {
		t.Fatalf("expected ls in result, got %q", got)
	}
	if !strings.Contains(got, "/tmp/my dir") {
		t.Fatalf("expected quoted path in result, got %q", got)
	}
}

// --- HostOnly (exported) ---

func TestHostOnlyExported(t *testing.T) {
	if got := HostOnly("host:1234"); got != "host" {
		t.Fatalf("expected %q, got %q", "host", got)
	}
}

// --- NormalizeClusterInstance ---

func TestNormalizeClusterInstanceWhitespace(t *testing.T) {
	if got := NormalizeClusterInstance(" "); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestNormalizeClusterInstanceTabsNewlines(t *testing.T) {
	if got := NormalizeClusterInstance("a\tb"); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

// --- parseNamespaceAttrs ---

func TestParseNamespaceAttrsEmptyInput(t *testing.T) {
	attrs := parseNamespaceAttrs([]byte(""))
	if len(attrs) != 0 {
		t.Fatalf("expected 0 attrs, got %d", len(attrs))
	}
}

func TestParseNamespaceAttrsStarLines(t *testing.T) {
	input := "* this is a header\nkey1=val1\n"
	attrs := parseNamespaceAttrs([]byte(input))
	if len(attrs) != 1 {
		t.Fatalf("expected 1 attr, got %d", len(attrs))
	}
	if attrs[0].Key != "key1" || attrs[0].Value != "val1" {
		t.Fatalf("unexpected attr: %+v", attrs[0])
	}
}

func TestParseNamespaceAttrsNoEquals(t *testing.T) {
	input := "no-equals-here\nkey=value\n"
	attrs := parseNamespaceAttrs([]byte(input))
	if len(attrs) != 1 {
		t.Fatalf("expected 1 attr, got %d", len(attrs))
	}
	if attrs[0].Key != "key" || attrs[0].Value != "value" {
		t.Fatalf("unexpected attr: %+v", attrs[0])
	}
}

func TestParseMonitoringKeyValues(t *testing.T) {
	input := []byte(`
uid=all gid=all is_master=true master_id=eospilot-ns-02.cern.ch:1094
uid=all gid=all ns.mgm.local=eospilot-ns-02.cern.ch:1094
uid=all gid=all ns.mgm.role=leader
uid=all gid=all ns.mgm.leader=eospilot-ns-02.cern.ch:1094
uid=all gid=all ns.mgm.followers=eospilot-ns-ip700.cern.ch:1094,eospilot-ns-01.cern.ch:1094
uid=all gid=all ns.qdb.leader=eospilot-ns-02.cern.ch:7777
uid=all gid=all ns.qdb.followers=eospilot-ns-01.cern.ch:7777,eospilot-ns-ip700.cern.ch:7777
`)
	values := parseMonitoringKeyValues(input)

	if got := values["ns.mgm.leader"]; got != "eospilot-ns-02.cern.ch:1094" {
		t.Fatalf("expected ns.mgm.leader, got %q", got)
	}
	if got := values["ns.qdb.followers"]; got != "eospilot-ns-01.cern.ch:7777,eospilot-ns-ip700.cern.ch:7777" {
		t.Fatalf("expected ns.qdb.followers, got %q", got)
	}
	if got := values["master_id"]; got != "eospilot-ns-02.cern.ch:1094" {
		t.Fatalf("expected master_id, got %q", got)
	}
}

func TestParseMGMsFromNSStatMonitoring(t *testing.T) {
	input := []byte(`
uid=all gid=all ns.mgm.local=eospilot-ns-02.cern.ch:1094
uid=all gid=all ns.mgm.role=leader
uid=all gid=all ns.mgm.leader=eospilot-ns-02.cern.ch:1094
uid=all gid=all ns.mgm.followers=eospilot-ns-ip700.cern.ch:1094,eospilot-ns-01.cern.ch:1094
uid=all gid=all ns.qdb.leader=eospilot-ns-02.cern.ch:7777
uid=all gid=all ns.qdb.followers=eospilot-ns-01.cern.ch:7777,eospilot-ns-ip700.cern.ch:7777
`)

	mgms, ok := parseMGMsFromNSStatMonitoring(input)
	if !ok {
		t.Fatal("expected structured ns stat monitoring output to parse")
	}
	if len(mgms) != 3 {
		t.Fatalf("expected 3 combined records, got %d", len(mgms))
	}

	if mgms[0].Host != "eospilot-ns-02.cern.ch" || mgms[0].Port != 1094 || mgms[0].Role != "leader" || mgms[0].Status != "online" {
		t.Fatalf("unexpected MGM leader record: %+v", mgms[0])
	}
	if mgms[0].QDBHost != "eospilot-ns-02.cern.ch" || mgms[0].QDBPort != 7777 || mgms[0].QDBRole != "leader" || mgms[0].QDBStatus != "online" {
		t.Fatalf("unexpected QDB leader record: %+v", mgms[0])
	}
	if mgms[1].Host != "eospilot-ns-ip700.cern.ch" || mgms[1].Role != "follower" {
		t.Fatalf("unexpected first follower record: %+v", mgms[1])
	}
	if mgms[1].QDBHost != "eospilot-ns-01.cern.ch" || mgms[1].QDBRole != "follower" {
		t.Fatalf("unexpected first QDB follower record: %+v", mgms[1])
	}
	if mgms[2].Host != "eospilot-ns-01.cern.ch" || mgms[2].QDBHost != "eospilot-ns-ip700.cern.ch" {
		t.Fatalf("unexpected combined record ordering: %+v", mgms[2])
	}
}

func TestMGMsRaftFallbackDoesNotUseQDBVersionForMGM(t *testing.T) {
	runner := &scriptedRunner{responses: []scriptedResponse{
		{out: []byte("uid=all gid=all master_id=eospilot-ns-02.cern.ch:1094\n")},
		{out: []byte(`LEADER eospilot-ns-02.cern.ch:7777
----------
MYSELF eospilot-ns-02.cern.ch:7777
VERSION 5.4.2.1
STATUS LEADER
----------
NODES eospilot-ns-02.cern.ch:7777
`)},
	}}
	c := &Client{timeout: time.Second, runner: runner}

	mgms, err := c.MGMs(context.Background())
	if err != nil {
		t.Fatalf("MGMs() error: %v", err)
	}
	if len(mgms) != 1 {
		t.Fatalf("expected one MGM record, got %d", len(mgms))
	}
	if got := mgms[0].EOSVersion; got != "" {
		t.Fatalf("expected MGM version to stay empty until eos version probe, got %q", got)
	}
	if got := mgms[0].QDBVersion; got != "5.4.2.1" {
		t.Fatalf("expected QDB version from raft-info, got %q", got)
	}
}

func TestNodeStatsFromMonitoringValues(t *testing.T) {
	values := parseMonitoringKeyValues([]byte(`
uid=all gid=all ns.total.files=23502173
uid=all gid=all ns.total.directories=383968
uid=all gid=all ns.current.fid=2590610131
uid=all gid=all ns.current.cid=3465125
uid=all gid=all ns.memory.virtual=27031158784
uid=all gid=all ns.memory.resident=13764378624
uid=all gid=all ns.memory.share=89653248
uid=all gid=all ns.memory.growth=23214104576
uid=all gid=all ns.stat.threads=666
uid=all gid=all ns.fds.all=866
uid=all gid=all ns.uptime=1523
`))

	stats := nodeStatsFromMonitoringValues(values)

	if stats.FileCount != 23502173 {
		t.Fatalf("expected file count, got %d", stats.FileCount)
	}
	if stats.DirCount != 383968 {
		t.Fatalf("expected dir count, got %d", stats.DirCount)
	}
	if stats.CurrentFID != 2590610131 || stats.CurrentCID != 3465125 {
		t.Fatalf("unexpected current IDs: fid=%d cid=%d", stats.CurrentFID, stats.CurrentCID)
	}
	if stats.MemVirtual != 27031158784 || stats.MemResident != 13764378624 {
		t.Fatalf("unexpected memory stats: virtual=%d resident=%d", stats.MemVirtual, stats.MemResident)
	}
	if stats.MemShared != 89653248 || stats.MemGrowth != 23214104576 {
		t.Fatalf("unexpected shared/growth memory stats: shared=%d growth=%d", stats.MemShared, stats.MemGrowth)
	}
	if stats.ThreadCount != 666 || stats.FileDescs != 866 {
		t.Fatalf("unexpected threads/fds: threads=%d fds=%d", stats.ThreadCount, stats.FileDescs)
	}
	if stats.Uptime.Seconds() != 1523 {
		t.Fatalf("expected uptime 1523s, got %s", stats.Uptime)
	}
}

func TestMGMPortFromMonitoringValues(t *testing.T) {
	values := parseMonitoringKeyValues([]byte(`
uid=all gid=all master_id=eospilot-ns-02.cern.ch:1094
`))
	if got := mgmPortFromMonitoringValues(values); got != "1094" {
		t.Fatalf("expected 1094, got %q", got)
	}
	if got := mgmPortFromMonitoringValues(map[string]string{}); got != "1094" {
		t.Fatalf("expected fallback port, got %q", got)
	}
}

func TestDiscoverMGMMasterUsesMGMLeaderFromMonitoring(t *testing.T) {
	runner := &recordingRunner{out: []byte(`
uid=all gid=all is_master=true master_id=eospilot-ns-02.cern.ch:1094
uid=all gid=all ns.mgm.leader=eospilot-ns-02.cern.ch:1094
uid=all gid=all ns.mgm.followers=eospilot-ns-ip700.cern.ch:1094,eospilot-ns-01.cern.ch:1094
uid=all gid=all ns.qdb.leader=eospilot-ns-01.cern.ch:7777
uid=all gid=all ns.qdb.followers=eospilot-ns-ip700.cern.ch:7777,eospilot-ns-02.cern.ch:7777
`)}
	c := &Client{
		sshTarget: "eospilot",
		timeout:   time.Second,
		runner:    runner,
	}

	resolved, err := c.DiscoverMGMMaster(context.Background())
	if err != nil {
		t.Fatalf("DiscoverMGMMaster() error: %v", err)
	}
	if resolved != "root@eospilot-ns-02.cern.ch" {
		t.Fatalf("expected MGM leader target, got %q", resolved)
	}
	if got := c.ResolvedSSHTarget(); got != resolved {
		t.Fatalf("expected resolved target to be stored, got %q", got)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one command, got %d", len(runner.calls))
	}
	call := runner.calls[0]
	if call.name != "ssh" {
		t.Fatalf("expected discovery to run over ssh, got %q", call.name)
	}
	joinedArgs := strings.Join(call.args, " ")
	if !strings.Contains(joinedArgs, "eospilot") ||
		!strings.Contains(joinedArgs, "eos") ||
		!strings.Contains(joinedArgs, "ns") ||
		!strings.Contains(joinedArgs, "stat") ||
		!strings.Contains(joinedArgs, "-m") {
		t.Fatalf("unexpected discovery command: %v", call.args)
	}
	if strings.Contains(joinedArgs, "redis-cli") {
		t.Fatalf("discovery should not use QDB raft leadership for EOS command routing: %v", call.args)
	}
}

func TestDiscoverMGMMasterFallsBackToMasterID(t *testing.T) {
	runner := &recordingRunner{out: []byte(`
uid=all gid=all is_master=true master_id=legacy-mgm.cern.ch:1094
`)}
	c := &Client{
		sshTarget: "legacy",
		timeout:   time.Second,
		runner:    runner,
	}

	resolved, err := c.DiscoverMGMMaster(context.Background())
	if err != nil {
		t.Fatalf("DiscoverMGMMaster() error: %v", err)
	}
	if resolved != "root@legacy-mgm.cern.ch" {
		t.Fatalf("expected master_id target, got %q", resolved)
	}
}

func TestRunCommandContextReportsContextDeadline(t *testing.T) {
	c := &Client{
		timeout: time.Nanosecond,
		runner:  deadlineRunner{},
	}

	_, err := c.runCommandContext(context.Background(), "eos", "version")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
}

func TestTailLogOnHostReportsMissingFile(t *testing.T) {
	runner := &recordingRunner{
		out: []byte("tail: cannot open '/var/log/eos/mgm/Drain.log' for reading: No such file or directory\n"),
		err: errors.New("exit status 1"),
	}
	c := &Client{
		timeout: time.Second,
		runner:  runner,
	}

	_, err := c.TailLog(context.Background(), "/var/log/eos/mgm/Drain.log", 2000)
	if !errors.Is(err, ErrLogFileNotFound) {
		t.Fatalf("expected ErrLogFileNotFound, got %v", err)
	}
}

func TestQDBAttemptCoupRunsOnSelectedHost(t *testing.T) {
	runner := &recordingRunner{out: []byte("OK\n")}
	c := &Client{
		sshTarget:         "mgm01.cern.ch",
		resolvedSSHTarget: "root@mgm01.cern.ch",
		timeout:           time.Second,
		runner:            runner,
	}

	out, err := c.QDBAttemptCoup(context.Background(), "qdb01.cern.ch")
	if err != nil {
		t.Fatalf("QDBAttemptCoup() error: %v", err)
	}
	if string(out) != "OK\n" {
		t.Fatalf("expected command output, got %q", out)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one command, got %d", len(runner.calls))
	}
	call := runner.calls[0]
	if call.name != "ssh" {
		t.Fatalf("expected ssh command, got %q", call.name)
	}
	joinedArgs := strings.Join(call.args, " ")
	for _, want := range []string{
		"-J root@mgm01.cern.ch",
		"root@qdb01.cern.ch",
		"redis-cli",
		"-p",
		"7777",
		"raft-attempt-coup",
	} {
		if !strings.Contains(joinedArgs, want) {
			t.Fatalf("expected ssh args to contain %q, got %v", want, call.args)
		}
	}
}

func TestSetNodeStatusRunsEOSNodeSet(t *testing.T) {
	runner := &recordingRunner{out: []byte("success\n")}
	c := &Client{
		timeout: time.Second,
		runner:  runner,
	}

	if err := c.SetNodeStatus(context.Background(), "st-120-100gb-astpuc.cern.ch", 1095, "off"); err != nil {
		t.Fatalf("SetNodeStatus() error: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one command, got %d", len(runner.calls))
	}
	call := runner.calls[0]
	if call.name != "eos" {
		t.Fatalf("expected eos command, got %q", call.name)
	}
	want := []string{"node", "set", "st-120-100gb-astpuc.cern.ch:1095", "off"}
	if strings.Join(call.args, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected node set args: got %v want %v", call.args, want)
	}
}

// --- entryFromCLI ---

func TestEntryFromCLIRootPath(t *testing.T) {
	info := cliFileInfo{
		Name: "",
		Path: "/",
		Mode: 040755, // directory
	}
	entry := entryFromCLI(info)
	if entry.Name != "/" {
		t.Fatalf("expected name %q, got %q", "/", entry.Name)
	}
	if entry.Path != "/" {
		t.Fatalf("expected path %q, got %q", "/", entry.Path)
	}
	if entry.Kind != EntryKindContainer {
		t.Fatalf("expected kind %q, got %q", EntryKindContainer, entry.Kind)
	}
}

func TestEntryFromCLIWithLinkTarget(t *testing.T) {
	info := cliFileInfo{
		Name:       "mylink",
		Path:       "/eos/mylink",
		LinkTarget: "/eos/target",
	}
	entry := entryFromCLI(info)
	if entry.LinkName != "/eos/target" {
		t.Fatalf("expected link target %q, got %q", "/eos/target", entry.LinkName)
	}
}

// --- parseRaftInfo ---

func TestParseRaftInfoEmpty(t *testing.T) {
	info := parseRaftInfo([]byte(""))
	if info.Leader != "" || info.Myself != "" || len(info.Nodes) != 0 {
		t.Fatalf("expected zero-value raftInfo, got %+v", info)
	}
}

func TestParseRaftInfoNoSeparatorLine(t *testing.T) {
	// Lines without spaces are skipped by the parser.
	info := parseRaftInfo([]byte("NOSPACEHERE\n"))
	if info.Leader != "" {
		t.Fatalf("expected empty leader, got %q", info.Leader)
	}
}

// --- parseSpaceStatus ---

func TestParseSpaceStatusEmpty(t *testing.T) {
	records := parseSpaceStatus([]byte(""))
	if len(records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(records))
	}
}

func TestParseSpaceStatusNoDelimiter(t *testing.T) {
	records := parseSpaceStatus([]byte("no delimiter here\nkey:=val\n"))
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Key != "key" || records[0].Value != "val" {
		t.Fatalf("unexpected record: %+v", records[0])
	}
}

// --- ioShapingPolicySetArgs ---

func TestIOShapingPolicySetArgsEmptyID(t *testing.T) {
	_, err := ioShapingPolicySetArgs(IOShapingPolicyUpdate{
		Mode: IOShapingApps,
		ID:   "",
	})
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestIOShapingPolicySetArgsForUser(t *testing.T) {
	got, err := ioShapingPolicySetArgs(IOShapingPolicyUpdate{
		Mode:    IOShapingUsers,
		ID:      "user1",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "--uid user1") {
		t.Fatalf("expected --uid user1, got %v", got)
	}
	if !strings.Contains(joined, "--enable") {
		t.Fatalf("expected --enable, got %v", got)
	}
}

func TestIOShapingPolicyTargetFlagInvalidMode(t *testing.T) {
	_, err := ioShapingPolicyTargetFlag(IOShapingMode(99))
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

// --- ioShapingPolicyRemoveArgs ---

func TestIOShapingPolicyRemoveArgsEmptyID(t *testing.T) {
	_, err := ioShapingPolicyRemoveArgs(IOShapingApps, "")
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestIOShapingPolicyRemoveArgsForGroup(t *testing.T) {
	got, err := ioShapingPolicyRemoveArgs(IOShapingGroups, "grp1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"eos", "io", "shaping", "policy", "rm", "--gid", "grp1"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestIOShapingPolicyRemoveArgsForApp(t *testing.T) {
	got, err := ioShapingPolicyRemoveArgs(IOShapingApps, "myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"eos", "io", "shaping", "policy", "rm", "--app", "myapp"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// --- isSessionCommandLine ---

func TestIsSessionCommandLineValid(t *testing.T) {
	if !isSessionCommandLine("[2024-01-01 00:00:00] ssh root@host eos version") {
		t.Fatal("expected true for valid command line")
	}
}

func TestIsSessionCommandLineError(t *testing.T) {
	if isSessionCommandLine("[2024-01-01 00:00:00] ERROR something went wrong") {
		t.Fatal("expected false for ERROR line")
	}
}

func TestIsSessionCommandLineOutput(t *testing.T) {
	if isSessionCommandLine("[2024-01-01 00:00:00]   output: some data") {
		t.Fatal("expected false for output line")
	}
}

func TestIsSessionCommandLineNoPrefix(t *testing.T) {
	if isSessionCommandLine("no bracket prefix") {
		t.Fatal("expected false for line without bracket prefix")
	}
}

// --- parseEOSServerVersion ---

func TestParseEOSServerVersionEmpty(t *testing.T) {
	if got := parseEOSServerVersion([]byte("")); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestParseEOSServerVersionNoMatch(t *testing.T) {
	if got := parseEOSServerVersion([]byte("random text\nnothing here\n")); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

// --- Client accessor methods ---

func TestAcceptNewHostKeysFromClient(t *testing.T) {
	c := &Client{acceptNewHostKeys: true}
	if !c.AcceptNewHostKeys() {
		t.Fatal("expected AcceptNewHostKeys to return true")
	}
	c2 := &Client{acceptNewHostKeys: false}
	if c2.AcceptNewHostKeys() {
		t.Fatal("expected AcceptNewHostKeys to return false")
	}
}

func TestOriginalSSHTargetFromClient(t *testing.T) {
	c := &Client{sshTarget: "myhost"}
	if got := c.OriginalSSHTarget(); got != "myhost" {
		t.Fatalf("expected %q, got %q", "myhost", got)
	}
}

func TestResolvedSSHTargetFromClient(t *testing.T) {
	t.Run("uses resolvedSSHTarget when set", func(t *testing.T) {
		c := &Client{sshTarget: "orig", resolvedSSHTarget: "resolved"}
		if got := c.ResolvedSSHTarget(); got != "resolved" {
			t.Fatalf("expected %q, got %q", "resolved", got)
		}
	})
	t.Run("falls back to sshTarget", func(t *testing.T) {
		c := &Client{sshTarget: "orig"}
		if got := c.ResolvedSSHTarget(); got != "orig" {
			t.Fatalf("expected %q, got %q", "orig", got)
		}
	})
}

// --- Client.Close ---

func TestClientClose(t *testing.T) {
	c := &Client{}
	if err := c.Close(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
