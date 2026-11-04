package shell

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// The window's web view does not always follow the desktop's light or dark setting (WebKitGTK on Linux often does not), so the shell asks the
// system itself and tells the page. The answer is "dark", "light", or "" when it cannot be found, in which case the page leaves the choice
// to the web view.

// Runner runs a command and returns what it printed; it is a parameter so that the reading can be tested without a desktop.
type Runner func(ctx context.Context, name string, args ...string) (string, error)

func execRunner(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = withoutSnapSchemas(os.Environ())
	out, err := cmd.Output()
	return string(out), err
}

// withoutSnapSchemas drops the variables that a snap-packaged terminal (the VS Code snap) leaks into its children. They point at that
// snap's own settings schemas and libraries, which lack the desktop's colour-scheme key, so the system's real setting could not be read.
func withoutSnapSchemas(env []string) []string {
	leaked := []string{"GSETTINGS_SCHEMA_DIR=", "GIO_MODULE_DIR=", "XDG_DATA_DIRS=", "GTK_PATH=", "LOCPATH="}
	out := make([]string, 0, len(env))
next:
	for _, kv := range env {
		if strings.Contains(kv, "/snap/") {
			for _, name := range leaked {
				if strings.HasPrefix(kv, name) {
					continue next
				}
			}
		}
		out = append(out, kv)
	}
	return out
}

// SystemTheme reads the desktop's setting for this operating system.
func SystemTheme() string { return systemTheme(runtime.GOOS, execRunner) }

func systemTheme(goos string, run Runner) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	switch goos {
	case "linux":
		if out, err := run(ctx, "gsettings", "get", "org.gnome.desktop.interface", "color-scheme"); err == nil {
			if v := ParseColorScheme(out); v != "" {
				return v
			}
		}
		// older desktops have only the name of the theme. A name without "dark" proves nothing (the default theme is dark or light by the
		// colour-scheme setting, which may be unreadable from here), so only "dark" is an answer.
		if out, err := run(ctx, "gsettings", "get", "org.gnome.desktop.interface", "gtk-theme"); err == nil {
			if strings.Contains(strings.ToLower(out), "dark") {
				return "dark"
			}
		}
	case "darwin":
		// "Dark" when the Mac is in dark mode; in light mode the key does not exist and the command fails, which proves nothing, so the page's own
		// prefers-color-scheme (which the web view takes from the system) decides.
		if out, err := run(ctx, "defaults", "read", "-g", "AppleInterfaceStyle"); err == nil && strings.Contains(strings.ToLower(out), "dark") {
			return "dark"
		}
	case "windows":
		if out, err := run(ctx, "reg", "query", `HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "/v", "AppsUseLightTheme"); err == nil {
			return ParseWindowsLightTheme(out)
		}
	}
	return ""
}

// ParseColorScheme reads the output of "gsettings get org.gnome.desktop.interface color-scheme" ('prefer-dark', 'prefer-light', 'default').
func ParseColorScheme(out string) string {
	s := strings.Trim(strings.TrimSpace(out), "'\"")
	switch s {
	case "prefer-dark":
		return "dark"
	case "prefer-light":
		return "light"
	}
	return "" // 'default' says nothing; the theme name decides
}

var lightValue = regexp.MustCompile(`AppsUseLightTheme\s+REG_DWORD\s+0x([0-9a-fA-F]+)`)

// ParseWindowsLightTheme reads the output of "reg query ... /v AppsUseLightTheme" (0x0 is dark, 0x1 is light).
func ParseWindowsLightTheme(out string) string {
	m := lightValue.FindStringSubmatch(out)
	if m == nil {
		return ""
	}
	if strings.TrimLeft(m[1], "0") == "" {
		return "dark"
	}
	return "light"
}
