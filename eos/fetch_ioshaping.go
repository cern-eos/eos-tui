package eos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrIOShapingUnsupported is returned when the connected EOS instance does
// not implement the `io shaping` subcommand at all (older builds expose only
// `io stat` / `io enable`). Callers can surface a friendly message instead
// of dumping the raw usage text.
var ErrIOShapingUnsupported = errors.New("io shaping is not supported by this EOS version")

// looksUnsupported recognises the response that EOS gives when an unknown
// `io` subcommand is supplied: exit status 22 plus a usage block that lists
// the supported `io` subcommands but does not mention `shaping`.
func looksUnsupported(err error, output []byte) bool {
	if err == nil {
		return false
	}
	if !strings.Contains(err.Error(), "exit status 22") {
		return false
	}
	text := string(output)
	if !strings.Contains(text, "usage:") {
		return false
	}
	// The genuine `io shaping` usage text starts with `io shaping`. The
	// fallback usage that EOS prints for an unknown subcommand starts with
	// `io stat` / `io enable` and never names `shaping`.
	return !strings.Contains(text, "io shaping") && !strings.Contains(text, "io_shaping")
}

func looksPressureUnsupported(err error, output []byte) bool {
	if err == nil {
		return false
	}
	if !strings.Contains(err.Error(), "exit status 22") {
		return false
	}
	text := string(output)
	if !strings.Contains(text, "usage:") {
		return false
	}
	return !strings.Contains(text, "pressure ls") && !strings.Contains(text, "io shaping pressure")
}

func (c *Client) IOShaping(ctx context.Context, mode IOShapingMode) ([]IOShapingRecord, error) {
	flag := "--apps"
	switch mode {
	case IOShapingUsers:
		flag = "--users"
	case IOShapingGroups:
		flag = "--groups"
	case IOShapingNodes:
		flag = "--nodes"
	}
	output, err := c.runCommandContext(ctx, "eos", "io", "shaping", "ls", flag, "--json", "--window", "5")
	if err != nil {
		if looksUnsupported(err, output) {
			return nil, ErrIOShapingUnsupported
		}
		return nil, fmt.Errorf("io shaping ls: %w: %s", err, strings.TrimSpace(string(output)))
	}

	var raw []struct {
		ID        string  `json:"id"`
		Type      string  `json:"type"`
		WindowSec int     `json:"window_sec"`
		ReadBps   float64 `json:"read_rate_bps"`
		WriteBps  float64 `json:"write_rate_bps"`
		ReadIOPS  float64 `json:"read_iops"`
		WriteIOPS float64 `json:"write_iops"`
	}
	if err := json.Unmarshal(stripEOSPreamble(output), &raw); err != nil {
		return nil, fmt.Errorf("parse io shaping: %w", err)
	}

	records := make([]IOShapingRecord, len(raw))
	for i, r := range raw {
		records[i] = IOShapingRecord{
			ID:        r.ID,
			Type:      r.Type,
			WindowSec: r.WindowSec,
			ReadBps:   r.ReadBps,
			WriteBps:  r.WriteBps,
			ReadIOPS:  r.ReadIOPS,
			WriteIOPS: r.WriteIOPS,
		}
	}
	return records, nil
}

