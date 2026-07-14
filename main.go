package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lobis/eos-tui/eos"
	"github.com/lobis/eos-tui/ui"
)

// version is overridden at build time via -ldflags="-X main.version=<value>".
var version = "dev"

func main() {
	// Support both `eos-tui version` subcommand and `eos-tui --version` flag.
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version)
		return
	}

	var (
		versionFlag       = flag.Bool("version", false, "print version and exit")
		sshTarget         = flag.String("ssh", envOrDefaultCompat([]string{"EOS_TUI_SSH", "EOS_TUI_SSH_TARGET"}, ""), "SSH target for running EOS CLI remotely")
		timeout           = flag.Duration("timeout", envDurationOrDefault("EOS_TUI_TIMEOUT", 15*time.Second), "per-request timeout")
		idleTimeout       = flag.Duration("idle-timeout", envDurationOrDefault("EOS_TUI_IDLE_TIMEOUT", time.Hour), "quit after this duration without keyboard input (0 disables)")
		refreshInterval   = flag.Duration("refresh", envDurationOrDefault("EOS_TUI_REFRESH_INTERVAL", 5*time.Second), "automatic refresh interval (0 disables)")
		noAltScreen       = flag.Bool("no-alt-screen", envBoolOrDefault("EOS_TUI_NO_ALT_SCREEN", false), "disable alternate screen mode")
		acceptNewHostKeys = flag.Bool("ssh-accept-new-host-keys", envBoolOrDefault("EOS_TUI_SSH_ACCEPT_NEW_HOST_KEYS", false), "auto-accept first-seen SSH host keys using StrictHostKeyChecking=accept-new")
	)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "Options:")
		fmt.Fprintln(flag.CommandLine.Output(), "  --version")
		fmt.Fprintln(flag.CommandLine.Output(), "        print version and exit")
		fmt.Fprintln(flag.CommandLine.Output(), "  --ssh string")
		fmt.Fprintln(flag.CommandLine.Output(), "        SSH target for running EOS CLI remotely")
		fmt.Fprintln(flag.CommandLine.Output(), "  --timeout duration")
		fmt.Fprintln(flag.CommandLine.Output(), "        per-request timeout")
		fmt.Fprintln(flag.CommandLine.Output(), "  --idle-timeout duration")
		fmt.Fprintln(flag.CommandLine.Output(), "        quit after this duration without keyboard input (0 disables)")
		fmt.Fprintln(flag.CommandLine.Output(), "  --refresh duration")
		fmt.Fprintln(flag.CommandLine.Output(), "        automatic refresh interval (0 disables)")
		fmt.Fprintln(flag.CommandLine.Output(), "  --no-alt-screen")
		fmt.Fprintln(flag.CommandLine.Output(), "        disable alternate screen mode")
		fmt.Fprintln(flag.CommandLine.Output(), "  --ssh-accept-new-host-keys")
		fmt.Fprintln(flag.CommandLine.Output(), "        auto-accept first-seen SSH host keys using StrictHostKeyChecking=accept-new")
	}
	flag.Parse()

	if *versionFlag {
		fmt.Println(version)
		return
	}
	if *timeout <= 0 {
		fmt.Fprintln(os.Stderr, "--timeout must be greater than zero")
		os.Exit(2)
	}
	if *idleTimeout < 0 {
		fmt.Fprintln(os.Stderr, "--idle-timeout cannot be negative")
		os.Exit(2)
	}
	if *refreshInterval < 0 {
		fmt.Fprintln(os.Stderr, "--refresh cannot be negative")
		os.Exit(2)
	}

	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer stop()

	client, err := eos.New(runCtx, eos.Config{
		SSHTarget:         *sshTarget,
		Timeout:           *timeout,
		AcceptNewHostKeys: *acceptNewHostKeys,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "create EOS client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	displayTarget := "local eos cli"
	if *sshTarget != "" {
		displayTarget = fmt.Sprintf("ssh %s", *sshTarget)

		// Discover the MGM master so subsequent EOS commands go directly to
		// the command-serving leader rather than through the gateway.
		discoverCtx, discoverCancel := context.WithTimeout(runCtx, *timeout)
		defer discoverCancel()
		if resolved, err := client.DiscoverMGMMaster(discoverCtx); err == nil && resolved != "" {
			displayTarget = fmt.Sprintf("ssh %s  →  %s", *sshTarget, resolved)
		}
	}

	useAltScreen := !*noAltScreen && terminalSupportsAltScreen()

	options := []tea.ProgramOption{}
	if useAltScreen {
		options = append(options, tea.WithAltScreen())
	}
	options = append(options, tea.WithContext(runCtx))

	program := tea.NewProgram(
		ui.NewModelWithOptions(client, displayTarget, "", ui.ModelOptions{
			IdleTimeout:        *idleTimeout,
			RefreshInterval:    *refreshInterval,
			DisableAutoRefresh: *refreshInterval == 0,
		}),
		options...,
	)

	if _, err := program.Run(); err != nil {
		if errors.Is(err, tea.ErrProgramKilled) || errors.Is(err, context.Canceled) {
			return
		}
		fmt.Fprintf(os.Stderr, "run TUI: %v\n", err)
		os.Exit(1)
	}
}

func envOrDefaultCompat(keys []string, fallback string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}

	return fallback
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func envBoolOrDefault(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func terminalSupportsAltScreen() bool {
	term := strings.TrimSpace(strings.ToLower(os.Getenv("TERM")))
	return term != "" && term != "dumb"
}
