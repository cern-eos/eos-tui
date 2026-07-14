package eos

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

func (c *Client) NamespaceStats(ctx context.Context) (NamespaceStats, error) {
	output, err := c.runCommandContext(ctx, "eos", "-j", "-b", "ns", "stat")
	if err != nil {
		return NamespaceStats{}, fmt.Errorf("eos ns stat: %w", err)
	}

	return parseNamespaceStats(output)
}

// parseNamespaceStats merges the sparse rows emitted by `eos -j ns stat`.
//
// EOS emits one metric per result row on current releases rather than a
// single fully-populated object. Pointer fields are intentional: zero is a
// valid metric value, so presence must be distinguished from an omitted
// field to avoid a later sparse row erasing a value parsed earlier.
func parseNamespaceStats(output []byte) (NamespaceStats, error) {
	var payload struct {
		Result []struct {
			Master *string `json:"master_id"`
			NS     struct {
				Total struct {
					Files       any `json:"files"`
					Directories any `json:"directories"`
				} `json:"total"`
				Current struct {
					FID *uint64 `json:"fid"`
					CID *uint64 `json:"cid"`
				} `json:"current"`
				Generated struct {
					FID *uint64 `json:"fid"`
					CID *uint64 `json:"cid"`
				} `json:"generated"`
				Contention struct {
					Read  *float64 `json:"read"`
					Write *float64 `json:"write"`
				} `json:"contention"`
				Cache struct {
					Files struct {
						MaxSize   *uint64 `json:"maxsize"`
						Occupancy *uint64 `json:"occupancy"`
						Requests  *uint64 `json:"requests"`
						Hits      *uint64 `json:"hits"`
					} `json:"files"`
					Containers struct {
						MaxSize   *uint64 `json:"maxsize"`
						Occupancy *uint64 `json:"occupancy"`
						Requests  *uint64 `json:"requests"`
						Hits      *uint64 `json:"hits"`
					} `json:"containers"`
				} `json:"cache"`
			} `json:"ns"`
		} `json:"result"`
	}

	if err := unmarshalEOSJSON(output, &payload); err != nil {
		return NamespaceStats{}, fmt.Errorf("parse ns stat: %w (output: %.200s)", err, output)
	}

	stats := NamespaceStats{}
	for _, item := range payload.Result {
		if item.Master != nil {
			stats.MasterHost = *item.Master
		}
		if val, ok := numericUint64(item.NS.Total.Files); ok {
			stats.TotalFiles = val
		}
		if val, ok := numericUint64(item.NS.Total.Directories); ok {
			stats.TotalDirectories = val
		}
		assignIfPresent(&stats.CurrentFID, item.NS.Current.FID)
		assignIfPresent(&stats.CurrentCID, item.NS.Current.CID)
		assignIfPresent(&stats.GeneratedFID, item.NS.Generated.FID)
		assignIfPresent(&stats.GeneratedCID, item.NS.Generated.CID)
		assignIfPresent(&stats.ContentionRead, item.NS.Contention.Read)
		assignIfPresent(&stats.ContentionWrite, item.NS.Contention.Write)
		assignIfPresent(&stats.CacheFilesMax, item.NS.Cache.Files.MaxSize)
		assignIfPresent(&stats.CacheFilesOccup, item.NS.Cache.Files.Occupancy)
		assignIfPresent(&stats.CacheFilesRequests, item.NS.Cache.Files.Requests)
		assignIfPresent(&stats.CacheFilesHits, item.NS.Cache.Files.Hits)
		assignIfPresent(&stats.CacheContainersMax, item.NS.Cache.Containers.MaxSize)
		assignIfPresent(&stats.CacheContainersOccup, item.NS.Cache.Containers.Occupancy)
		assignIfPresent(&stats.CacheContainersRequests, item.NS.Cache.Containers.Requests)
		assignIfPresent(&stats.CacheContainersHits, item.NS.Cache.Containers.Hits)
	}

	return stats, nil
}

func numericUint64(value any) (uint64, bool) {
	switch value := value.(type) {
	case float64:
		if value < 0 {
			return 0, false
		}
		return uint64(value), true
	case uint64:
		return value, true
	case int64:
		if value < 0 {
			return 0, false
		}
		return uint64(value), true
	case int:
		if value < 0 {
			return 0, false
		}
		return uint64(value), true
	default:
		return 0, false
	}
}

func assignIfPresent[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}

func (c *Client) ListPath(ctx context.Context, rawPath string) (Directory, error) {
	return c.listPathViaCLI(ctx, rawPath)
}

