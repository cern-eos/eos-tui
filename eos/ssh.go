package eos

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var hostnameFunc = os.Hostname

// ErrLogFileNotFound marks optional log-file targets that are absent on a host.
var ErrLogFileNotFound = errors.New("log file not found")

type execCommandRunner struct{}

func (execCommandRunner) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func (c *Client) sshOptions(batchMode bool) []string {
	options := []string{"-o", "LogLevel=ERROR"}
	if batchMode {
		options = append(options, "-o", "BatchMode=yes")
	} else {
		options = append(options, "-o", "BatchMode=no")
	}
	if c.acceptNewHostKeys {
		options = append(options, "-o", "StrictHostKeyChecking=accept-new")
	}
	return options
}

func (c *Client) SSHArgs(batchMode bool, extraArgs ...string) []string {
	args := c.sshOptions(batchMode)
	args = append(args, extraArgs...)
	return args
}

func (c *Client) runCommand(args ...string) ([]byte, error) {
	return c.runCommandContext(context.Background(), args...)
}

func (c *Client) runCommandContext(ctx context.Context, args ...string) ([]byte, error) {
	c.logCommand(args)

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := c.commandContext(ctx)
	defer cancel()
	runner := c.runner
	if runner == nil {
		runner = execCommandRunner{}
	}

	target := c.effectiveSSHTarget()
	var out []byte
	var err error
	if target == "" {
		out, err = runner.CombinedOutput(ctx, args[0], args[1:]...)
	} else {
		remoteCommand := shellJoin(args)
		sshArgs := c.SSHArgs(true)
		if jump := c.jumpTargetFor(target); jump != "" {
			sshArgs = append(sshArgs, "-J", jump)
		}
		sshArgs = append(sshArgs, target, remoteCommand)
		out, err = runner.CombinedOutput(ctx, "ssh", sshArgs...)
	}

	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = fmt.Errorf("%w: %v", ctxErr, err)
		}
	}
	if err != nil {
		c.logResponse(args, out, err)
	}
	return out, err
}

func (c *Client) runCommandOnHost(ctx context.Context, host string, args ...string) ([]byte, error) {
	host = strings.TrimSpace(host)
	if host == "" || isCurrentExecutionHost(host, c.effectiveSSHTarget()) {
		return c.runCommandContext(ctx, args...)
	}

	target := ensureRootPrefix(host)
	remoteCommand := shellJoin(args)

	if ctx == nil {
		ctx = context.Background()
	}
	ctxTimeout, cancel := c.commandContext(ctx)
	defer cancel()

	effective := c.effectiveSSHTarget()
	var sshArgs []string
	if effective == "" {
		sshArgs = append(c.SSHArgs(true), target)
	} else {
		jump := c.jumpTargetFor(target)
		sshArgs = c.SSHArgs(true)
		if jump != "" {
			sshArgs = append(sshArgs, "-J", jump)
		}
		sshArgs = append(sshArgs, target)
	}
	c.logRoutedCommand(sshArgs, args)
	sshArgs = append(sshArgs, remoteCommand)

	runner := c.runner
	if runner == nil {
		runner = execCommandRunner{}
	}
	out, err := runner.CombinedOutput(ctxTimeout, "ssh", sshArgs...)
	if err != nil {
		if ctxErr := ctxTimeout.Err(); ctxErr != nil {
			err = fmt.Errorf("%w: %v", ctxErr, err)
		}
	}
	if err != nil {
		c.logResponse(args, out, err)
	}
	return out, err
}

// RTLog queries EOS real-time logs through the CLI instead of tailing a
// specific logfile path. The queue identifies either the MGM (".") or an FST
// queue such as /eos/host:port/fst.
func (c *Client) RTLog(ctx context.Context, queue string, seconds int, tag string) ([]byte, error) {
	if queue == "" {
		queue = "."
	}
	if seconds <= 0 {
		seconds = 600
	}
	if tag == "" {
		tag = "info"
	}

	args := []string{"eos", "rtlog", queue, fmt.Sprintf("%d", seconds), tag}
	out, err := c.runCommandContext(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("eos rtlog %s %d %s: %w (output: %.300s)", queue, seconds, tag, err, out)
	}
	return out, nil
}

// TailLog returns the last n lines of a log file on the effective SSH target
// (or locally when no SSH target is configured).
func (c *Client) TailLog(ctx context.Context, filePath string, n int) ([]byte, error) {
	return c.TailLogOnHost(ctx, "", filePath, n)
}

