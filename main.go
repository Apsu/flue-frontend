package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"flue-frontend/pkg/server"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/log"
)

// CLI holds the command line flags for the application.
type CLI struct {
	Host string `default:"localhost" help:"Host to run the server on."`
	Port int `default:"8080" help:"Port to run the server on."`
	Backend string `default:"http://localhost:8000" help:"URL of the backend API to send requests to."`
}

func main() {
	// Create a cancelable context and install a signal handler.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Parse CLI flags directly into the global configuration.
	var cli CLI
	kctx := kong.Parse(&cli,
		kong.Bind(&ctx, &stop),
		kong.Name("flue-frontend"),
		kong.Description("Flue Frontend: A simple web interface for generating images using Flue."),
		kong.Vars{"version": "0.1.0"},
	)

	// Run the application.
	err := kctx.Run()
	kctx.FatalIfErrorf(err)
}

func (c *CLI) Run(ctx *context.Context, stop *context.CancelFunc) error {
	// Create a new server instance
	log.Infof("Starting Flue Frontend on %s:%d, backend: %s", c.Host, c.Port, c.Backend)
	srv := server.New(c.Host, c.Port, c.Backend)
	log.Infof("Server initialized, setting up to run...")
	err := srv.Run(*ctx, *stop)
	if err != nil {
		// Handle server run error
		// This will typically not happen unless there is a fatal error in starting the server
		log.Errorf("Failed to run server: %v", err)
		return err
	}
	log.Infof("Server Run() returned successfully")
	return nil
}