func (c *Client) StatPath(ctx context.Context, rawPath string) (Entry, error) {
	return c.statPathViaCLI(ctx, rawPath)
}

func (c *Client) ListAttrs(ctx context.Context, rawPath string) ([]NamespaceAttr, error) {
	output, err := c.runCommandContext(ctx, "eos", "attr", "ls", rawPath)
	if err != nil {
		return nil, fmt.Errorf("eos attr ls: %w", err)
	}

	return parseNamespaceAttrs(output), nil
}

func (c *Client) SetAttr(ctx context.Context, rawPath, key, value string, recursive bool) error {
	args := attrSetArgs(rawPath, key, value, recursive)
	_, err := c.runCommandContext(ctx, args...)
	if err != nil {
		return fmt.Errorf("eos attr set: %w", err)
	}
	return nil
}

func (c *Client) Mkdir(ctx context.Context, rawPath string) error {
	_, err := c.runCommandContext(ctx, "eos", "mkdir", rawPath)
	if err != nil {
		return fmt.Errorf("eos mkdir: %w", err)
	}
	return nil
}

func attrSetArgs(rawPath, key, value string, recursive bool) []string {
	args := []string{"eos", "attr"}
	if recursive {
		args = append(args, "-r")
	}
	return append(args, "set", fmt.Sprintf("%s=%s", key, value), rawPath)
}

func (c *Client) statPathViaCLI(ctx context.Context, rawPath string) (Entry, error) {
	info, err := c.fetchCLIFileInfo(ctx, rawPath)
	if err != nil {
		return Entry{}, err
	}

	return entryFromCLI(info), nil
}

func (c *Client) listPathViaCLI(ctx context.Context, rawPath string) (Directory, error) {
	info, err := c.fetchCLIFileInfo(ctx, rawPath)
	if err != nil {
		return Directory{}, err
	}

	entries := make([]Entry, 0, len(info.Children))
	for _, child := range info.Children {
		entry := entryFromCLI(child)
		if entry.Path == cleanPath(rawPath) {
			continue
		}
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind == EntryKindContainer
		}

		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	return Directory{
		Path:    cleanPath(rawPath),
		Self:    entryFromCLI(info),
		Entries: entries,
	}, nil
}

func (c *Client) fetchCLIFileInfo(ctx context.Context, rawPath string) (cliFileInfo, error) {
	output, err := c.runCommandContext(ctx, "eos", "-j", "-b", "fileinfo", rawPath)
	if err != nil {
		return cliFileInfo{}, fmt.Errorf("eos fileinfo: %w", err)
	}

	var info cliFileInfo
	if err := unmarshalEOSJSON(output, &info); err != nil {
		return cliFileInfo{}, fmt.Errorf("parse fileinfo: %w (output: %.200s)", err, output)
	}

	return info, nil
}

func entryFromCLI(info cliFileInfo) Entry {
	fullPath := cleanPath(strings.TrimSpace(info.Path))
	name := strings.TrimSpace(info.Name)
	if name == "" && fullPath == "/" {
		name = "/"
	}

	kind := EntryKindFile
	if info.Mode&040000 != 0 {
		kind = EntryKindContainer
	}

	entry := Entry{
		Kind:       kind,
		Name:       name,
		Path:       fullPath,
		ID:         info.ID,
		ParentID:   info.PID,
		Inode:      info.Inode,
		UID:        info.UID,
		GID:        info.GID,
		Size:       info.Size,
		TreeSize:   info.TreeSize,
		Files:      info.NFiles,
		Containers: info.NNDirectories,
		Flags:      info.Flags,
		Mode:       info.Mode,
		Locations:  len(info.Locations),
		LinkName:   strings.TrimSpace(info.LinkTarget),
		ETag:       strings.TrimSpace(info.ETag),
		ModifiedAt: time.Unix(info.MTime, info.MTimeNS).UTC(),
		ChangedAt:  time.Unix(info.CTime, info.CTimeNS).UTC(),
	}

	if entry.Kind == EntryKindContainer {
		entry.Files = info.TreeFiles
		entry.Containers = info.TreeContainers
	}

	return entry
}

func parseNamespaceAttrs(output []byte) []NamespaceAttr {
	lines := strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n")
	attrs := make([]NamespaceAttr, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "*") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, "\"")
		if key == "" {
			continue
		}

		attrs = append(attrs, NamespaceAttr{
			Key:   key,
			Value: value,
		})
	}

	sort.Slice(attrs, func(i, j int) bool {
		return strings.ToLower(attrs[i].Key) < strings.ToLower(attrs[j].Key)
	})

	return attrs
}
