// Noraneko WinUpdater - Windows updater for Noraneko Browser
// Based on LibreWolf WinUpdater by ltguillaume
// https://codeberg.org/ltguillaume/librewolf-winupdater
//
// This is a Go port adapted for Noraneko Browser

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/f3liz-dev/noraneko-winupdater/pkg/config"
	"github.com/f3liz-dev/noraneko-winupdater/pkg/updater"
)

const (
	Version    = "1.0.0"
	BrowserName = "Noraneko"
)

func main() {
	// Parse command line flags
	scheduled := flag.Bool("scheduled", false, "Run as scheduled task")
	portable := flag.Bool("portable", false, "Run in portable mode")
	createTask := flag.Bool("create-task", false, "Create scheduled task")
	removeTask := flag.Bool("remove-task", false, "Remove scheduled task")
	checkOnly := flag.Bool("check-only", false, "Only check for updates, do not install")
	version := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *version {
		fmt.Printf("%s WinUpdater v%s\n", BrowserName, Version)
		os.Exit(0)
	}

	// Get executable directory
	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting executable path: %v\n", err)
		os.Exit(1)
	}
	exeDir := filepath.Dir(exePath)

	// Load configuration
	cfg, err := config.Load(exeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// Create updater instance
	u := updater.New(cfg, updater.Options{
		Scheduled:  *scheduled,
		Portable:   *portable,
		CheckOnly:  *checkOnly,
		CreateTask: *createTask,
		RemoveTask: *removeTask,
		Version:    Version,
	})

	// Handle scheduled task operations
	if *createTask || *removeTask {
		if err := u.HandleScheduledTask(); err != nil {
			fmt.Fprintf(os.Stderr, "Error handling scheduled task: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Run the updater
	if err := u.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
