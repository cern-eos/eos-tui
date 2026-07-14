package eos

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLooksUnsupported(t *testing.T) {
	// Real-world output from eosaliceo2-ns-02 when running `eos io shaping ls`:
	// EOS doesn't recognise `shaping` so it dumps the help text for the
	// supported `io` subcommands (`stat`, `enable`, ...) and exits 22.
	unknownSubcmdHelp := []byte(`usage:
io stat [-l] [-a] [-m] [-n] [-t] [-d] [-x] [--ss] [--sa] [--si] : print io statistics
    -l : show summary information (this is the default if -a,-t,-d,-x is not selected)
io enable [-r] [-p] [-n] [--udp <address>] : enable collection of io statistics
io disable [-r] [-p] [-n] [--udp <address>] : disable collection of io statistics
`)

	cases := []struct {
		name   string
		err    error
		output []byte
		want   bool
	}{
		{"nil error", nil, unknownSubcmdHelp, false},
		{"exit 22 + unknown subcommand help", errors.New("exit status 22"), unknownSubcmdHelp, true},
		{"exit 22 wrapping help with shaping mention", errors.New("exit status 22"), []byte("usage: io shaping ls --json"), false},
		{"exit 1 + same help text", errors.New("exit status 1"), unknownSubcmdHelp, false},
		{"exit 22 + unrelated message", errors.New("exit status 22"), []byte("permission denied"), false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksUnsupported(tt.err, tt.output); got != tt.want {
				t.Fatalf("looksUnsupported = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIOShapingPressureReportsCLILibraryMismatch(t *testing.T) {
	runner := &recordingRunner{
		out: []byte("eos: symbol lookup error: eos: undefined symbol: _ZN3eos...\n"),
		err: errors.New("exit status 127"),
	}
	client := &Client{timeout: time.Second, runner: runner}

	_, err := client.IOShapingPressure(context.Background())
	if err == nil || !strings.Contains(err.Error(), "CLI/library mismatch") {
		t.Fatalf("IOShapingPressure() error = %v, want library mismatch", err)
	}
}
