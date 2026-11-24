package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func dataHome() (string, error) {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" && filepath.IsAbs(d) {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", ErrNoShortcut
	}
	return filepath.Join(home, ".local", "share"), nil
}

func menuFile() (string, error) {
	d, err := dataHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "applications", "socrates.desktop"), nil
}

// desktopDir is the person's desktop folder, or "" when there is none.
func desktopDir() string {
	if out, err := exec.Command("xdg-user-dir", "DESKTOP").Output(); err == nil {
		if d := strings.TrimSpace(string(out)); d != "" && d != os.Getenv("HOME") {
			if st, err := os.Stat(d); err == nil && st.IsDir() {
				return d
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		d := filepath.Join(home, "Desktop")
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
	}
	return ""
}

// ShortcutNeeded says whether to offer the entry: not when it is there already and points at this program, and not when the program is
// installed system-wide (a package puts its own entry).
func ShortcutNeeded() bool {
	exe, err := realExe()
	if err != nil || strings.HasPrefix(exe, "/usr/") || strings.HasPrefix(exe, "/opt/") {
		return false
	}
	f, err := menuFile()
	if err != nil {
		return false
	}
	b, err := os.ReadFile(f)
	return err != nil || !strings.Contains(string(b), "Exec="+execQuote(exe)+"\n")
}

// InstallShortcut makes the menu entry (and, when onDesktop, an icon on the desktop). icon is the PNG of the program's icon.
func InstallShortcut(icon []byte, onDesktop bool) error {
	exe, err := realExe()
	if err != nil {
		return ErrNoShortcut
	}
	d, err := dataHome()
	if err != nil {
		return err
	}
	iconPath := filepath.Join(d, "icons", "hicolor", "512x512", "apps", "socrates.png")
	if err := os.MkdirAll(filepath.Dir(iconPath), 0o755); err != nil {
		return ErrNoShortcut
	}
	if err := os.WriteFile(iconPath, icon, 0o644); err != nil {
		return ErrNoShortcut
	}
	entry := desktopEntry(exe, iconPath)
	menu, _ := menuFile()
	if err := os.MkdirAll(filepath.Dir(menu), 0o755); err != nil {
		return ErrNoShortcut
	}
	if err := os.WriteFile(menu, []byte(entry), 0o755); err != nil {
		return ErrNoShortcut
	}
	_ = exec.Command("update-desktop-database", filepath.Dir(menu)).Run() // helps some menus notice it; not needed
	if onDesktop {
		if dir := desktopDir(); dir != "" {
			f := filepath.Join(dir, "Socrates.desktop")
			if err := os.WriteFile(f, []byte(entry), 0o755); err != nil {
				return ErrNoShortcut
			}
			_ = exec.Command("gio", "set", f, "metadata::trusted", "true").Run() // GNOME asks before it starts an unknown launcher; this is its "allow launching"
		}
	}
	return nil
}
