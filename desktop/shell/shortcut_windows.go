//go:build windows

package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// The shortcuts are .lnk files made by the system's own scripting object, through PowerShell, with the paths passed in the environment
// (never put into the script text). The icon is the one inside the program.
const lnkScript = `
$w = New-Object -ComObject WScript.Shell
$paths = @($env:SOCRATES_LNK1)
if ($env:SOCRATES_ONDESKTOP -eq '1') { $paths += (Join-Path ([Environment]::GetFolderPath('Desktop')) 'Socrates.lnk') }
foreach ($p in $paths) {
  $s = $w.CreateShortcut($p)
  $s.TargetPath = $env:SOCRATES_EXE
  $s.WorkingDirectory = Split-Path $env:SOCRATES_EXE
  $s.IconLocation = $env:SOCRATES_EXE + ',0'
  $s.Description = 'Talk with artificial Greek philosophers'
  $s.Save()
}
`

func startMenuLink() string {
	if a := os.Getenv("APPDATA"); a != "" {
		return filepath.Join(a, "Microsoft", "Windows", "Start Menu", "Programs", "Socrates.lnk")
	}
	return ""
}

// ShortcutNeeded: not when the Start menu entry is there, and not for a copy that Scoop manages (it makes its own shortcut).
func ShortcutNeeded() bool {
	exe, err := realExe()
	if err != nil || strings.Contains(strings.ToLower(exe), `\scoop\apps\`) {
		return false
	}
	l := startMenuLink()
	if l == "" {
		return false
	}
	_, err = os.Stat(l)
	return err != nil
}

func InstallShortcut(icon []byte, onDesktop bool) error {
	exe, err := realExe()
	if err != nil {
		return ErrNoShortcut
	}
	l1 := startMenuLink()
	if l1 == "" {
		return ErrNoShortcut
	}
	desk := "0"
	if onDesktop {
		desk = "1" // the script asks the system for the desktop folder, which may have been moved (for instance into OneDrive)
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", lnkScript)
	cmd.Env = append(os.Environ(), "SOCRATES_EXE="+exe, "SOCRATES_LNK1="+l1, "SOCRATES_ONDESKTOP="+desk)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Run(); err != nil {
		return ErrNoShortcut
	}
	return nil
}
