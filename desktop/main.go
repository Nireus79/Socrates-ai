// Command socrates-desktop is the desktop window of Socrates: it starts the engine, shows the screens of ../app in a window, and sends the
// page's /api requests to the engine with the token added. It has no logic of its own beyond that (see the shell package).
package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"socrates-desktop/shell"
)

//go:embed all:frontend/dist
var assets embed.FS

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
	err = wails.Run(&options.App{
		Title:       "Socrates",
		Width:       980,
		Height:      760,
		MinWidth:    420,
		MinHeight:   520,
		AssetServer: &assetserver.Options{Assets: assets, Handler: eng.Proxy(nil)},
	})
	if err != nil {
		log.Fatal(err)
	}
}
