package shell

import (
	"io"
	"os"
	"path/filepath"
	"sync"
)

// The engine's log lines (it keeps people's words out of them) are also kept in a file, so that a person who has a problem can find
// and send it: the app is a window without a terminal. The file is cut to its last part when it grows too big, at the start of each run.
const (
	logMax  = 512 * 1024
	logKeep = 128 * 1024
)

var (
	logMu  sync.Mutex
	logOut io.Writer = os.Stderr
)

// UseLogFile makes every engine started later write its log to the file at path as well as to standard error. A file that cannot be
// opened leaves the log on standard error alone.
func UseLogFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	trimLog(path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	logMu.Lock()
	logOut = io.MultiWriter(os.Stderr, f)
	logMu.Unlock()
	return nil
}

func engineLog() io.Writer {
	logMu.Lock()
	defer logMu.Unlock()
	return logOut
}

// trimLog keeps the last logKeep bytes of a file that has grown past logMax.
func trimLog(path string) {
	fi, err := os.Stat(path)
	if err != nil || fi.Size() <= logMax {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	tail := b[len(b)-logKeep:]
	for i, c := range tail { // start at a whole line
		if c == '\n' {
			tail = tail[i+1:]
			break
		}
	}
	_ = os.WriteFile(path, tail, 0o600)
}
