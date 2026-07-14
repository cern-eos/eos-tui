package eos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func New(ctx context.Context, cfg Config) (*Client, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if ctx == nil {
		ctx = context.Background()
	}
	clientCtx, cancel := context.WithCancel(ctx)

	c := &Client{
		ctx:               clientCtx,
		cancel:            cancel,
		sshTarget:         cfg.SSHTarget,
		timeout:           timeout,
		acceptNewHostKeys: cfg.AcceptNewHostKeys,
		runner:            execCommandRunner{},
	}
	c.sessionLogPath = initSessionLog()
	return c, nil
}

// initSessionLog creates ~/.eos-tui/sessions/ if needed, generates a
// timestamped log file path for this session, and updates the
// ~/.eos-tui/latest.log symlink to point at it.
// Returns the session log path, or "" if setup fails (logging silently disabled).
func initSessionLog() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	baseDir := filepath.Join(home, ".eos-tui")
	if err := ensurePrivateDir(baseDir); err != nil {
		return ""
	}
	logDir := filepath.Join(baseDir, "sessions")
	if err := ensurePrivateDir(logDir); err != nil {
		return ""
	}

	// Keep the timestamp human-readable while letting CreateTemp add a random
	// suffix. This avoids different processes reusing the same session file
	// when they start within the same second.
	ts := time.Now().Format("2006-01-02T15-04-05")
	f, err := os.CreateTemp(logDir, ts+"-*.log")
	if err != nil {
		return ""
	}
	sessionFile := f.Name()
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		_ = os.Remove(sessionFile)
		return ""
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(sessionFile)
		return ""
	}

	// Update ~/.eos-tui/latest.log atomically so readers never observe a window
	// where the link is absent. A relative target keeps the directory movable.
	latestLink := filepath.Join(baseDir, "latest.log")
	relTarget := filepath.Join("sessions", filepath.Base(sessionFile))
	temporaryLink := filepath.Join(baseDir, ".latest.log."+filepath.Base(sessionFile)+".tmp")
	if err := os.Symlink(relTarget, temporaryLink); err == nil {
		if err := os.Rename(temporaryLink, latestLink); err != nil {
			_ = os.Remove(temporaryLink)
		}
	}

	return sessionFile
}

func ensurePrivateDir(dir string) error {
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return err
	}

	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("session log path is not a directory: %s", dir)
	}

	return os.Chmod(dir, 0700)
}

func (c *Client) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

func (c *Client) commandContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}

	timeout := c.timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	if c.ctx == nil {
		return cmdCtx, cancel
	}

	stop := context.AfterFunc(c.ctx, cancel)
	return cmdCtx, func() {
		stop()
		cancel()
	}
}

// effectiveSSHTarget returns the host that runCommand will actually SSH to.
func (c *Client) effectiveSSHTarget() string {
	if c.resolvedSSHTarget != "" {
		return c.resolvedSSHTarget
	}
	return c.sshTarget
}

// ResolvedSSHTarget returns the effective SSH target after master discovery,
// or the original target if discovery has not run.
func (c *Client) ResolvedSSHTarget() string {
	return c.effectiveSSHTarget()
}

// OriginalSSHTarget returns the user-supplied SSH target before master discovery.
func (c *Client) OriginalSSHTarget() string {
	return c.sshTarget
}

// ensureRootPrefix returns target with a "root@" prefix, adding one only if
// it is not already present.
func ensureRootPrefix(target string) string {
	if strings.HasPrefix(target, "root@") {
		return target
	}
	return "root@" + target
}

// DiscoverMGMMaster runs `eos -b ns stat -m` on the current SSH target,
// identifies the MGM leader, and updates the client so that all subsequent
// EOS commands are routed directly to the leader host.
// Returns the resolved hostname (e.g. "eospilot-ns-02.cern.ch").
func (c *Client) DiscoverMGMMaster(ctx context.Context) (string, error) {
	output, err := c.runCommandContext(ctx, "eos", "-b", "ns", "stat", "-m")
	if err != nil {
		return "", fmt.Errorf("eos ns stat -m for MGM master discovery: %w", err)
	}

	values := parseMonitoringKeyValues(output)
	leader := mgmLeaderFromMonitoringValues(values)
	if leader == "" {
		return "", fmt.Errorf("no MGM leader found in eos ns stat -m output")
	}

	// EOS nodes run as root; use explicit root@ so the resolved hostname
	// works without relying on SSH config aliases.
	resolved := ensureRootPrefix(hostOnly(leader))
	c.resolvedSSHTarget = resolved
	return resolved, nil
}
