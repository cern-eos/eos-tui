package eos

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestParseInspectorStats(t *testing.T) {
	input := []byte(`
key=last tag=summary::avg_filesize value=4096
key=last tag=links::hardlink_count value=3817
key=last tag=links::hardlink_volume value=1346800
key=last tag=links::symlink_count value=7900
key=last layout=00100012 type=replica locations=477406 physicalsize=53308906460832 volume=53308906460832
key=last layout=20140b42 type=raid6 locations=3315636 physicalsize=551296974750284 volume=459414145717156
key=last tag=user::cost::disk username=eos uid=74693 cost=44647.462383 price=20.000000 tbyears=2232.373119
key=last tag=user::cost::disk username=cmst0 uid=103031 cost=5058.986655 price=20.000000 tbyears=252.949333
key=last tag=group::cost::disk groupname=c3 gid=1028 cost=47343.679351 price=20.000000 tbyears=2367.183968
key=last tag=group::cost::disk groupname=zh gid=1399 cost=5447.730977 price=20.000000 tbyears=272.386549
`)

	got := parseInspectorStats(input)
	if got.AvgFileSize != 4096 {
		t.Fatalf("AvgFileSize = %d, want 4096", got.AvgFileSize)
	}
	if got.HardlinkCount != 3817 {
		t.Fatalf("HardlinkCount = %d, want 3817", got.HardlinkCount)
	}
	if got.HardlinkVolume != 1346800 {
		t.Fatalf("HardlinkVolume = %d, want 1346800", got.HardlinkVolume)
	}
	if got.SymlinkCount != 7900 {
		t.Fatalf("SymlinkCount = %d, want 7900", got.SymlinkCount)
	}
	if got.LayoutCount != 2 {
		t.Fatalf("LayoutCount = %d, want 2", got.LayoutCount)
	}
	if got.TopLayout.Layout != "20140b42" || got.TopLayout.Type != "raid6" || got.TopLayout.VolumeBytes != 459414145717156 {
		t.Fatalf("TopLayout = %+v, want raid6 layout 20140b42", got.TopLayout)
	}
	if len(got.Layouts) != 2 || got.Layouts[0].Layout != "20140b42" {
		t.Fatalf("Layouts = %+v, want sorted layouts with 20140b42 first", got.Layouts)
	}
	if got.TopUserCost.Name != "eos" || got.TopUserCost.ID != 74693 {
		t.Fatalf("TopUserCost = %+v, want user eos/74693", got.TopUserCost)
	}
	if len(got.UserCosts) != 2 || got.UserCosts[0].Name != "eos" {
		t.Fatalf("UserCosts = %+v, want sorted users with eos first", got.UserCosts)
	}
	if got.TopGroupCost.Name != "c3" || got.TopGroupCost.ID != 1028 {
		t.Fatalf("TopGroupCost = %+v, want group c3/1028", got.TopGroupCost)
	}
	if len(got.GroupCosts) != 2 || got.GroupCosts[0].Name != "c3" {
		t.Fatalf("GroupCosts = %+v, want sorted groups with c3 first", got.GroupCosts)
	}
}

func TestParseInspectorStatsSkipsPreambleAndFallsBackToNumericIDs(t *testing.T) {
	input := []byte(`
* info
key=last tag=user::cost::disk uid=42 cost=7.5 tbyears=1.5
key=last tag=group::cost::disk gid=84 cost=9.5 tbyears=2.5
key=last tag=accesstime::files bin=86400 value=11
key=last tag=accesstime::files bin=0 value=7
key=last tag=birthtime::volume bin=604800 value=33
`)

	got := parseInspectorStats(input)
	if got.TopUserCost.Name != "42" || got.TopUserCost.ID != 42 {
		t.Fatalf("TopUserCost = %+v, want numeric fallback 42", got.TopUserCost)
	}
	if got.TopGroupCost.Name != "84" || got.TopGroupCost.ID != 84 {
		t.Fatalf("TopGroupCost = %+v, want numeric fallback 84", got.TopGroupCost)
	}
	if len(got.AccessFiles) != 2 || got.AccessFiles[0].BinSeconds != 0 || got.AccessFiles[1].BinSeconds != 86400 {
		t.Fatalf("AccessFiles = %+v, want bins sorted by seconds", got.AccessFiles)
	}
	if len(got.BirthVolume) != 1 || got.BirthVolume[0].Value != 33 {
		t.Fatalf("BirthVolume = %+v, want one volume bin", got.BirthVolume)
	}
}

func TestInspectorTreatsErrorRowAsFailureDespiteZeroExitStatus(t *testing.T) {
	runner := &recordingRunner{out: []byte(`key=error space=default msg="inspector disabled"`)}
	client := &Client{timeout: time.Second, runner: runner}

	_, err := client.Inspector(context.Background())
	if err == nil || !strings.Contains(err.Error(), "inspector disabled") {
		t.Fatalf("Inspector() error = %v, want disabled error", err)
	}
}

func TestInspectorTreatsGenericErrorRowAsFailure(t *testing.T) {
	runner := &recordingRunner{out: []byte(`key=error msg="backend unavailable"`)}
	client := &Client{timeout: time.Second, runner: runner}

	_, err := client.Inspector(context.Background())
	if err == nil || !strings.Contains(err.Error(), "backend unavailable") {
		t.Fatalf("Inspector() error = %v, want backend error", err)
	}
}
