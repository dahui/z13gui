package main

// z13gui — GTK4 Wayland overlay drawer for z13ctl.
// Slides in from the right edge on Armoury Crate button press (via z13ctl daemon).

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/dahui/z13gui/internal/gui"
	"github.com/dahui/z13gui/internal/theme"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Version is set at link time via -X main.Version=<version>.
var Version = "dev"

func main() {
	// Scan args for our flags before GTK sees them. We cannot use flag.Parse()
	// because app.Run() passes remaining args to GLib's option parser, which
	// would error on any flags it doesn't recognize.
	verbose := false
	gtkArgs := []string{os.Args[0]}
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--verbose", "-v":
			verbose = true
		case "--version":
			fmt.Printf("z13gui %s\n", Version)
			os.Exit(0)
		case "--print-theme":
			fmt.Print(gui.DefaultThemeTOML())
			os.Exit(0)
		case "--list-themes":
			for _, t := range theme.Builtins {
				fmt.Printf("%-20s %s\n", t.ID, t.Name)
			}
			os.Exit(0)
		default:
			gtkArgs = append(gtkArgs, arg)
		}
	}

	// Configure slog with split-level filtering. gotk4's glib init() routes
	// all GLib/GTK messages through slog.Default(), adding a "glib_domain"
	// attribute. Our filterHandler uses that to apply separate thresholds:
	//   default: app=Info, GTK=Error (show app events, suppress GTK noise)
	//   -v:      app=Debug, GTK=Debug (show everything)
	appLevel, gtkLevel := slog.LevelInfo, slog.LevelError
	if verbose {
		appLevel, gtkLevel = slog.LevelDebug, slog.LevelDebug
	}
	text := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: appLevel})
	slog.SetDefault(slog.New(gui.NewFilterHandler(text, appLevel, gtkLevel)))

	slog.Info("starting", "version", Version)

	app := gtk.NewApplication("com.github.dahui.z13gui", 0)
	app.ConnectActivate(func() {
		gui.New(app)
	})
	os.Exit(app.Run(gtkArgs))
}
