package eos

import (
	"context"
	"fmt"
	"strings"
)

func (c *Client) QDBAttemptCoup(ctx context.Context, host string) ([]byte, error) {
	args := QDBCoupArgs()
	out, err := c.runCommandOnHost(ctx, host, args...)
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			return out, fmt.Errorf("redis-cli raft-attempt-coup on %s: %w\n%s", host, err, detail)
		}
		return out, fmt.Errorf("redis-cli raft-attempt-coup on %s: %w", host, err)
	}
	if detail := redisCLIError(out); detail != "" {
		return out, fmt.Errorf("redis-cli raft-attempt-coup on %s: %s", host, detail)
	}
	return out, nil
}

// QDBCoupArgs is shared with the UI command preview so the displayed command
// exactly matches the one that will execute. Do not use redis-cli -e here: it
// was introduced in Redis 6.2, while supported EOS hosts may still carry
// older clients. redisCLIError provides version-independent error detection.
func QDBCoupArgs() []string {
	return []string{"redis-cli", "-p", "7777", "raft-attempt-coup"}
}

// redis-cli historically exits successfully for some server-side errors.
// Detect the standard RESP error prefixes so a failed coup is never reported
// to the operator as successful merely because the process exit code was 0.
func redisCLIError(output []byte) string {
	for _, rawLine := range strings.Split(string(output), "\n") {
		line := strings.TrimSpace(rawLine)
		upper := strings.ToUpper(line)
		for _, prefix := range []string{
			"(ERROR)", "-ERR ", "ERR ", "NOAUTH ", "WRONGTYPE ", "CLUSTERDOWN ",
			"READONLY ", "MISCONF ", "LOADING ", "MOVED ", "ASK ", "MASTERDOWN ",
			"BUSY ", "NOSCRIPT ", "OOM ", "EXECABORT ",
		} {
			if strings.HasPrefix(upper, prefix) {
				return line
			}
		}
	}
	return ""
}
