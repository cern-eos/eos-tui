package eos

// Integration tests that run real commands against a live EOS instance.
//
// By default the tests run against "lobis-eos-dev" via SSH.
// Set EOS_TEST_INTEGRATION=1 to enable the integration tests.
// Set EOS_TEST_SSH_TARGET to override the SSH target.
// Set EOS_TEST_SKIP=1 to force-skip all integration tests.

import (
	"context"
	"os"
	"testing"
	"time"
)

func integrationClient(t *testing.T) *Client {
	t.Helper()

	if os.Getenv("EOS_TEST_SKIP") == "1" {
		t.Skip("EOS_TEST_SKIP=1: skipping integration tests")
	}
	if os.Getenv("EOS_TEST_INTEGRATION") != "1" {
		t.Skip("EOS_TEST_INTEGRATION is not set: skipping integration tests")
	}

	target := os.Getenv("EOS_TEST_SSH_TARGET")
	if target == "" {
		target = "lobis-eos-dev"
	}

	c, err := New(context.Background(), Config{
		SSHTarget: target,
		Timeout:   30 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return c
}

func TestIntegrationNodes(t *testing.T) {
	c := integrationClient(t)

	nodes, err := c.Nodes(context.Background())
	if err != nil {
		t.Fatalf("Nodes() error: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("expected at least one node, got none")
	}

	for _, n := range nodes {
		if n.Host == "" {
			t.Errorf("node has empty Host: %+v", n)
		}
	}

	t.Logf("Nodes: %d returned (first: %s status=%s nofs=%d)",
		len(nodes), nodes[0].Host, nodes[0].Status, nodes[0].FileSystemCount)
}

func TestIntegrationFileSystems(t *testing.T) {
	c := integrationClient(t)

	fs, err := c.FileSystems(context.Background())
	if err != nil {
		t.Fatalf("FileSystems() error: %v", err)
	}
	if len(fs) == 0 {
		t.Fatal("expected at least one filesystem, got none")
	}

	for _, f := range fs {
		if f.Host == "" {
			t.Errorf("filesystem has empty Host: %+v", f)
		}
		if f.ID == 0 {
			t.Errorf("filesystem has zero ID: %+v", f)
		}
	}

	t.Logf("FileSystems: %d returned (first: id=%d host=%s path=%s active=%s)",
		len(fs), fs[0].ID, fs[0].Host, fs[0].Path, fs[0].Active)
}

func TestIntegrationSpaces(t *testing.T) {
	c := integrationClient(t)

	spaces, err := c.Spaces(context.Background())
	if err != nil {
		t.Fatalf("Spaces() error: %v", err)
	}
	if len(spaces) == 0 {
		t.Fatal("expected at least one space, got none")
	}

	for _, s := range spaces {
		if s.Name == "" {
			t.Errorf("space has empty Name: %+v", s)
		}
	}

	t.Logf("Spaces: %d returned (first: %s type=%s capacity=%d)",
		len(spaces), spaces[0].Name, spaces[0].Type, spaces[0].CapacityBytes)
}

func TestIntegrationNamespaceStats(t *testing.T) {
	c := integrationClient(t)

	stats, err := c.NamespaceStats(context.Background())
	if err != nil {
		t.Fatalf("NamespaceStats() error: %v", err)
	}

	if stats.TotalFiles == 0 {
		t.Error("expected non-zero TotalFiles")
	}
	if stats.TotalDirectories == 0 {
		t.Error("expected non-zero TotalDirectories")
	}
	if stats.CurrentFID == 0 {
		t.Error("expected non-zero CurrentFID")
	}
	if stats.CurrentCID == 0 {
		t.Error("expected non-zero CurrentCID")
	}
	if stats.MasterHost == "" {
		t.Error("expected non-empty MasterHost")
	}
	if stats.CacheFilesMax == 0 {
		t.Error("expected non-zero CacheFilesMax")
	}
	if stats.CacheContainersMax == 0 {
		t.Error("expected non-zero CacheContainersMax")
	}
	// Request and hit counters are activity metrics. A newly started or idle
	// namespace can legitimately report zero, so their values are useful to log
	// but are not integration-test health requirements.

	t.Logf("NamespaceStats: files=%d dirs=%d fid=%d cid=%d master=%q file_cache=%d/%d container_cache=%d/%d",
		stats.TotalFiles, stats.TotalDirectories, stats.CurrentFID, stats.CurrentCID, stats.MasterHost,
		stats.CacheFilesRequests, stats.CacheFilesMax, stats.CacheContainersRequests, stats.CacheContainersMax)
}

func TestIntegrationEOSVersion(t *testing.T) {
	c := integrationClient(t)

	version, err := c.EOSVersion(context.Background())
	if err != nil {
		t.Fatalf("EOSVersion() error: %v", err)
	}
	if version == "" {
		t.Fatal("expected non-empty version string")
	}

	t.Logf("EOSVersion: %q", version)
}

func TestIntegrationMGMs(t *testing.T) {
	c := integrationClient(t)

	mgms, err := c.MGMs(context.Background())
	if err != nil {
		t.Fatalf("MGMs() error: %v", err)
	}
	if len(mgms) == 0 {
		t.Fatal("expected at least one MGM, got none")
	}

	hasLeader := false
	for _, m := range mgms {
		if m.Host == "" {
			t.Errorf("MGM has empty Host: %+v", m)
		}
		if m.Role == "leader" {
			hasLeader = true
		}
	}
	if !hasLeader {
		t.Error("expected at least one MGM with role=leader")
	}

	t.Logf("MGMs: %d returned", len(mgms))
	for _, m := range mgms {
		t.Logf("  %s role=%s status=%s eos=%s", m.Host, m.Role, m.Status, m.EOSVersion)
	}
}

func TestIntegrationDiscoverMGMMaster(t *testing.T) {
	c := integrationClient(t)

	resolved, err := c.DiscoverMGMMaster(context.Background())
	if err != nil {
		t.Fatalf("DiscoverMGMMaster() error: %v", err)
	}
	if resolved == "" {
		t.Fatal("expected non-empty resolved target")
	}

	// Should be in the form root@hostname
	if len(resolved) < 6 || resolved[:5] != "root@" {
		t.Errorf("expected resolved target to start with root@, got %q", resolved)
	}

	// Exercise the post-discovery route as well as the lookup itself. When the
	// configured target is an SSH alias/bastion, the resolved private MGM must
	// remain reachable through that original gateway.
	version, err := c.EOSVersion(context.Background())
	if err != nil {
		t.Fatalf("EOSVersion() after discovery error: %v", err)
	}
	if version == "" {
		t.Fatal("expected non-empty EOS version after discovery")
	}

	t.Logf("DiscoverMGMMaster: resolved to %q, post-discovery EOS version %q", resolved, version)
}

func TestIntegrationAllComponentsLoad(t *testing.T) {
	// Smoke test: all components load without error, matching what loadInfraCmd does at startup.
	c := integrationClient(t)
	ctx := context.Background()

	type result struct {
		name string
		err  error
	}

	results := make(chan result, 5)

	go func() {
		_, err := c.Nodes(ctx)
		results <- result{"Nodes", err}
	}()
	go func() {
		_, err := c.FileSystems(ctx)
		results <- result{"FileSystems", err}
	}()
	go func() {
		_, err := c.Spaces(ctx)
		results <- result{"Spaces", err}
	}()
	go func() {
		_, err := c.NamespaceStats(ctx)
		results <- result{"NamespaceStats", err}
	}()
	go func() {
		_, err := c.MGMs(ctx)
		results <- result{"MGMs", err}
	}()
	failed := false
	for i := 0; i < 5; i++ {
		r := <-results
		if r.err != nil {
			t.Errorf("%s failed: %v", r.name, r.err)
			failed = true
		} else {
			t.Logf("%s: OK", r.name)
		}
	}
	if failed {
		t.Fatal("one or more components failed to load")
	}
}

func TestIntegrationGroups(t *testing.T) {
	c := integrationClient(t)

	groups, err := c.Groups(context.Background())
	if err != nil {
		t.Fatalf("Groups() error: %v", err)
	}
	if len(groups) == 0 {
		t.Fatal("expected at least one group, got none")
	}

	for _, g := range groups {
		if g.Name == "" {
			t.Errorf("group has empty Name: %+v", g)
		}
	}

	t.Logf("Groups: %d returned (first: %s status=%s nofs=%d)",
		len(groups), groups[0].Name, groups[0].Status, groups[0].NoFS)
}
