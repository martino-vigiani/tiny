package main

// spin: run a child command while drawing a spinner beside a message.
//
//	tiny spin "Building…" -- go build ./...
//	tiny spin "Deploying" -- ./deploy.sh --prod
//	tiny spin --stream "Deploying" -- ./deploy.sh --prod
//
// The whole point of the command is that it is safe to put in the middle of
// a pipeline: the child's stdout passes straight through to tiny's stdout
// untouched, and tiny's own exit code is the child's, so `tiny spin "…" --
// cmd | jq; echo $?` behaves exactly as if spin weren't there. Everything
// spin itself draws goes to ui.Err, per the package's stdout-is-payload rule.

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/martino-vigiani/tiny/internal/theme"
)

// spinFrames are the animation frames. Plain ASCII rather than a themed
// symbol: the theme package has no spinner concept yet, and inventing one
// here would make this command the only place a "spinner style" theme field
// existed.
var spinFrames = []string{"|", "/", "-", "\\"}

// frameInterval is how often the spinner redraws. A package variable rather
// than a constant so a test can shrink it instead of sleeping for real time.
var frameInterval = 80 * time.Millisecond

// capturedStderrLimit bounds how much of the child's stderr spin buffers
// while animating. See the comment above cmd.Stderr's assignment in runSpin
// for why buffering happens at all and why it must not be unbounded.
const capturedStderrLimit = 64 * 1024

func runSpin(args []string, ui *UI) int {
	message, command, stream, err := parseSpinArgs(args)
	if err != nil {
		return usageError(ui, "%s\nusage: tiny spin [--stream] <message> -- <command> [args...]", err)
	}
	// message is caller-controlled text (argv, possibly built from a file or
	// another command's output) that is written to the terminal on every
	// redraw, so its control bytes go — a stray OSC 52 in supplied text would
	// otherwise reach the clipboard. Tabs go too, which is the one way this
	// differs from what `say` does to its input: the spinner draws a line
	// clipped to a fixed width, and a tab advances the cursor to the next tab
	// stop while measuring as a single cell. The line then wraps, and a
	// wrapped line is out of reach of the next frame's erase — the spinner
	// leaves debris behind for the rest of the run.
	message = sanitizeText(message, false, false)

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	// plain covers two cases that both mean "no animation": no terminal to
	// animate on, and --stream, which trades the animation for seeing the
	// child's stderr as it happens instead of only after it exits.
	plain := !ui.TTY || stream

	// The child's stderr must not collide with the spinner redrawing the same
	// line. When animating, we buffer it — capped at capturedStderrLimit so a
	// chatty child cannot grow tiny's memory without bound — and flush it
	// once the spinner line is erased, so it always appears as a clean block
	// after the animation instead of tearing a frame in half. The cost is
	// that a long build shows nothing until it exits, which is exactly when
	// the user most wants to see errors arriving; --stream is the escape
	// hatch that trades the animation for that visibility.
	var captured boundedBuffer
	if plain {
		cmd.Stderr = ui.Err
	} else {
		captured.limit = capturedStderrLimit
		cmd.Stderr = &captured
	}

	if plain {
		// No animation: print the message once, plainly. Without a TTY this
		// is also the no-escape-sequences rule — a CI log full of spinner
		// frames is why people turn spinners off.
		ui.Writef("%s\n", message)
	} else {
		ui.Write(theme.HideCursor)
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(ui.Err, "tiny: could not start %q: %v\n", command[0], err)
		if !plain {
			// Restore the terminal before returning: a hidden cursor left
			// behind makes the user's shell look broken.
			ui.Write(theme.ColumnZero + theme.EraseEOL + theme.ShowCursor)
		}
		return 1
	}

	// Forward the signals a shell would deliver to a foreground process, so
	// Ctrl-C on `tiny spin` stops the child instead of leaving it orphaned.
	// stop, rather than closing sigCh itself, is what lets the goroutine exit
	// without racing signal.Stop (which does not close the channel).
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	stop := make(chan struct{})
	defer func() {
		signal.Stop(sigCh)
		close(stop)
	}()
	go func() {
		for {
			select {
			case sig := <-sigCh:
				_ = cmd.Process.Signal(sig)
			case <-stop:
				return
			}
		}
	}()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var waitErr error
	if plain {
		waitErr = <-done
	} else {
		waitErr = animate(ui, message, done)
		// Wipe the spinner line, then flush whatever the child wrote to
		// stderr (plus a note if it was truncated), then hand the cursor
		// back — in that order, so the flushed text lands on a clean line
		// instead of overwriting or trailing a half-erased frame.
		ui.Write(theme.ColumnZero + theme.EraseEOL)
		if captured.buf.Len() > 0 {
			ui.Write(captured.buf.String())
		}
		if captured.truncated {
			fmt.Fprintf(ui.Err, "tiny: spin: child's stderr exceeded %dKB and was truncated; rerun with --stream to see it all live\n",
				capturedStderrLimit/1024)
		}
		ui.Write(theme.ShowCursor)
	}

	if waitErr == nil {
		return 0
	}
	if exitErr, ok := waitErr.(*exec.ExitError); ok {
		return exitCodeFrom(exitErr.ProcessState)
	}
	// Wait failing without an ExitError means something more basic broke
	// (e.g. an I/O error), not that the child ran and returned a status.
	fmt.Fprintf(ui.Err, "tiny: %v\n", waitErr)
	return 1
}

