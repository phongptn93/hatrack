//go:build darwin

// Command hatrack-gui is the macOS menu bar app: the hat menu plus a dashboard window.
package main

import (
	"fmt"
	"log"

	"github.com/baodq97/hatrack/internal/claude"
	"github.com/baodq97/hatrack/internal/gui"
	"github.com/baodq97/hatrack/internal/tray"
)

func main() {
	store, err := claude.Default()
	if err != nil {
		log.Fatalf("Failed to initialize claude store: %v", err)
	}

	srv, err := gui.NewServer(store, 4428)
	if err != nil {
		log.Fatalf("Failed to create GUI server: %v", err)
	}

	fmt.Printf("🎩 Hatrack Desktop GUI App running at %s\n", srv.Addr)

	// Start HTTP server in background
	go func() {
		if err := srv.Start(); err != nil {
			log.Printf("GUI server notice: %v", err)
		}
	}()

	// Open App Window in background
	go func() {
		if err := srv.OpenAppWindow(); err != nil {
			fmt.Printf("Notice: Access GUI at: %s\n", srv.Addr)
		}
	}()

	// Run macOS Menu Bar Tray icon on main thread
	tray.Start(store, func() {
		_ = srv.OpenAppWindow()
	}, func() error { return gui.OpenAddInTerminal("") })
}
