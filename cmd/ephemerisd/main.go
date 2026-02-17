// Ephemerisd is the main daemon for the Ephemeris Engine satellite receiver.
//
// It loads configuration, starts the HTTP/WebSocket server, and runs either
// the real satellite scheduler or a demo loop depending on config. Shutdown
// is handled gracefully on SIGINT or SIGTERM.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/pflag"

	"github.com/large-farva/ephemeris-engine/internal/app"
	"github.com/large-farva/ephemeris-engine/internal/config"
)

func main() {
	var (
		configPath = pflag.StringP("config", "c", "", "Path to config TOML (auto-discovers if omitted)")
		bind       = pflag.String("bind", "0.0.0.0:8080", "HTTP bind address")
	)
	pflag.Parse()

	// Resolve config file: explicit flag → auto-discovery chain → defaults.
	cfgFile := *configPath
	if cfgFile == "" {
		cfgFile = config.FindConfigFile()
	}

	logger := log.New(os.Stdout, "ephemerisd ", log.LstdFlags|log.Lmicroseconds)

	var cfg config.Config
	if cfgFile == "" {
		cfg = config.Default()
		logger.Printf("no config file found, using defaults")
		logger.Printf("create %s/config.toml to customize", config.DefaultConfigDir())
	} else {
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			log.Fatalf("config load failed: %v", err)
		}
		logger.Printf("loaded config from %s", cfgFile)
	}

	if err := config.EnsureDirectories(cfg); err != nil {
		log.Fatalf("directory setup: %v", err)
	}

	a := app.New(app.Options{
		Logger:     logger,
		Cfg:        cfg,
		Bind:       *bind,
		ConfigPath: cfgFile,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := a.Run(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("ephemerisd failed: %v", err)
	}

	// Brief pause so in-flight log writes can flush before exit.
	time.Sleep(50 * time.Millisecond)
}
