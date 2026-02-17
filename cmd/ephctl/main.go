// Ephctl is the command-line client for monitoring and controlling a running
// ephemerisd instance. It connects over HTTP and WebSocket to query status
// and stream live events from the daemon.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"

	"github.com/large-farva/ephemeris-engine/internal/ctl"
)

func main() {
	var (
		host    = pflag.StringP("host", "H", "http://127.0.0.1:8080", "Ephemeris daemon URL (e.g. http://192.168.8.1:8080)")
		jsonOut = pflag.Bool("json", false, "Output raw JSON instead of formatted text")
		filter  = pflag.StringSlice("filter", nil, "Event types to show in watch (e.g. --filter state,log)")
	)

	// Stop parsing global flags at the first non-flag argument (the command
	// name), so subcommand-specific flags like --duration are not rejected.
	pflag.CommandLine.SetInterspersed(false)
	pflag.Parse()

	if pflag.NArg() < 1 {
		usage()
		os.Exit(2)
	}

	cmd := pflag.Arg(0)
	subArgs := pflag.Args()[1:]

	var err error
	switch cmd {
	// ── Query commands ────────────────────────────────────────────
	case "status":
		err = ctl.Status(*host, *jsonOut)

	case "health":
		err = ctl.Health(*host, *jsonOut)

	case "version":
		err = ctl.VersionInfo(*host, *jsonOut)

	case "satellites":
		err = ctl.Satellites(*host, *jsonOut)

	case "config":
		err = ctl.Config(*host, *jsonOut)

	case "config-list":
		err = ctl.ConfigList(*host, *jsonOut)

	case "passes":
		opts := ctl.PassesOptions{JSON: *jsonOut}
		passFlags := pflag.NewFlagSet("passes", pflag.ContinueOnError)
		passFlags.IntVar(&opts.Count, "count", 0, "Limit number of passes shown")
		passFlags.StringVar(&opts.Satellite, "satellite", "", "Filter by satellite name")
		_ = passFlags.Parse(subArgs)
		err = ctl.Passes(*host, opts)

	case "next-pass":
		opts := ctl.NextPassOptions{JSON: *jsonOut}
		npFlags := pflag.NewFlagSet("next-pass", pflag.ContinueOnError)
		npFlags.StringVar(&opts.Satellite, "satellite", "", "Filter by satellite name")
		_ = npFlags.Parse(subArgs)
		err = ctl.NextPass(*host, opts)

	case "captures":
		opts := ctl.CapturesOptions{JSON: *jsonOut}
		capFlags := pflag.NewFlagSet("captures", pflag.ContinueOnError)
		capFlags.StringVar(&opts.Delete, "delete", "", "Delete a capture file by name")
		_ = capFlags.Parse(subArgs)
		err = ctl.Captures(*host, opts)

	case "tle-info":
		err = ctl.TLEInfo(*host, *jsonOut)

	case "stats":
		err = ctl.Stats(*host, *jsonOut)

	case "logs":
		opts := ctl.LogsOptions{JSON: *jsonOut}
		logFlags := pflag.NewFlagSet("logs", pflag.ContinueOnError)
		logFlags.StringVar(&opts.Level, "level", "", "Filter by log level (info, error, warn)")
		logFlags.IntVar(&opts.Limit, "limit", 0, "Limit number of log entries shown")
		logFlags.BoolVar(&opts.Tail, "tail", false, "Stream live log events (like watch --filter log)")
		_ = logFlags.Parse(subArgs)
		err = ctl.Logs(*host, opts)

	case "system-info":
		err = ctl.SystemInfo(*host, *jsonOut)

	// ── Control commands ──────────────────────────────────────────
	case "trigger":
		opts := ctl.TriggerOptions{JSON: *jsonOut}
		triggerFlags := pflag.NewFlagSet("trigger", pflag.ContinueOnError)
		triggerFlags.IntVar(&opts.NoradID, "norad-id", 0, "NORAD catalog ID")
		triggerFlags.IntVar(&opts.DurationSeconds, "duration", 600, "Capture duration in seconds")
		_ = triggerFlags.Parse(subArgs)
		if triggerFlags.NArg() > 0 {
			opts.Satellite = triggerFlags.Arg(0)
		}
		err = ctl.Trigger(*host, opts)

	case "tle-refresh":
		err = ctl.TLERefresh(*host, *jsonOut)

	case "pause":
		err = ctl.Pause(*host, *jsonOut)

	case "resume":
		err = ctl.Resume(*host, *jsonOut)

	case "skip":
		err = ctl.Skip(*host, *jsonOut)

	case "cancel":
		err = ctl.Cancel(*host, *jsonOut)

	case "reload":
		opts := ctl.ReloadOptions{JSON: *jsonOut}
		reloadFlags := pflag.NewFlagSet("reload", pflag.ContinueOnError)
		reloadFlags.StringVar(&opts.Profile, "profile", "", "Switch to a named config profile")
		_ = reloadFlags.Parse(subArgs)
		err = ctl.Reload(*host, opts)

	// ── Live streaming ────────────────────────────────────────────
	case "watch":
		err = ctl.Watch(*host, ctl.WatchOptions{
			Filter: *filter,
			JSON:   *jsonOut,
		})

	default:
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`
  ephctl — Ephemeris Engine control CLI

  USAGE
    ephctl [flags] <command> [command-flags]

  COMMANDS (query)
    status          Show daemon state, uptime, and current activity
    health          Check daemon and component health
    version         Show CLI and daemon version information
    satellites      List the satellite catalog
    config          Show the daemon's running configuration
    config-list     List available config profiles
    passes          List upcoming satellite passes
    next-pass       Show the next upcoming pass
    captures        List recorded capture files
    tle-info        Show TLE cache status and freshness
    stats           Show aggregate capture statistics
    logs            Show recent daemon log messages
    system-info     Show runtime and hardware information

  COMMANDS (control)
    trigger         Force an immediate satellite capture
    tle-refresh     Force a TLE data update from the network
    pause           Pause automatic pass scheduling
    resume          Resume pass scheduling
    skip            Skip the current/next scheduled pass
    cancel          Abort an in-progress capture
    reload          Reload configuration from disk

  COMMANDS (live)
    watch           Stream live events from the daemon (Ctrl-C to stop)

  GLOBAL FLAGS
    -H, --host URL      Daemon base URL (default: http://127.0.0.1:8080)
        --json          Output raw JSON instead of formatted text
        --filter TYPE   Event types to show in watch (comma-separated)

  COMMAND FLAGS
    passes:
        --count N           Limit number of passes shown
        --satellite NAME    Filter by satellite name

    next-pass:
        --satellite NAME    Filter by satellite name

    captures:
        --delete NAME       Delete a capture file by name

    trigger:
        --norad-id ID       NORAD catalog ID (alternative to satellite name)
        --duration SECS     Capture duration in seconds (default: 600)

    logs:
        --level LEVEL       Filter by log level (info, error, warn)
        --limit N           Limit number of log entries shown
        --tail              Stream live log events

    reload:
        --profile NAME      Switch to a named config profile

  EXAMPLES
    ephctl status
    ephctl --json status
    ephctl --host http://192.168.8.1:8080 watch
    ephctl passes --satellite NOAA-19 --count 5
    ephctl next-pass
    ephctl captures
    ephctl trigger NOAA-19 --duration 600
    ephctl tle-refresh
    ephctl tle-info
    ephctl logs --level error --limit 20
    ephctl logs --tail
    ephctl pause
    ephctl resume
    ephctl skip
    ephctl cancel
    ephctl config-list
    ephctl system-info
    ephctl stats
    ephctl reload
    ephctl reload --profile example
    ephctl watch --filter state,log,pass_scheduled

`)
}
