package shell

import (
	"context"
	"errors"
	"testing"
)

func fake(answers map[string]string) Runner {
	return func(_ context.Context, name string, args ...string) (string, error) {
		key := name
		for _, a := range args {
			key += " " + a
		}
		if v, ok := answers[key]; ok {
			return v, nil
		}
		return "", errors.New("no such command")
	}
}

const scheme = "gsettings get org.gnome.desktop.interface color-scheme"
const gtkName = "gsettings get org.gnome.desktop.interface gtk-theme"

func TestTheDesktopSettingIsReadOnLinuxAndTheThemeNameIsTheFallback(t *testing.T) {
	cases := []struct {
		name    string
		answers map[string]string
		want    string
	}{
		{"dark", map[string]string{scheme: "'prefer-dark'\n"}, "dark"},
		{"light", map[string]string{scheme: "'prefer-light'\n"}, "light"},
		{"default with a dark theme name", map[string]string{scheme: "'default'\n", gtkName: "'Yaru-dark'\n"}, "dark"},
		{"default with a theme name that says nothing", map[string]string{scheme: "'default'\n", gtkName: "'Adwaita'\n"}, ""},
		{"colour scheme unreadable and a plain theme name must not turn a dark desktop light", map[string]string{gtkName: "'Adwaita'\n"}, ""},
		{"an old desktop with only a theme name", map[string]string{gtkName: "'Adwaita-dark'\n"}, "dark"},
		{"nothing to read", map[string]string{}, ""},
	}
	for _, c := range cases {
		if got := systemTheme("linux", fake(c.answers)); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestWindowsDarkAndLightAreReadFromTheRegistryAnswer(t *testing.T) {
	const key = `reg query HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize /v AppsUseLightTheme`
	dark := "\r\nHKEY_CURRENT_USER\\...\\Personalize\r\n    AppsUseLightTheme    REG_DWORD    0x0\r\n"
	light := "\r\nHKEY_CURRENT_USER\\...\\Personalize\r\n    AppsUseLightTheme    REG_DWORD    0x1\r\n"
	if systemTheme("windows", fake(map[string]string{key: dark})) != "dark" || systemTheme("windows", fake(map[string]string{key: light})) != "light" {
		t.Fatal("the registry answer was misread")
	}
	if systemTheme("windows", fake(map[string]string{key: "garbage"})) != "" {
		t.Fatal("an unreadable answer must say nothing")
	}
	if systemTheme("darwin", fake(nil)) != "" {
		t.Fatal("other systems say nothing and the web view decides")
	}
}

func TestTheSnapSchemaFolderOfAnEditorTerminalIsNotPassedOn(t *testing.T) {
	in := []string{"HOME=/h", "GSETTINGS_SCHEMA_DIR=/home/u/snap/code/274/.local/share/glib-2.0/schemas", "XDG_DATA_DIRS=/home/u/snap/code/274/.local/share:/usr/share", "PATH=/usr/bin"}
	out := withoutSnapSchemas(in)
	if len(out) != 2 || out[0] != "HOME=/h" || out[1] != "PATH=/usr/bin" {
		t.Fatalf("%v", out)
	}
	own := []string{"GSETTINGS_SCHEMA_DIR=/usr/share/glib-2.0/schemas"}
	if got := withoutSnapSchemas(own); len(got) != 1 {
		t.Fatal("a folder that is not a snap's own must be kept")
	}
}

func TestMacDarkIsReadFromTheInterfaceStyleAndLightProvesNothing(t *testing.T) {
	const style = "defaults read -g AppleInterfaceStyle"
	if got := systemTheme("darwin", fake(map[string]string{style: "Dark\n"})); got != "dark" {
		t.Fatalf("dark mode: got %q", got)
	}
	if got := systemTheme("darwin", fake(map[string]string{})); got != "" {
		t.Fatalf("a failing command (light mode, or unreadable) must not claim a theme: got %q", got)
	}
}
