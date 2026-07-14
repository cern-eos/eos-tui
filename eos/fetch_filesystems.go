package eos

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

func (c *Client) FileSystems(ctx context.Context) ([]FileSystemRecord, error) {
	output, err := c.runCommandContext(ctx, "eos", "-j", "-b", "fs", "ls")
	if err != nil {
		return nil, fmt.Errorf("eos fs ls: %w", err)
	}

	return parseFileSystemsJSON(output)
}

func parseFileSystemsJSON(output []byte) ([]FileSystemRecord, error) {
	var payload struct {
		Result []struct {
			Host         string `json:"host"`
			Port         uint64 `json:"port"`
			ID           uint64 `json:"id"`
			Path         string `json:"path"`
			SchedGroup   string `json:"schedgroup"`
			ConfigStatus string `json:"configstatus"`
			Local        struct {
				Drain struct {
					Status string `json:"status"`
				} `json:"drain"`
			} `json:"local"`
			Stat struct {
				Active string         `json:"active"`
				Boot   string         `json:"boot"`
				Geotag flexibleString `json:"geotag"`
				Health struct {
					Status string `json:"status"`
				} `json:"health"`
				Disk struct {
					BW          float64 `json:"bw"`
					IOPS        float64 `json:"iops"`
					ReadRateMB  float64 `json:"readratemb"`
					WriteRateMB float64 `json:"writeratemb"`
				} `json:"disk"`
				StatFS struct {
					Capacity  uint64 `json:"capacity"`
					FreeBytes uint64 `json:"freebytes"`
					UsedBytes uint64 `json:"usedbytes"`
				} `json:"statfs"`
				UsedFiles uint64 `json:"usedfiles"`
			} `json:"stat"`
		} `json:"result"`
	}

	if err := unmarshalEOSJSON(output, &payload); err != nil {
		return nil, fmt.Errorf("parse fs ls: %w (output: %.200s)", err, output)
	}

	fileSystems := make([]FileSystemRecord, 0, len(payload.Result))
	for _, item := range payload.Result {
		fileSystems = append(fileSystems, FileSystemRecord{
			Host:          item.Host,
			Port:          item.Port,
			ID:            item.ID,
			Path:          item.Path,
			SchedGroup:    item.SchedGroup,
			Geotag:        string(item.Stat.Geotag),
			Boot:          item.Stat.Boot,
			ConfigStatus:  item.ConfigStatus,
			DrainStatus:   item.Local.Drain.Status,
			Active:        item.Stat.Active,
			Health:        strings.ReplaceAll(item.Stat.Health.Status, "%20", " "),
			CapacityBytes: item.Stat.StatFS.Capacity,
			UsedBytes:     item.Stat.StatFS.UsedBytes,
			FreeBytes:     item.Stat.StatFS.FreeBytes,
			UsedFiles:     item.Stat.UsedFiles,
			DiskBWMB:      item.Stat.Disk.BW,
			DiskIOPS:      item.Stat.Disk.IOPS,
			ReadRateMB:    item.Stat.Disk.ReadRateMB,
			WriteRateMB:   item.Stat.Disk.WriteRateMB,
		})
	}

	sort.Slice(fileSystems, func(i, j int) bool {
		return fileSystems[i].ID < fileSystems[j].ID
	})

	return fileSystems, nil
}

// FsConfigStatus sets the configstatus of a filesystem by its ID.
// Valid values are "rw", "ro", and "" (empty to clear).
func (c *Client) FsConfigStatus(ctx context.Context, fsID uint64, value string) error {
	_, err := c.runCommandContext(ctx, "eos", "-b", "fs", "config", fmt.Sprintf("%d", fsID), fmt.Sprintf("configstatus=%s", value))
	if err != nil {
		return fmt.Errorf("eos fs config %d configstatus=%s: %w", fsID, value, err)
	}
	return nil
}