func (c *Client) IOShapingPressure(ctx context.Context) ([]IOShapingPressureRecord, error) {
	output, err := c.runCommandContext(ctx, "eos", "io", "shaping", "pressure", "ls", "--json")
	if err != nil {
		if looksUnsupported(err, output) || looksPressureUnsupported(err, output) {
			return nil, ErrIOShapingUnsupported
		}
		return nil, fmt.Errorf("io shaping pressure ls: %w: %s", err, strings.TrimSpace(string(output)))
	}

	var raw []struct {
		Type                              string  `json:"type"`
		App                               string  `json:"app"`
		NodeID                            string  `json:"node_id"`
		NodeIOPressure                    float64 `json:"node_io_pressure"`
		HasNodeIOPressure                 bool    `json:"has_node_io_pressure"`
		ReadRateBps                       float64 `json:"read_rate_bps"`
		WriteRateBps                      float64 `json:"write_rate_bps"`
		GlobalReadRateBps                 float64 `json:"global_read_rate_bps"`
		GlobalWriteRateBps                float64 `json:"global_write_rate_bps"`
		ReservationReadBytesPerSec        float64 `json:"reservation_read_bytes_per_sec"`
		ReservationWriteBytesPerSec       float64 `json:"reservation_write_bytes_per_sec"`
		ReadReservationDeficitBps         float64 `json:"read_reservation_deficit_bps"`
		WriteReservationDeficitBps        float64 `json:"write_reservation_deficit_bps"`
		ReadPressureActive                bool    `json:"read_pressure_active"`
		WritePressureActive               bool    `json:"write_pressure_active"`
		ReadReservationDeficitActive      bool    `json:"read_reservation_deficit_active"`
		WriteReservationDeficitActive     bool    `json:"write_reservation_deficit_active"`
		ReadTriggersCompetitorThrottling  bool    `json:"read_triggers_competitor_throttling"`
		WriteTriggersCompetitorThrottling bool    `json:"write_triggers_competitor_throttling"`
		NodeHasPressuredReadReservation   bool    `json:"node_has_pressured_read_reservation"`
		NodeHasPressuredWriteReservation  bool    `json:"node_has_pressured_write_reservation"`
	}
	if err := json.Unmarshal(stripEOSPreamble(output), &raw); err != nil {
		return nil, fmt.Errorf("parse io shaping pressure: %w", err)
	}

	records := make([]IOShapingPressureRecord, len(raw))
	for i, r := range raw {
		records[i] = IOShapingPressureRecord{
			Type:                              r.Type,
			App:                               r.App,
			NodeID:                            r.NodeID,
			NodeIOPressure:                    r.NodeIOPressure,
			HasNodeIOPressure:                 r.HasNodeIOPressure,
			ReadRateBps:                       r.ReadRateBps,
			WriteRateBps:                      r.WriteRateBps,
			GlobalReadRateBps:                 r.GlobalReadRateBps,
			GlobalWriteRateBps:                r.GlobalWriteRateBps,
			ReservationReadBytesPerSec:        r.ReservationReadBytesPerSec,
			ReservationWriteBytesPerSec:       r.ReservationWriteBytesPerSec,
			ReadReservationDeficitBps:         r.ReadReservationDeficitBps,
			WriteReservationDeficitBps:        r.WriteReservationDeficitBps,
			ReadPressureActive:                r.ReadPressureActive,
			WritePressureActive:               r.WritePressureActive,
			ReadReservationDeficitActive:      r.ReadReservationDeficitActive,
			WriteReservationDeficitActive:     r.WriteReservationDeficitActive,
			ReadTriggersCompetitorThrottling:  r.ReadTriggersCompetitorThrottling,
			WriteTriggersCompetitorThrottling: r.WriteTriggersCompetitorThrottling,
			NodeHasPressuredReadReservation:   r.NodeHasPressuredReadReservation,
			NodeHasPressuredWriteReservation:  r.NodeHasPressuredWriteReservation,
		}
	}
	return records, nil
}

func (c *Client) IOShapingPolicies(ctx context.Context) ([]IOShapingPolicyRecord, error) {
	output, err := c.runCommandContext(ctx, "eos", "io", "shaping", "policy", "ls", "--json")
	if err != nil {
		if looksUnsupported(err, output) {
			return nil, ErrIOShapingUnsupported
		}
		return nil, fmt.Errorf("io shaping policy ls: %w: %s", err, strings.TrimSpace(string(output)))
	}

	var raw []struct {
		ID                          string  `json:"id"`
		Type                        string  `json:"type"`
		Enabled                     bool    `json:"is_enabled"`
		LimitReadBytesPerSec        float64 `json:"limit_read_bytes_per_sec"`
		LimitWriteBytesPerSec       float64 `json:"limit_write_bytes_per_sec"`
		ReservationReadBytesPerSec  float64 `json:"reservation_read_bytes_per_sec"`
		ReservationWriteBytesPerSec float64 `json:"reservation_write_bytes_per_sec"`
	}
	if err := json.Unmarshal(stripEOSPreamble(output), &raw); err != nil {
		return nil, fmt.Errorf("parse io shaping policy: %w", err)
	}

	records := make([]IOShapingPolicyRecord, len(raw))
	for i, r := range raw {
		records[i] = IOShapingPolicyRecord{
			ID:                          r.ID,
			Type:                        r.Type,
			Enabled:                     r.Enabled,
			LimitReadBytesPerSec:        r.LimitReadBytesPerSec,
			LimitWriteBytesPerSec:       r.LimitWriteBytesPerSec,
			ReservationReadBytesPerSec:  r.ReservationReadBytesPerSec,
			ReservationWriteBytesPerSec: r.ReservationWriteBytesPerSec,
		}
	}
	return records, nil
}

