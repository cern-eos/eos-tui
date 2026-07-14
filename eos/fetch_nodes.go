package eos

import (
	"context"
	"fmt"
	"sort"
	"time"
)

func (c *Client) NodeStats(ctx context.Context) (NodeStats, error) {
	return c.nodeStatsViaCLI(ctx)
}

func (c *Client) nodeStatsViaCLI(ctx context.Context) (NodeStats, error) {
	// Fetch namespace stats via eos ns stat -m. The monitoring format already
	// contains the counters we need for cluster summary and is also used by the
	// MGM/QDB topology path, so this avoids an extra plain-text ns stat call.
	// State (health) is not fetched here; it is derived in the UI from the
	// already-loaded node and filesystem data, avoiding a redundant call to
	// `eos status` which internally runs the eos-status shell script and
	// creates temporary files under /tmp.
	nsStatOut, err := c.runCommandContext(ctx, "eos", "-b", "ns", "stat", "-m")
	if err != nil {
		return NodeStats{}, fmt.Errorf("eos ns stat -m: %w", err)
	}

	stats := nodeStatsFromMonitoringValues(parseMonitoringKeyValues(nsStatOut))
	if stats.Uptime > 0 {
		stats.BootTime = time.Now().Add(-stats.Uptime)
	}

	return stats, nil
}

func nodeStatsFromMonitoringValues(values map[string]string) NodeStats {
	return NodeStats{
		FileCount:   parseUint(values["ns.total.files"]),
		DirCount:    parseUint(values["ns.total.directories"]),
		CurrentFID:  parseUint(values["ns.current.fid"]),
		CurrentCID:  parseUint(values["ns.current.cid"]),
		MemVirtual:  parseUint(values["ns.memory.virtual"]),
		MemResident: parseUint(values["ns.memory.resident"]),
		MemShared:   parseUint(values["ns.memory.share"]),
		MemGrowth:   parseUint(values["ns.memory.growth"]),
		ThreadCount: parseUint(values["ns.stat.threads"]),
		FileDescs:   parseUint(values["ns.fds.all"]),
		Uptime:      time.Duration(parseUint(values["ns.uptime"])) * time.Second,
	}
}

func (c *Client) Nodes(ctx context.Context) ([]FstRecord, error) {
	output, err := c.runCommandContext(ctx, "eos", "-j", "node", "ls")
	if err != nil {
		return nil, fmt.Errorf("eos node ls: %w", err)
	}

	var payload struct {
		Result []struct {
			Type           string         `json:"type"`
			HostPort       string         `json:"hostport"`
			Geotag         flexibleString `json:"geotag"`
			Status         string         `json:"status"`
			HeartbeatDelta int64          `json:"heartbeatdelta"`
			NoFS           int            `json:"nofs"`
			Cfg            struct {
				Status string `json:"status"`
				Stat   struct {
					Geotag flexibleString `json:"geotag"`
					Sys    struct {
						Kernel  string `json:"kernel"`
						RSS     uint64 `json:"rss"`
						Threads uint64 `json:"threads"`
						Uptime  string `json:"uptime"`
						VSize   uint64 `json:"vsize"`
						EOS     struct {
							Version string `json:"version"`
						} `json:"eos"`
					} `json:"sys"`
				} `json:"stat"`
			} `json:"cfg"`
			Avg struct {
				Stat struct {
					Disk struct {
						Load float64 `json:"load"`
					} `json:"disk"`
				} `json:"stat"`
			} `json:"avg"`
			Sum struct {
				Stat struct {
					Disk struct {
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
			} `json:"sum"`
		} `json:"result"`
	}

	if err := unmarshalEOSJSON(output, &payload); err != nil {
		return nil, fmt.Errorf("parse node ls: %w (output: %.200s)", err, output)
	}

	nodes := make([]FstRecord, 0, len(payload.Result))
	for _, item := range payload.Result {
		geotag := string(item.Geotag)
		if geotag == "" {
			geotag = string(item.Cfg.Stat.Geotag)
		}
		h, p := splitHostPort(item.HostPort)
		nodes = append(nodes, FstRecord{
			Type:            item.Type,
			Host:            h,
			Port:            p,
			Geotag:          geotag,
			Status:          item.Status,
			Activated:       item.Cfg.Status,
			HeartbeatDelta:  item.HeartbeatDelta,
			FileSystemCount: item.NoFS,
			EOSVersion:      item.Cfg.Stat.Sys.EOS.Version,
			Kernel:          item.Cfg.Stat.Sys.Kernel,
			Uptime:          item.Cfg.Stat.Sys.Uptime,
			ThreadCount:     item.Cfg.Stat.Sys.Threads,
			RSSBytes:        item.Cfg.Stat.Sys.RSS,
			VSizeBytes:      item.Cfg.Stat.Sys.VSize,
			CapacityBytes:   item.Sum.Stat.StatFS.Capacity,
			UsedBytes:       item.Sum.Stat.StatFS.UsedBytes,
			FreeBytes:       item.Sum.Stat.StatFS.FreeBytes,
			UsedFiles:       item.Sum.Stat.UsedFiles,
			DiskLoad:        item.Avg.Stat.Disk.Load,
			ReadRateMB:      item.Sum.Stat.Disk.ReadRateMB,
			WriteRateMB:     item.Sum.Stat.Disk.WriteRateMB,
		})
	}

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Host != nodes[j].Host {
			return nodes[i].Host < nodes[j].Host
		}
		return nodes[i].Port < nodes[j].Port
	})

	return nodes, nil
}

func (c *Client) SetNodeStatus(ctx context.Context, host string, port int, status string) error {
	target := fmt.Sprintf("%s:%d", host, port)
	if _, err := c.runCommandContext(ctx, "eos", "node", "set", target, status); err != nil {
		return fmt.Errorf("eos node set %s %s: %w", target, status, err)
	}
	return nil
}
