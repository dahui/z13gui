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

	// Configure slog level. gotk4's glib init() installs a GLib log writer that
	// calls slog.Default() on every GLib/GTK message, mapping GTK-WARNING →
	// slog.LevelWarn. Setting LevelError here suppresses all GTK-WARNING noise
	// without any additional plumbing.
	level := slog.LevelError
	if verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})))

	app := gtk.NewApplication("com.github.dahui.z13gui", 0)
	app.ConnectActivate(func() {
		gui.New(app)
	})
	os.Exit(app.Run(gtkArgs))
}
