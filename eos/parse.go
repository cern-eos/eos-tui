package eos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// flexibleString is a JSON-decodable string that also accepts numbers and
// null. EOS sometimes emits numeric values for fields that are normally
// strings (e.g. "geotag":83737789 instead of "geotag":"83737789"); decoding
// such payloads into a plain string field would fail, so use this type for
// any field where EOS may produce either form.
type flexibleString string

func (s *flexibleString) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*s = ""
		return nil
	}
	if trimmed[0] == '"' {
		var str string
		if err := json.Unmarshal(trimmed, &str); err != nil {
			return err
		}
		*s = flexibleString(str)
		return nil
	}
	// Anything else (number, bool, etc.) — preserve the raw token as text.
	*s = flexibleString(string(trimmed))
	return nil
}

// String exists so that fmt and string conversions read naturally at call
// sites.
func (s flexibleString) String() string { return string(s) }

var shellSafeArgPattern = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// stripEOSPreamble extracts the first complete JSON value from mixed EOS
// output. EOS and SSH may add informational lines before or after an otherwise
// valid payload, especially when stdout and stderr are combined.
func stripEOSPreamble(b []byte) []byte {
	_, payload, _, ok := extractEOSJSON(b)
	if ok {
		return payload
	}
	return b
}

// extractEOSJSON finds the first complete JSON value in mixed command output.
// Invalid bracket-prefixed banners are skipped rather than hiding a valid JSON
// value on a later line. The returned remainder begins immediately after the
// decoded value and is used to detect trailing EOS error annotations.
func extractEOSJSON(b []byte) (prefix, payload, remainder []byte, ok bool) {
	offset := 0
	for _, line := range strings.SplitAfter(string(b), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
			lineOffset := strings.Index(line, trimmed)
			if lineOffset >= 0 {
				start := offset + lineOffset
				candidate := b[start:]
				decoder := json.NewDecoder(bytes.NewReader(candidate))
				var raw json.RawMessage
				if err := decoder.Decode(&raw); err == nil {
					consumed := decoder.InputOffset()
					return bytes.TrimSpace(b[:start]), bytes.TrimSpace(raw), bytes.TrimSpace(candidate[consumed:]), true
				}
			}
		}
		offset += len(line)
	}
	return nil, nil, nil, false
}

// unmarshalEOSJSON decodes EOS machine output and rejects command-level error
// envelopes even when the eos process exits with status 0.
func unmarshalEOSJSON(output []byte, target any) error {
	prefix, payload, remainder, ok := extractEOSJSON(output)
	if !ok {
		payload = bytes.TrimSpace(output)
	}
	if detail := eosJSONAnnotationError(prefix); detail != "" && !eosJSONKnownFallbackNoise(detail, payload) {
		return fmt.Errorf("EOS command reported an error before its JSON payload: %s", detail)
	}
	if err := eosJSONStatusError(payload); err != nil {
		return err
	}
	if detail := eosJSONAnnotationError(remainder); detail != "" {
		return fmt.Errorf("EOS command reported an error after its JSON payload: %s", detail)
	}
	return json.Unmarshal(payload, target)
}

func eosJSONKnownFallbackNoise(detail string, payload []byte) bool {
	lower := strings.ToLower(detail)
	localhostFailure := strings.Contains(lower, "localhost") &&
		(strings.Contains(lower, "cannot connect") || strings.Contains(lower, "failed to connect") || strings.Contains(lower, "connection refused"))
	if !localhostFailure {
		return false
	}
	return eosJSONExplicitSuccessStatus(payload)
}

func eosJSONExplicitSuccessStatus(payload []byte) bool {
	var status struct {
		Retc json.RawMessage `json:"retc"`
	}
	if err := json.Unmarshal(payload, &status); err != nil || len(status.Retc) == 0 {
		return false
	}
	retc := strings.Trim(strings.TrimSpace(string(status.Retc)), `"`)
	return retc == "0"
}

func eosJSONStatusError(payload []byte) error {
	if len(payload) == 0 || payload[0] != '{' {
		return nil
	}
	var status struct {
		Retc     json.RawMessage `json:"retc"`
		ErrorMsg string          `json:"errormsg"`
	}
	if err := json.Unmarshal(payload, &status); err != nil || len(status.Retc) == 0 {
		return nil
	}
	retc := strings.Trim(strings.TrimSpace(string(status.Retc)), `"`)
	if retc == "" || retc == "0" {
		return nil
	}
	detail := strings.TrimSpace(status.ErrorMsg)
	if detail == "" {
		detail = "unspecified EOS error"
	}
	return fmt.Errorf("%s (retc %s)", detail, retc)
}

