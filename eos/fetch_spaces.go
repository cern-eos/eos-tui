package eos

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func (c *Client) Spaces(ctx context.Context) ([]SpaceRecord, error) {
	output, err := c.runCommandContext(ctx, "eos", "-j", "-b", "space", "ls")
	if err != nil {
		return nil, fmt.Errorf("eos space ls: %w", err)
	}

	var payload struct {
		Result []struct {
			Name string `json:"name"`
			Type string `json:"type"`
			Cfg  struct {
				GroupSize uint64 `json:"groupsize"`
			} `json:"cfg"`
			Sum struct {
				NRW  uint64 `json:"n_rw"`
				Stat struct {
					StatFS struct {
						Capacity  uint64 `json:"capacity"`
						UsedBytes uint64 `json:"usedbytes"`
						FreeBytes uint64 `json:"freebytes"`
						Files     uint64 `json:"files"`
					} `json:"statfs"`
				} `json:"stat"`
			} `json:"sum"`
		} `json:"result"`
	}

	if err := unmarshalEOSJSON(output, &payload); err != nil {
		return nil, fmt.Errorf("parse space ls: %w (output: %.200s)", err, output)
	}

	spaces := make([]SpaceRecord, 0, len(payload.Result))
	for _, item := range payload.Result {
		spaces = append(spaces, SpaceRecord{
			Name:          item.Name,
			Type:          item.Type,
			Status:        "active",
			Groups:        item.Cfg.GroupSize,
			NumFiles:      item.Sum.Stat.StatFS.Files,
			NumContainers: item.Sum.NRW,
			CapacityBytes: item.Sum.Stat.StatFS.Capacity,
			UsedBytes:     item.Sum.Stat.StatFS.UsedBytes,
			FreeBytes:     item.Sum.Stat.StatFS.FreeBytes,
		})
	}

	sort.Slice(spaces, func(i, j int) bool {
		return spaces[i].Name < spaces[j].Name
	})

	return spaces, nil
}

func (c *Client) SpaceStatus(ctx context.Context, name string) ([]SpaceStatusRecord, error) {
	output, err := c.runCommandContext(ctx, "eos", "--json", "-b", "space", "status", name)
	if err == nil {
		records, parseErr := parseSpaceStatusJSON(output)
		if parseErr == nil && len(records) > 0 {
			return records, nil
		}
	}

	// TEMPORARY COMPATIBILITY WORKAROUND: older EOS clients still ship a broken
	// JSON implementation for `eos space status`. Keep the legacy text parser as
	// a fallback until those versions are out of support, then remove this block
	// and the parseSpaceStatusLegacy helper.
	legacyOutput, legacyErr := c.runCommandContext(ctx, "eos", "-b", "space", "status", name)
	if legacyErr != nil {
		if err != nil {
			return nil, fmt.Errorf("eos space status %s: json path failed: %w; legacy fallback failed: %w", name, err, legacyErr)
		}
		return nil, fmt.Errorf("eos space status %s: %w", name, legacyErr)
	}

	return parseSpaceStatusLegacy(legacyOutput), nil
}

func (c *Client) SpaceConfig(ctx context.Context, name string, key, value string) error {
	fullKey := key
	if !strings.HasPrefix(key, "space.") && !strings.HasPrefix(key, "fs.") {
		fullKey = "space." + key
	}

	_, err := c.runCommandContext(ctx, "eos", "-b", "space", "config", name, fmt.Sprintf("%s=%s", fullKey, value))
	if err != nil {
		return fmt.Errorf("eos space config %s %s=%s: %w", name, fullKey, value, err)
	}

	return nil
}

func parseSpaceStatusJSON(output []byte) ([]SpaceStatusRecord, error) {
	var payload struct {
		Result []map[string]any `json:"result"`
	}

	if err := unmarshalEOSJSON(output, &payload); err != nil {
		return nil, fmt.Errorf("parse space status json: %w", err)
	}
	if len(payload.Result) == 0 {
		return nil, nil
	}

	flattened := make(map[string]string)
	flattenSpaceStatusMap(flattened, "", payload.Result[0])
	if len(flattened) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(flattened))
	for key := range flattened {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	records := make([]SpaceStatusRecord, 0, len(keys))
	for _, key := range keys {
		records = append(records, SpaceStatusRecord{
			Key:   key,
			Value: flattened[key],
		})
	}
	return records, nil
}

func flattenSpaceStatusMap(dst map[string]string, prefix string, value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			next := key
			if prefix != "" {
				next = prefix + "." + key
			}
			flattenSpaceStatusMap(dst, next, child)
		}
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, formatSpaceStatusValue(item))
		}
		dst[prefix] = strings.Join(parts, ",")
	case nil:
		dst[prefix] = ""
	default:
		dst[prefix] = formatSpaceStatusValue(typed)
	}
}

func formatSpaceStatusValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return fmt.Sprint(typed)
	}
}

// parseSpaceStatus is kept as a compatibility shim for existing tests and
// callers that still expect the legacy text parser name.
func parseSpaceStatus(output []byte) []SpaceStatusRecord {
	return parseSpaceStatusLegacy(output)
}

func parseSpaceStatusLegacy(output []byte) []SpaceStatusRecord {
	lines := strings.Split(string(output), "\n")
	records := make([]SpaceStatusRecord, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":=", 2)
		if len(parts) != 2 {
			continue
		}

		records = append(records, SpaceStatusRecord{
			Key:   strings.TrimSpace(parts[0]),
			Value: strings.TrimSpace(parts[1]),
		})
	}

	return records
}