func (c *Client) IOShapingConfig(ctx context.Context) (IOShapingConfig, error) {
	output, err := c.runCommandContext(ctx, "eos", "io", "shaping", "config", "ls", "--json")
	if err != nil {
		if looksUnsupported(err, output) {
			return IOShapingConfig{}, ErrIOShapingUnsupported
		}
		return IOShapingConfig{}, fmt.Errorf("io shaping config ls: %w: %s", err, strings.TrimSpace(string(output)))
	}

	var raw struct {
		LimitsEnabled bool `json:"limits_enabled"`
	}
	if err := json.Unmarshal(stripEOSPreamble(output), &raw); err != nil {
		return IOShapingConfig{}, fmt.Errorf("parse io shaping config: %w", err)
	}

	return IOShapingConfig{LimitsEnabled: raw.LimitsEnabled}, nil
}

func (c *Client) SetIOShapingLimitsEnabled(ctx context.Context, enabled bool) error {
	state := "disabled"
	if enabled {
		state = "enabled"
	}
	if _, err := c.runCommandContext(ctx, "eos", "io", "shaping", "config", "set", "--limits", state); err != nil {
		return fmt.Errorf("eos io shaping config set --limits %s: %w", state, err)
	}
	return nil
}

func (c *Client) SetIOShapingPolicy(ctx context.Context, update IOShapingPolicyUpdate) error {
	args, err := ioShapingPolicySetArgs(update)
	if err != nil {
		return err
	}

	if _, err := c.runCommandContext(ctx, args...); err != nil {
		return fmt.Errorf("eos io shaping policy set %s: %w", update.ID, err)
	}
	return nil
}

func (c *Client) RemoveIOShapingPolicy(ctx context.Context, mode IOShapingMode, id string) error {
	args, err := ioShapingPolicyRemoveArgs(mode, id)
	if err != nil {
		return err
	}
	if _, err := c.runCommandContext(ctx, args...); err != nil {
		return fmt.Errorf("eos io shaping policy rm %s: %w", id, err)
	}
	return nil
}

func ioShapingPolicySetArgs(update IOShapingPolicyUpdate) ([]string, error) {
	targetFlag, err := ioShapingPolicyTargetFlag(update.Mode)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(update.ID) == "" {
		return nil, fmt.Errorf("io shaping policy id is required")
	}

	args := []string{
		"eos", "io", "shaping", "policy", "set",
		targetFlag, update.ID,
	}
	if update.Enabled {
		args = append(args, "--enable")
	} else {
		args = append(args, "--disable")
	}
	args = append(args,
		"--limit-read", strconv.FormatUint(update.LimitReadBytesPerSec, 10),
		"--limit-write", strconv.FormatUint(update.LimitWriteBytesPerSec, 10),
		"--reservation-read", strconv.FormatUint(update.ReservationReadBytesPerSec, 10),
		"--reservation-write", strconv.FormatUint(update.ReservationWriteBytesPerSec, 10),
	)
	return args, nil
}

func ioShapingPolicyTargetFlag(mode IOShapingMode) (string, error) {
	switch mode {
	case IOShapingApps:
		return "--app", nil
	case IOShapingUsers:
		return "--uid", nil
	case IOShapingGroups:
		return "--gid", nil
	default:
		return "", fmt.Errorf("unsupported io shaping mode %d", mode)
	}
}

func ioShapingPolicyRemoveArgs(mode IOShapingMode, id string) ([]string, error) {
	targetFlag, err := ioShapingPolicyTargetFlag(mode)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("io shaping policy id is required")
	}

	return []string{
		"eos", "io", "shaping", "policy", "rm",
		targetFlag, id,
	}, nil
}
