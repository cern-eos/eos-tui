package eos

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func (c *Client) Inspector(ctx context.Context) (InspectorStats, error) {
	output, err := c.runCommandContext(ctx, "eos", "inspector", "-l", "-m")
	if reported := inspectorReportedError(output); reported != nil {
		return InspectorStats{}, reported
	}
	if err != nil {
		msg := strings.TrimSpace(string(output))
		lower := strings.ToLower(msg)
		switch {
		case strings.Contains(lower, "permission denied"):
			return InspectorStats{}, fmt.Errorf("inspector unavailable on this host: permission denied")
		case strings.Contains(lower, "inspector disabled"):
			return InspectorStats{}, fmt.Errorf("inspector disabled")
		case msg != "":
			return InspectorStats{}, fmt.Errorf("eos inspector -l -m: %s", msg)
		default:
			return InspectorStats{}, fmt.Errorf("eos inspector -l -m: %w", err)
		}
	}

	return parseInspectorStats(output), nil
}

// inspectorReportedError handles an EOS quirk where `eos inspector` can emit
// a machine-readable error row while still returning exit status 0.
func inspectorReportedError(output []byte) error {
	text := strings.TrimSpace(string(output))
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "inspector disabled"):
		return fmt.Errorf("inspector disabled")
	case strings.Contains(lower, "permission denied"):
		return fmt.Errorf("inspector unavailable on this host: permission denied")
	}

	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.Contains(strings.ToLower(line), "key=error") {
			continue
		}
		detail := line
		if _, value, found := strings.Cut(line, "msg="); found {
			detail = strings.Trim(strings.TrimSpace(value), `"`)
		}
		return fmt.Errorf("inspector unavailable: %s", detail)
	}
	return nil
}

func parseInspectorStats(output []byte) InspectorStats {
	stats := InspectorStats{}
	for _, rawLine := range strings.Split(string(output), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "* ") {
			continue
		}

		fields := parseInspectorFields(line)
		tag := fields["tag"]
		switch tag {
		case "summary::avg_filesize":
			stats.AvgFileSize = parseUint(fields["value"])
		case "links::hardlink_count":
			stats.HardlinkCount = parseUint(fields["value"])
		case "links::hardlink_volume":
			stats.HardlinkVolume = parseUint(fields["value"])
		case "links::symlink_count":
			stats.SymlinkCount = parseUint(fields["value"])
		case "user::cost::disk":
			record := InspectorCostRecord{
				Name:    fallbackString(fields["username"], fields["uid"]),
				ID:      parseUint(fields["uid"]),
				Cost:    parseFloat(fields["cost"]),
				TBYears: parseFloat(fields["tbyears"]),
			}
			stats.UserCosts = append(stats.UserCosts, record)
			if record.Cost > stats.TopUserCost.Cost {
				stats.TopUserCost = record
			}
		case "group::cost::disk":
			record := InspectorCostRecord{
				Name:    fallbackString(fields["groupname"], fields["gid"]),
				ID:      parseUint(fields["gid"]),
				Cost:    parseFloat(fields["cost"]),
				TBYears: parseFloat(fields["tbyears"]),
			}
			stats.GroupCosts = append(stats.GroupCosts, record)
			if record.Cost > stats.TopGroupCost.Cost {
				stats.TopGroupCost = record
			}
		case "accesstime::files":
			stats.AccessFiles = upsertInspectorBin(stats.AccessFiles, parseUint(fields["bin"]), parseUint(fields["value"]))
		case "accesstime::volume":
			stats.AccessVolume = upsertInspectorBin(stats.AccessVolume, parseUint(fields["bin"]), parseUint(fields["value"]))
		case "birthtime::files":
			stats.BirthFiles = upsertInspectorBin(stats.BirthFiles, parseUint(fields["bin"]), parseUint(fields["value"]))
		case "birthtime::volume":
			stats.BirthVolume = upsertInspectorBin(stats.BirthVolume, parseUint(fields["bin"]), parseUint(fields["value"]))
		}

		if layout := fields["layout"]; layout != "" {
			stats.LayoutCount++
			record := InspectorLayoutSummary{
				Layout:        layout,
				Type:          fields["type"],
				VolumeBytes:   parseUint(fields["volume"]),
				PhysicalBytes: parseUint(fields["physicalsize"]),
				Locations:     parseUint(fields["locations"]),
			}
			stats.Layouts = append(stats.Layouts, record)
			if record.VolumeBytes > stats.TopLayout.VolumeBytes {
				stats.TopLayout = record
			}
		}
	}

	sort.Slice(stats.Layouts, func(i, j int) bool {
		if stats.Layouts[i].VolumeBytes != stats.Layouts[j].VolumeBytes {
			return stats.Layouts[i].VolumeBytes > stats.Layouts[j].VolumeBytes
		}
		return stats.Layouts[i].Layout < stats.Layouts[j].Layout
	})
	sort.Slice(stats.UserCosts, func(i, j int) bool {
		if stats.UserCosts[i].Cost != stats.UserCosts[j].Cost {
			return stats.UserCosts[i].Cost > stats.UserCosts[j].Cost
		}
		return stats.UserCosts[i].Name < stats.UserCosts[j].Name
	})
	sort.Slice(stats.GroupCosts, func(i, j int) bool {
		if stats.GroupCosts[i].Cost != stats.GroupCosts[j].Cost {
			return stats.GroupCosts[i].Cost > stats.GroupCosts[j].Cost
		}
		return stats.GroupCosts[i].Name < stats.GroupCosts[j].Name
	})
	sortInspectorBins(stats.AccessFiles)
	sortInspectorBins(stats.AccessVolume)
	sortInspectorBins(stats.BirthFiles)
	sortInspectorBins(stats.BirthVolume)

	return stats
}

func parseInspectorFields(line string) map[string]string {
	fields := make(map[string]string)
	for _, token := range strings.Fields(line) {
		key, value, found := strings.Cut(token, "=")
		if found {
			fields[key] = value
		}
	}
	return fields
}

func parseFloat(raw string) float64 {
	value, _ := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	return value
}

func fallbackString(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return strings.TrimSpace(defaultValue)
	}
	return strings.TrimSpace(value)
}

func upsertInspectorBin(bins []InspectorBin, binSeconds, value uint64) []InspectorBin {
	for i := range bins {
		if bins[i].BinSeconds == binSeconds {
			bins[i].Value = value
			return bins
		}
	}
	return append(bins, InspectorBin{BinSeconds: binSeconds, Value: value})
}

func sortInspectorBins(bins []InspectorBin) {
	sort.Slice(bins, func(i, j int) bool {
		return bins[i].BinSeconds < bins[j].BinSeconds
	})
}