func eosJSONAnnotationError(output []byte) string {
	for _, rawLine := range strings.Split(string(output), "\n") {
		line := strings.TrimSpace(rawLine)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "* error") || strings.HasPrefix(lower, "error:") {
			return line
		}
	}
	return ""
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	return strings.Join(quoted, " ")
}

func shellDisplayJoin(args []string) string {
	display := make([]string, len(args))
	for i, arg := range args {
		if arg != "" && shellSafeArgPattern.MatchString(arg) {
			display[i] = arg
		} else {
			display[i] = shellQuote(arg)
		}
	}
	return strings.Join(display, " ")
}

// ShellJoin is the exported version of shellJoin for use by other packages.
func ShellJoin(args []string) string { return shellJoin(args) }

// ShellDisplayJoin is the exported version of shellDisplayJoin for use by other packages.
func ShellDisplayJoin(args []string) string { return shellDisplayJoin(args) }

func normalizeClusterInstance(target string) string {
	target = strings.TrimSpace(target)
	if target == "" || strings.ContainsAny(target, " \t\r\n") {
		return ""
	}

	if at := strings.LastIndex(target, "@"); at != -1 {
		target = target[at+1:]
	}
	target = hostOnly(target)
	target = strings.Split(target, ".")[0]
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}

	for _, marker := range []string{"-ns-", "-mgm-", "-qdb-", "-fst-"} {
		if idx := strings.Index(target, marker); idx > 0 {
			target = target[:idx]
			break
		}
	}
	for _, suffix := range []string{"-ns", "-mgm", "-qdb", "-fst"} {
		if strings.HasSuffix(target, suffix) {
			target = strings.TrimSuffix(target, suffix)
			break
		}
	}

	if target == "" || strings.ContainsAny(target, " \t\r\n") {
		return ""
	}
	return target
}

// NormalizeClusterInstance extracts the logical EOS instance name from a
// cluster alias or hostname such as "eospilot", "root@eospilot.cern.ch", or
// "root@eospilot-ns-02.cern.ch".
func NormalizeClusterInstance(target string) string { return normalizeClusterInstance(target) }

func toUint64(v any) uint64 {
	switch val := v.(type) {
	case float64:
		return uint64(val)
	case uint64:
		return val
	case int64:
		return uint64(val)
	case int:
		return uint64(val)
	default:
		return 0
	}
}

var nsStatLinePattern = regexp.MustCompile(`^ALL\s+(.+?)\s{2,}(.+)$`)

func parseLabeledValues(output string) map[string]string {
	values := make(map[string]string)
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		matches := nsStatLinePattern.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}

		values[strings.TrimSpace(matches[1])] = strings.TrimSpace(matches[2])
	}

	return values
}

func parseUint(raw string) uint64 {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return 0
	}

	value, _ := strconv.ParseUint(fields[0], 10, 64)
	return value
}

func parseHumanBytes(raw string) uint64 {
	fields := strings.Fields(raw)
	if len(fields) < 2 {
		return 0
	}

	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}

	multiplier := float64(1)
	switch strings.ToUpper(fields[1]) {
	case "KB":
		multiplier = 1024
	case "MB":
		multiplier = 1024 * 1024
	case "GB":
		multiplier = 1024 * 1024 * 1024
	case "TB":
		multiplier = 1024 * 1024 * 1024 * 1024
	}

	return uint64(value * multiplier)
}

func cleanPath(rawPath string) string {
	if rawPath == "" {
		return "/"
	}

	cleaned := path.Clean(rawPath)
	if !strings.HasPrefix(cleaned, "/") {
		return "/" + cleaned
	}

	return cleaned
}

func splitHostPort(hp string) (string, int) {
	parts := strings.Split(hp, ":")
	host := parts[0]
	port := 0
	if len(parts) > 1 {
		port, _ = strconv.Atoi(parts[1])
	}
	return host, port
}

// hostOnly strips the port suffix from a host:port string.
func hostOnly(hostPort string) string {
	if idx := strings.LastIndex(hostPort, ":"); idx != -1 {
		return hostPort[:idx]
	}
	return hostPort
}

// HostOnly is the exported version of hostOnly for use by other packages.
func HostOnly(hostPort string) string { return hostOnly(hostPort) }
