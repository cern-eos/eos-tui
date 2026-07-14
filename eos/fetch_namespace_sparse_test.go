package eos

import (
	"strings"
	"testing"
)

func TestParseNamespaceStatsMergesSparseRows(t *testing.T) {
	input := []byte(`* info: namespace booted
{
  "result": [
    {"ns":{"total":{"files":267}}},
    {"ns":{"total":{"directories":39}}},
    {"ns":{"current":{"fid":9571}}},
    {"ns":{"current":{"cid":79}}},
    {"ns":{"generated":{"fid":2}}},
    {"ns":{"generated":{"cid":7}}},
    {"ns":{"contention":{"read":5.26316}}},
    {"ns":{"contention":{"write":0}}},
    {"ns":{"cache":{"files":{"maxsize":40000000}}}},
    {"ns":{"cache":{"files":{"occupancy":8}}}},
    {"ns":{"cache":{"files":{"requests":331454}}}},
    {"ns":{"cache":{"files":{"hits":331442}}}},
    {"ns":{"cache":{"containers":{"maxsize":5000000}}}},
    {"ns":{"cache":{"containers":{"occupancy":24}}}},
    {"ns":{"cache":{"containers":{"requests":898349}}}},
    {"ns":{"cache":{"containers":{"hits":898297}}}},
    {"master_id":"lobisapa-dev.cern.ch:1094"},
    {"ns":{"generated":{"cid":0}}},
    {"ns":{"total":{"files":0}}},
    {"ns":{"total":{"files":{"changelog":{"size":0}}}}}
  ]
}`)

	got, err := parseNamespaceStats(input)
	if err != nil {
		t.Fatalf("parseNamespaceStats() error = %v", err)
	}

	want := NamespaceStats{
		MasterHost:              "lobisapa-dev.cern.ch:1094",
		TotalFiles:              0,
		TotalDirectories:        39,
		CurrentFID:              9571,
		CurrentCID:              79,
		GeneratedFID:            2,
		GeneratedCID:            0,
		ContentionRead:          5.26316,
		ContentionWrite:         0,
		CacheFilesMax:           40000000,
		CacheFilesOccup:         8,
		CacheFilesRequests:      331454,
		CacheFilesHits:          331442,
		CacheContainersMax:      5000000,
		CacheContainersOccup:    24,
		CacheContainersRequests: 898349,
		CacheContainersHits:     898297,
	}

	if got != want {
		t.Fatalf("parseNamespaceStats() = %+v, want %+v", got, want)
	}
}

func TestParseNamespaceStatsRejectsEOSRetcEnvelope(t *testing.T) {
	_, err := parseNamespaceStats([]byte(`{"retc":"5","errormsg":"namespace unavailable"}`))
	if err == nil || !strings.Contains(err.Error(), "namespace unavailable") {
		t.Fatalf("parseNamespaceStats() error = %v, want EOS retc error", err)
	}
}
