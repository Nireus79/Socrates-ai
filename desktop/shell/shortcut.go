package shell

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// A shortcut: Socrates in the person's applications menu (and on the desktop), so that it starts like any other program. The release
// packages are an unpacked folder (a .tar.gz or a .zip) with no installer, so the window offers to make the entry itself. Nothing needs
// administrator rights and nothing is changed outside the person's own folders: on Linux a .desktop file and the icon under ~/.local/share,
// on Windows .lnk files in the person's Start menu and desktop. macOS has Socrates.app, which is already an application.

// ErrNoShortcut is returned where a shortcut makes no sense (macOS) or cannot be made.
var ErrNoShortcut = errors.New("Socrates cannot add itself to the menu on this computer.")

// execQuote writes a path as the Exec value of a .desktop file: in double quotes, with the characters the specification names escaped,
// and then every backslash doubled (the string layer of the file format); a percent sign is doubled too, since % starts a field code.
func execQuote(path string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`)
	q := strings.ReplaceAll(r.Replace(path), `\`, `\\`)
	return `"` + strings.ReplaceAll(q, "%", "%%") + `"`
}

// desktopEntry is the text of the .desktop file for the program at exe with the icon at iconPath.
func desktopEntry(exe, iconPath string) string {
	return "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=Socrates\n" +
		"Comment=Talk with artificial Greek philosophers\n" +
		"Exec=" + execQuote(exe) + "\n" +
		"Icon=" + iconPath + "\n" +
		"Terminal=false\n" +
		"Categories=Education;Utility;\n" +
		"StartupNotify=true\n"
}

// realExe is the path of the running program with links followed, so that the shortcut keeps working if it was started through one.
func realExe() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return filepath.Abs(exe)
}
