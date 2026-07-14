package eos

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

func (c *Client) Groups(ctx context.Context) ([]GroupRecord, error) {
	output, err := c.runCommandContext(ctx, "eos", "-j", "-b", "group", "ls")
	if err != nil {
		return nil, fmt.Errorf("eos group ls: %w", err)
	}

	var payload struct {
		Result []struct {
			Name string `json:"name"`
			NoFS int    `json:"nofs"`
			Cfg  struct {
				Status string `json:"status"`
			} `json:"cfg"`
			Sum struct {
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
		return nil, fmt.Errorf("parse group ls: %w (output: %.200s)", err, output)
	}

	groups := make([]GroupRecord, 0, len(payload.Result))
	for _, item := range payload.Result {
		groups = append(groups, GroupRecord{
			Name:          item.Name,
			Status:        item.Cfg.Status,
			NoFS:          item.NoFS,
			CapacityBytes: item.Sum.Stat.StatFS.Capacity,
			UsedBytes:     item.Sum.Stat.StatFS.UsedBytes,
			FreeBytes:     item.Sum.Stat.StatFS.FreeBytes,
			NumFiles:      item.Sum.Stat.StatFS.Files,
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})

	return groups, nil
}

func (c *Client) SetGroupStatus(ctx context.Context, group, status string) error {
	args, err := groupSetArgs(group, status)
	if err != nil {
		return err
	}

	_, err = c.runCommandContext(ctx, args...)
	if err != nil {
		return fmt.Errorf("eos group set %s %s: %w", group, status, err)
	}

	return nil
}

func groupSetArgs(group, status string) ([]string, error) {
	group = strings.TrimSpace(group)
	status = strings.TrimSpace(status)
	if group == "" {
		return nil, fmt.Errorf("group name is required")
	}

	switch status {
	case "on", "off", "drain":
	default:
		return nil, fmt.Errorf("unsupported group status %q", status)
	}

	return []string{"eos", "-b", "group", "set", group, status}, nil
}
