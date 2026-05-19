package main

import (
	"fmt"
	"os"

	"github.com/madLinux7/khi-explorer/config"
	"github.com/madLinux7/khi-explorer/tui"
)

func main() {
	if len(os.Args) > 1 {
		arg := os.Args[1]
		if arg == "-h" || arg == "--help" || arg == "help" {
			printHelp()
			os.Exit(0)
		}
		if arg == "-v" || arg == "--version" || arg == "version" {
			fmt.Printf("khi-explorer version %s\n", Version)
			os.Exit(0)
		}
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Save default config if it doesn't exist
	if _, err := os.Stat(config.GetConfigPath()); os.IsNotExist(err) {
		cfg.Save()
	}

	if err := tui.Start(cfg); err != nil {
		fmt.Printf("Alas, there's been an error: %v\n", err)
		os.Exit(1)
	}
}

var Version = "dev"

func printHelp() {
	helpText := fmt.Sprintf(`khi-explorer (v%s) - A fast, interactive TUI browser and downloader for KHInsider.

Usage:
  khi-explorer [flags]

Flags:
  -h, --help      Show this help overview
  -v, --version   Show application version

TUI Navigation Controls:
  ↑/↓ or k/j      Move selection cursor up/down
  Left/Right      Previous/Next page (paging results & song lists)
  Enter           Select album, search, or play song
  Esc/Backspace   Go back to the previous view
  /               Filter lists
  ctrl+c, q       Quit the application

TUI Playback Controls (while playing a song):
  P               Pause or Resume playback
  S               Stop playback completely

TUI Download Controls:
  D               Queue highlighted song for download (in Album view)
  D               Queue entire album for download (in Search results view)

Configuration:
  Settings path:  ~/.khi_explorer.yaml
  Music folder:   ~/Music/khi_explorer/
`, Version)
	fmt.Print(helpText)
}