// parseSpinArgs splits "[--stream] <message...> -- <command...>" the way the
// usage line promises: an optional leading --stream flag, then everything up
// to -- is the message, everything after is the command to run.
//
// --stream is only recognised as the very first argument rather than parsed
// out from anywhere in args (the way say's --style is): the message here is
// free-form text, and a flag scanner that can appear after it would treat a
// message that happens to start with "--stream" as the flag instead of as
// text.
func parseSpinArgs(args []string) (message string, command []string, stream bool, err error) {
	if len(args) > 0 && args[0] == "--stream" {
		stream = true
		args = args[1:]
	}

	sep := -1
	for i, a := range args {
		if a == "--" {
			sep = i
			break
		}
	}
	if sep == -1 {
		return "", nil, false, fmt.Errorf("missing -- before the command")
	}
	if sep == 0 {
		return "", nil, false, fmt.Errorf("missing message")
	}
	if len(args) == sep+1 {
		return "", nil, false, fmt.Errorf("missing command after --")
	}
	return strings.Join(args[:sep], " "), args[sep+1:], stream, nil
}

// animate redraws the spinner on one line, at frameInterval, until done
// yields the child's Wait error. It never busy-loops: the only work between
// frames is blocking on a ticker or the done channel.
func animate(ui *UI, message string, done <-chan error) error {
	draw := func(frame int) {
		prefix := spinFrames[frame%len(spinFrames)] + " "
		// A message longer than the terminal wraps onto a second physical
		// row; ColumnZero+EraseEOL then only erases the last row, so every
		// subsequent frame leaves the wrapped remainder of earlier frames on
		// screen and the display fills with spinner debris. Clipping to the
		// line's own budget is what keeps every frame to exactly one row.
		// cellWidth/truncateCells (box.go, same package) measure in terminal
		// cells rather than bytes or runes for the same reason box's borders
		// do: a CJK character is two columns wide, so counting runes would
		// let a message wrap at roughly half the terminal's actual width.
		avail := ui.Width() - 1 - cellWidth(prefix)
		if avail < 0 {
			avail = 0
		}
		ui.Write(theme.ColumnZero)
		ui.Write(prefix)
		ui.Write(truncateCells(message, ui.Theme.Ellipsis, avail))
		ui.Write(theme.EraseEOL)
	}
	draw(0)

	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()
	frame := 1
	for {
		select {
		case err := <-done:
			return err
		case <-ticker.C:
			draw(frame)
			frame++
		}
	}
}

// exitCodeFrom turns a finished child's process state into the code tiny
// should exit with. A process killed by a signal has no normal exit code
// (ExitCode reports -1 for it), so it is reported the way a shell reports a
// signaled foreground job: 128+signal.
func exitCodeFrom(state *os.ProcessState) int {
	if ws, ok := state.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return state.ExitCode()
}

// boundedBuffer is a bytes.Buffer-backed io.Writer that stops growing past
// limit and remembers that it did, instead of buffering an unbounded amount
// of a chatty child's stderr. Write never fails because of the cap: reporting
// an error to cmd.Wait would make spin treat a long log as a broken pipe
// rather than the thing it actually is, a truncated one.
type boundedBuffer struct {
	buf       strings.Builder
	limit     int
	truncated bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	room := b.limit - b.buf.Len()
	if room <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > room {
		b.buf.Write(p[:room])
		b.truncated = true
		return len(p), nil
	}
	b.buf.Write(p)
	return len(p), nil
}
