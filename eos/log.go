package eos

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"
)

const maxSessionCommandCacheLines = 1000

func (c *Client) openLogFile() (*os.File, error) {
	if c.sessionLogPath == "" {
		return nil, fmt.Errorf("logging disabled")
	}
	f, err := os.OpenFile(c.sessionLogPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func (c *Client) SessionLogPath() string {
	return c.sessionLogPath
}

func (c *Client) SessionCommands(n int) ([]string, error) {
	if c.sessionLogPath == "" {
		return nil, fmt.Errorf("logging disabled")
	}
	if n <= 0 {
		return nil, nil
	}

	c.sessionCommandMu.Lock()
	defer c.sessionCommandMu.Unlock()

	f, err := os.Open(c.sessionLogPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() < c.sessionCommandOffset {
		c.sessionCommandOffset = 0
		c.sessionCommandCache = nil
	}
	if c.sessionCommandOffset > 0 {
		if _, err := f.Seek(c.sessionCommandOffset, 0); err != nil {
			return nil, err
		}
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !isSessionCommandLine(line) {
			continue
		}
		c.sessionCommandCache = append(c.sessionCommandCache, line)
		if len(c.sessionCommandCache) > maxSessionCommandCacheLines {
			copy(c.sessionCommandCache, c.sessionCommandCache[len(c.sessionCommandCache)-maxSessionCommandCacheLines:])
			c.sessionCommandCache = c.sessionCommandCache[:maxSessionCommandCacheLines]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if info, err := f.Stat(); err == nil {
		c.sessionCommandOffset = info.Size()
	}

	lines := c.sessionCommandCache
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	lines = append([]string(nil), lines...)
	return lines, nil
}

func isSessionCommandLine(line string) bool {
	if !strings.HasPrefix(line, "[") {
		return false
	}
	if strings.Contains(line, "] ERROR ") {
		return false
	}
	if strings.Contains(line, "]   output: ") {
		return false
	}
	return true
}

// LogCommand writes an arbitrary command line to the session log in the same
// format used by runCommand. Use this for commands issued outside of the
// normal eos.Client SSH path (e.g. direct exec.Command calls from the UI).
func (c *Client) LogCommand(args []string) {
	c.logCommand(args)
}

func (c *Client) logCommand(args []string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	safeArgs := sanitizeLogStrings(args)
	var command string
	if target := c.effectiveSSHTarget(); target != "" {
		sshArgs := c.SSHArgs(true)
		if jump := c.jumpTargetFor(target); jump != "" {
			sshArgs = append(sshArgs, "-J", jump)
		}
		command = fmt.Sprintf(
			"ssh %s %s %s",
			shellDisplayJoin(sanitizeLogStrings(sshArgs)),
			sanitizeLogText(target),
			shellDisplayJoin(safeArgs),
		)
	} else {
		command = shellDisplayJoin(safeArgs)
	}
	c.appendSessionLog(fmt.Sprintf("[%s] %s\n", timestamp, command))
}

// logRoutedCommand records a command whose SSH destination was selected by a
// higher-level operation (for example a QDB or FST peer). sshArgs excludes the
// final remote-command argv entry; args is rendered as the logical command so
// the audit line is both route-accurate and readable.
func (c *Client) logRoutedCommand(sshArgs, args []string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	command := "ssh " + shellDisplayJoin(sanitizeLogStrings(sshArgs))
	if len(args) > 0 {
		command += " " + shellDisplayJoin(sanitizeLogStrings(args))
	}
	c.appendSessionLog(fmt.Sprintf("[%s] %s\n", timestamp, command))
}

func (c *Client) logResponse(args []string, output []byte, err error) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	// Abbreviate very long output to avoid flooding the log.
	preview := strings.TrimSpace(string(output))
	const maxPreview = 500
	if len(preview) > maxPreview {
		preview = preview[:maxPreview] + "...(truncated)"
	}
	var cmdStr string
	if len(args) > 0 {
		cmdStr = sanitizeLogText(args[len(args)-1]) // last arg as a short label
	}

	var entry strings.Builder
	entry.WriteString(fmt.Sprintf("[%s] ERROR (%s): %s\n", timestamp, cmdStr, sanitizeLogText(fmt.Sprint(err))))
	if preview != "" {
		entry.WriteString(fmt.Sprintf("[%s]   output: %s\n", timestamp, sanitizeLogText(preview)))
	}
	c.appendSessionLog(entry.String())
}

func (c *Client) appendSessionLog(entry string) {
	c.sessionCommandMu.Lock()
	defer c.sessionCommandMu.Unlock()

	f, err := c.openLogFile()
	if err != nil {
		return
	}
	defer f.Close()

	_, _ = f.WriteString(entry)
}

func sanitizeLogStrings(values []string) []string {
	sanitized := make([]string, len(values))
	for i, value := range values {
		sanitized[i] = sanitizeLogText(value)
	}
	return sanitized
}

func sanitizeLogText(value string) string {
	var sanitized strings.Builder
	for _, r := range value {
		switch r {
		case '\n':
			sanitized.WriteString(`\n`)
		case '\r':
			sanitized.WriteString(`\r`)
		case '\t':
			sanitized.WriteString(`\t`)
		default:
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
				fmt.Fprintf(&sanitized, `\u%04x`, r)
				continue
			}
			sanitized.WriteRune(r)
		}
	}
	return sanitized.String()
}
