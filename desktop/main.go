// Command socrates-desktop is the desktop window of Socrates: it starts the engine, shows the screens of ../app in a window, and sends the
// page's /api requests to the engine with the token added. It has no logic of its own beyond that (see the shell package).
package main

import (
	"context"
	"embed"
	"errors"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"socrates-desktop/shell"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed icon.png
var icon []byte

// App is what the page may call in the window: opening the key pages of the providers in the person's browser, nothing else.
type App struct{ ctx context.Context }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// OpenLink opens an address in the system browser if the shell allows it (shell.AllowedLink).
func (a *App) OpenLink(url string) error {
	if !shell.AllowedLink(url) {
		return errors.New("that address is not one the app opens")
	}
	wruntime.BrowserOpenURL(a.ctx, url)
	return nil
}

func enginePath() string {
	if p := os.Getenv("SOCRATES_ENGINE"); p != "" {
		return p
	}
	exe, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	name := "socrates-engine"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(exe), name)
}

func main() {
	var args []string
	if d := os.Getenv("SOCRATES_DATA_DIR"); d != "" {
		args = append(args, "--data-dir", d)
	}
	eng, err := shell.Start(enginePath(), args, nil, 30*time.Second)
	if err != nil {
		log.Fatal(err)
	}
	defer eng.Stop()
	app := &App{}
	err = wails.Run(&options.App{
		Title:       "Socrates",
		Width:       980,
		Height:      760,
		MinWidth:    420,
		MinHeight:   520,
		AssetServer: &assetserver.Options{Assets: assets, Handler: eng.Proxy(nil)},
		OnStartup:   app.startup,
		Bind:        []interface{}{app},
		Linux:       &linux.Options{Icon: icon},
	})
	if err != nil {
		log.Fatal(err)
	}
}