// TailLogOnHost returns the last n lines of a log file on a specific host.
// When host is empty or matches the current effective target it is equivalent
// to TailLog.  Otherwise the command is routed to the named host, using the
// effective SSH target as a jump proxy when one is configured.
func (c *Client) TailLogOnHost(ctx context.Context, host, filePath string, n int) ([]byte, error) {
	tailArgs := []string{"tail", fmt.Sprintf("-n%d", n), filePath}

	effective := c.effectiveSSHTarget() // e.g. "root@eospilot-ns-02.cern.ch"

	// Direct case: no specific host, or the host IS the current target.
	if host == "" || isCurrentExecutionHost(host, effective) {
		out, err := c.runCommandContext(ctx, tailArgs...)
		if err != nil {
			if isTailLogNotFoundOutput(out) {
				return nil, fmt.Errorf("%w: %s", ErrLogFileNotFound, filePath)
			}
			return nil, fmt.Errorf("tail %s: %w (output: %.300s)", filePath, err, out)
		}
		return out, nil
	}

	// We need to reach a different host.  Use the effective target as a jump
	// proxy (or SSH directly when running locally).
	target := "root@" + host
	tailCmd := shellJoin(tailArgs)

	ctxTimeout, cancel := c.commandContext(ctx)
	defer cancel()

	var out []byte
	var err error
	runner := c.runner
	if runner == nil {
		runner = execCommandRunner{}
	}
	if effective == "" {
		sshArgs := append(c.SSHArgs(true), target)
		c.logRoutedCommand(sshArgs, tailArgs)
		sshArgs = append(sshArgs, tailCmd)
		out, err = runner.CombinedOutput(ctxTimeout, "ssh", sshArgs...)
	} else {
		sshArgs := c.SSHArgs(true)
		if jump := c.jumpTargetFor(target); jump != "" {
			sshArgs = append(sshArgs, "-J", jump)
		}
		sshArgs = append(sshArgs, target)
		c.logRoutedCommand(sshArgs, tailArgs)
		sshArgs = append(sshArgs, tailCmd)
		out, err = runner.CombinedOutput(ctxTimeout, "ssh", sshArgs...)
	}
	if err != nil {
		c.logResponse(tailArgs, out, err)
		if isTailLogNotFoundOutput(out) {
			return nil, fmt.Errorf("%w: %s on %s", ErrLogFileNotFound, filePath, host)
		}
		return nil, fmt.Errorf("tail %s on %s: %w (output: %.300s)", filePath, host, err, out)
	}
	return out, nil
}

func isTailLogNotFoundOutput(out []byte) bool {
	text := string(out)
	return strings.Contains(text, "No such file or directory") &&
		(strings.Contains(text, "cannot open") || strings.Contains(text, "No such file"))
}

// SSHTargetForHost returns the ssh arguments needed to open an interactive
// shell on host, routing via the effective SSH target when necessary.
// Returns (directTarget, jumpProxy) where jumpProxy may be empty.
func (c *Client) SSHTargetForHost(host string) (target, jump string) {
	effective := c.effectiveSSHTarget()

	if host == "" || isCurrentExecutionHost(host, effective) {
		if effective != "" {
			return effective, c.jumpTargetFor(effective)
		}
		return "", ""
	}

	target = "root@" + host
	if effective != "" {
		return target, c.jumpTargetFor(target)
	}
	return target, ""
}

// jumpTargetFor keeps the user-supplied SSH target as the stable gateway after
// MGM discovery. This allows resolved MGM and FST hostnames to remain private
// to the remote network. Clients constructed only with a resolved target keep
// the previous behavior of using that target as the jump host for peers.
func (c *Client) jumpTargetFor(target string) string {
	if c.sshTarget != "" && !isCurrentExecutionHost(target, c.sshTarget) {
		return c.sshTarget
	}
	effective := c.effectiveSSHTarget()
	if c.sshTarget == "" && effective != "" && !isCurrentExecutionHost(target, effective) {
		return effective
	}
	return ""
}

func (c *Client) AcceptNewHostKeys() bool {
	return c.acceptNewHostKeys
}

func isCurrentExecutionHost(host, effective string) bool {
	host = canonicalHost(host)
	if host == "" {
		return true
	}

	effectiveHost := canonicalHost(strings.TrimPrefix(effective, "root@"))
	if effectiveHost != "" {
		return equivalentHostnames(host, effectiveHost)
	}

	return matchesLocalHost(host)
}

// equivalentHostnames treats a short hostname and its FQDN as the same host,
// while avoiding false matches between two different fully qualified domains
// that happen to share a short label.
func equivalentHostnames(left, right string) bool {
	left = canonicalHost(left)
	right = canonicalHost(right)
	if left == "" || right == "" {
		return left == right
	}
	if left == right {
		return true
	}
	leftQualified := strings.Contains(left, ".")
	rightQualified := strings.Contains(right, ".")
	return (!leftQualified || !rightQualified) && shortHost(left) == shortHost(right)
}

func matchesLocalHost(host string) bool {
	host = canonicalHost(host)
	if host == "" {
		return false
	}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}

	local, err := hostnameFunc()
	if err != nil {
		return false
	}
	local = canonicalHost(local)
	if local == "" {
		return false
	}

	return host == local || shortHost(host) == shortHost(local)
}

func canonicalHost(host string) string {
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(host, "root@")
	if host == "::1" || host == "[::1]" {
		return "::1"
	}
	host = hostOnly(host)
	host = strings.TrimSuffix(host, ".")
	return strings.ToLower(host)
}

func shortHost(host string) string {
	host = canonicalHost(host)
	if idx := strings.Index(host, "."); idx != -1 {
		return host[:idx]
	}
	return host
}
