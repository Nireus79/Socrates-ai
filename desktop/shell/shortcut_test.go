//go:build linux

package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecQuoteKeepsOddPathsInOneArgument(t *testing.T) {
	cases := map[string]string{
		"/home/a/socrates-desktop":   `"/home/a/socrates-desktop"`,
		"/home/a b/socrates-desktop": `"/home/a b/socrates-desktop"`,
		`/home/a"b/x`:                `"/home/a\\"b/x"`,
		"/home/$USER/x":              `"/home/\\$USER/x"`,
		"/home/100%/x":               `"/home/100%%/x"`,
		"/home/a`b/x":                "\"/home/a\\\\`b/x\"",
		`/home/back\slash/x`:         `"/home/back\\\\slash/x"`,
	}
	for in, want := range cases {
		if got := execQuote(in); got != want {
			t.Errorf("execQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestInstallingTheShortcutMakesTheMenuEntryAndTheDesktopIconOnlyWhenAsked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config")) // xdg-user-dirs must not read the real one
	if err := os.Mkdir(filepath.Join(home, "Desktop"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !ShortcutNeeded() {
		t.Fatal("a computer with no entry must be offered one")
	}
	png := []byte("\x89PNG test icon")
	if err := InstallShortcut(png, false); err != nil {
		t.Fatal(err)
	}
	menu := filepath.Join(home, "data", "applications", "socrates.desktop")
	b, err := os.ReadFile(menu)
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := realExe()
	for _, want := range []string{"[Desktop Entry]\n", "Type=Application\n", "Name=Socrates\n", "Exec=" + execQuote(exe) + "\n", "Terminal=false\n"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the entry lacks %q:\n%s", want, b)
		}
	}
	icon := filepath.Join(home, "data", "icons", "hicolor", "512x512", "apps", "socrates.png")
	if got, err := os.ReadFile(icon); err != nil || string(got) != string(png) {
		t.Fatalf("the icon: %v", err)
	}
	if !strings.Contains(string(b), "Icon="+icon+"\n") {
		t.Errorf("the entry does not point at the icon file:\n%s", b)
	}
	if st, _ := os.Stat(menu); st.Mode()&0o111 == 0 {
		t.Error("the entry must be executable for the launchers that insist")
	}
	if _, err := os.Stat(filepath.Join(home, "Desktop", "Socrates.desktop")); err == nil {
		t.Fatal("a desktop icon was made without being asked")
	}
	if ShortcutNeeded() {
		t.Fatal("the entry is there and points here: it must not be offered again")
	}
	if err := InstallShortcut(png, true); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(home, "Desktop", "Socrates.desktop"))
	if err != nil || st.Mode()&0o100 == 0 {
		t.Fatalf("the desktop icon: %v", err)
	}
}

func TestAnEntryThatPointsElsewhereIsOfferedAgain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	menu := filepath.Join(home, "data", "applications", "socrates.desktop")
	os.MkdirAll(filepath.Dir(menu), 0o755)
	os.WriteFile(menu, []byte("[Desktop Entry]\nExec=\"/old/place/socrates-desktop\"\n"), 0o755)
	if !ShortcutNeeded() {
		t.Fatal("the program was moved: the old entry no longer starts it")
	}
}
