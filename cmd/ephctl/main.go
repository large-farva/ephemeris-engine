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
		host = pflag.StringP("host", "H", "http://127.0.0.1:8080", "Ephemeris daemon URL (e.g. http://192.168.8.1:8080)")
	)
	pflag.Parse()

	if pflag.NArg() < 1 {
		usage()
		os.Exit(2)
	}

	cmd := pflag.Arg(0)

	switch cmd {
	case "status":
		if err := ctl.Status(*host); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "watch":
		if err := ctl.Watch(*host); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`
  ephctl — Ephemeris Engine control CLI

  USAGE
    ephctl [flags] <command>

  COMMANDS
    status    Show daemon state, uptime, and configuration
    watch     Stream live events from the daemon (Ctrl-C to stop)

  FLAGS
    -H, --host URL    Daemon base URL (default: http://127.0.0.1:8080)

  EXAMPLES
    ephctl status
    ephctl --host http://192.168.8.1:8080 watch

`)
}
