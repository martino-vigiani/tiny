# Media

`tiny-demo.gif` / `tiny-demo.mp4` is the README hero: a Remotion walkthrough
of the YAML → wizard → JSON → `-defaults` story.

`wizard.svg` and `display.svg` are recordings of tiny actually running, not
mock-ups: `record.py` drives the real binary under a pty, replays the bytes it
emitted through a small terminal emulator, and writes the frames out as an
animated SVG.

It is written rather than installed because neither vhs nor asciinema was
available, and because a clip of a terminal program should come from the
program. The emulator understands exactly what tiny emits — carriage returns,
newlines, erase-to-end-of-line, erase-below, cursor-up and SGR colour — and
nothing else.

    python3 record.py '{"cmd": "tiny demo", "out": "demo.svg", "title": "tiny demo"}'

The final frame is the one visible at rest, so a viewer that does not animate
still shows the finished session rather than a blank terminal.
