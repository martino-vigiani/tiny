package keys

import (
	"errors"
	"io"
	"sync"

	"golang.org/x/term"
)

// errEndOfScript is what a spent Script reports; it is io.EOF so callers apply
// their ordinary end-of-input policy.
var errEndOfScript = io.EOF

// Session owns a terminal put into raw mode.
//
// Raw mode is not a cosmetic detail: it disables line buffering and echo, and
// also ISIG, which means Ctrl-C arrives as the byte 0x03 rather than a signal.
// If the terminal is not restored on *every* exit path — return, error, panic,
// or a signal from outside — the user is left with a shell that does not echo
// what they type. That is why restoration is tracked globally and is
// idempotent.
type Session struct {
	fd    int
	state *term.State
	once  sync.Once
}

var (
	activeMu sync.Mutex
	active   []*Session
)

// EnterRaw puts fd into raw mode and registers the session for restoration.
func EnterRaw(fd int) (*Session, error) {
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	s := &Session{fd: fd, state: state}
	activeMu.Lock()
	active = append(active, s)
	activeMu.Unlock()
	return s, nil
}

// Close restores the terminal. It is safe to call more than once, and safe to
// call after RestoreAll has already run.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	var err error
	s.once.Do(func() {
		err = term.Restore(s.fd, s.state)
		activeMu.Lock()
		for i, other := range active {
			if other == s {
				active = append(active[:i], active[i+1:]...)
				break
			}
		}
		activeMu.Unlock()
	})
	return err
}

// RestoreAll restores every terminal still in raw mode. It is the last line of
// defence, wired to the signal context in main: a SIGTERM arriving mid-prompt
// must not leave the shell broken.
func RestoreAll() {
	activeMu.Lock()
	sessions := make([]*Session, len(active))
	copy(sessions, active)
	activeMu.Unlock()
	for _, s := range sessions {
		_ = s.Close()
	}
}

// IsEOF reports whether err is the end of the key stream.
func IsEOF(err error) bool { return errors.Is(err, io.EOF) }
