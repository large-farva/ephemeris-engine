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
	case "status":
		err = ctl.Status(*host, *jsonOut)

	case "watch":
		err = ctl.Watch(*host, ctl.WatchOptions{
			Filter: *filter,
			JSON:   *jsonOut,
		})

	case "health":
		err = ctl.Health(*host, *jsonOut)

	case "version":
		err = ctl.VersionInfo(*host, *jsonOut)

	case "satellites":
		err = ctl.Satellites(*host, *jsonOut)

	case "config":
		err = ctl.Config(*host, *jsonOut)

	case "passes":
		opts := ctl.PassesOptions{JSON: *jsonOut}
		passFlags := pflag.NewFlagSet("passes", pflag.ContinueOnError)
		passFlags.IntVar(&opts.Count, "count", 0, "Limit number of passes shown")
		passFlags.StringVar(&opts.Satellite, "satellite", "", "Filter by satellite name")
		_ = passFlags.Parse(subArgs)
		err = ctl.Passes(*host, opts)

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

  COMMANDS
    status          Show daemon state, uptime, and configuration
    watch           Stream live events from the daemon (Ctrl-C to stop)
    health          Check if the daemon is reachable
    version         Show CLI and daemon version information
    satellites      List the satellite catalog
    config          Show the daemon's running configuration
    passes          List upcoming satellite passes
    trigger         Force an immediate satellite capture
    tle-refresh     Force a TLE data update from the network

  GLOBAL FLAGS
    -H, --host URL      Daemon base URL (default: http://127.0.0.1:8080)
        --json          Output raw JSON instead of formatted text
        --filter TYPE   Event types to show in watch (comma-separated)

  COMMAND FLAGS
    passes:
        --count N           Limit number of passes shown
        --satellite NAME    Filter by satellite name

    trigger:
        --norad-id ID       NORAD catalog ID (alternative to satellite name)
        --duration SECS     Capture duration in seconds (default: 600)

  EXAMPLES
    ephctl status
    ephctl --host http://192.168.8.1:8080 watch
    ephctl --json passes --count 5
    ephctl passes --satellite NOAA-19
    ephctl trigger NOAA-19 --duration 600
    ephctl tle-refresh
    ephctl watch --filter state,log,pass_scheduled

`)
}
