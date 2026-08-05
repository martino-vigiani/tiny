#!/usr/bin/env python3
"""Record a real terminal session and emit an animated SVG.

Written rather than installed: vhs and asciinema are not on this machine, and
a README clip has to come from the program actually running, not from a
reconstruction. The emulator below understands only what tiny emits — carriage
returns, newlines, erase-to-end-of-line, erase-below, cursor-up and SGR colour
— which is enough to replay its frames exactly and nothing more.
"""

import fcntl
import os
import pty
import struct
import termios
import re
import select
import sys
import time
from html import escape

COLS, ROWS = 78, 24

# xterm's first sixteen, close enough for the palette a theme actually uses.
BASE = [
    "#1c1c1c", "#e05252", "#4fb04f", "#c8a24a", "#4f8fd0", "#b06fd0", "#3fa8a8", "#d0d0d0",
    "#6b6b6b", "#ff6b6b", "#5fd75f", "#ffd75f", "#6bb6ff", "#d787ff", "#5fd7d7", "#ffffff",
]
FG_DEFAULT = "#d8d8d8"


def cube(n):
    if n < 16:
        return BASE[n]
    if n < 232:
        n -= 16
        r, g, b = n // 36, (n % 36) // 6, n % 6
        f = lambda v: 0 if v == 0 else 55 + 40 * v
        return "#%02x%02x%02x" % (f(r), f(g), f(b))
    v = 8 + (n - 232) * 10
    return "#%02x%02x%02x" % (v, v, v)


class Screen:
    def __init__(self):
        self.cells = [[(" ", FG_DEFAULT, False, False) for _ in range(COLS)] for _ in range(ROWS)]
        self.x = self.y = 0
        self.fg = FG_DEFAULT
        self.bold = self.dim = False

    def snapshot(self):
        return tuple(tuple(row) for row in self.cells)

    def put(self, ch):
        if self.x >= COLS:
            self.newline()
        self.cells[self.y][self.x] = (ch, self.fg, self.bold, self.dim)
        self.x += 1

    def newline(self):
        self.x = 0
        self.y += 1
        if self.y >= ROWS:
            self.cells.pop(0)
            self.cells.append([(" ", FG_DEFAULT, False, False) for _ in range(COLS)])
            self.y = ROWS - 1

    def erase_eol(self):
        for i in range(self.x, COLS):
            self.cells[self.y][i] = (" ", FG_DEFAULT, False, False)

    def erase_below(self):
        self.erase_eol()
        for r in range(self.y + 1, ROWS):
            self.cells[r] = [(" ", FG_DEFAULT, False, False) for _ in range(COLS)]

    def sgr(self, params):
        i = 0
        if not params:
            params = [0]
        while i < len(params):
            p = params[i]
            if p == 0:
                self.fg, self.bold, self.dim = FG_DEFAULT, False, False
            elif p == 1:
                self.bold = True
            elif p == 2:
                self.dim = True
            elif p == 22:
                self.bold = self.dim = False
            elif 30 <= p <= 37:
                self.fg = BASE[p - 30]
            elif 90 <= p <= 97:
                self.fg = BASE[p - 90 + 8]
            elif p == 39:
                self.fg = FG_DEFAULT
            elif p == 38 and i + 1 < len(params):
                if params[i + 1] == 5 and i + 2 < len(params):
                    self.fg = cube(params[i + 2])
                    i += 2
                elif params[i + 1] == 2 and i + 4 < len(params):
                    self.fg = "#%02x%02x%02x" % tuple(params[i + 2:i + 5])
                    i += 4
            i += 1


CSI = re.compile(rb"\x1b\[([0-9;?]*)([a-zA-Z])")


def replay(data, every=0.06):
    """Feed captured bytes through the emulator, keeping distinct frames."""
    screen = Screen()
    frames = []
    i = 0
    while i < len(data):
        b = data[i]
        if b == 0x1b:
            m = CSI.match(data, i)
            if m:
                raw, final = m.group(1), m.group(2).decode()
                nums = [int(n) for n in raw.decode().split(";") if n.isdigit()]
                if final == "m":
                    screen.sgr(nums)
                elif final == "K":
                    screen.erase_eol()
                elif final == "J":
                    screen.erase_below()
                elif final == "A":
                    screen.y = max(0, screen.y - (nums[0] if nums else 1))
                elif final == "G":
                    screen.x = 0
                i = m.end()
                continue
            i += 1
            continue
        ch = chr(b)
        if ch == "\r":
            screen.x = 0
        elif ch == "\n":
            screen.newline()
        elif ch == "\t":
            screen.x = min(COLS - 1, (screen.x // 8 + 1) * 8)
        elif b >= 0x20:
            # Decode one UTF-8 rune.
            n = 1
            if b >= 0xf0:
                n = 4
            elif b >= 0xe0:
                n = 3
            elif b >= 0xc0:
                n = 2
            try:
                ch = data[i:i + n].decode()
            except UnicodeDecodeError:
                ch = "?"
            screen.put(ch)
            i += n
            continue
        i += 1
        snap = screen.snapshot()
        if not frames or frames[-1] != snap:
            frames.append(snap)
    return frames


def sample(frames, want):
    """Keep at most `want` frames, evenly spaced, always keeping the last."""
    # Drop the empty lead-in: a clip that opens on a blank terminal wastes the
    # only second most readers give it.
    blank = Screen().snapshot()
    while frames and frames[0] == blank:
        frames.pop(0)
    if len(frames) <= want:
        return frames
    step = len(frames) / want
    out = [frames[int(k * step)] for k in range(want)]
    out[-1] = frames[-1]
    return out


def crop(frames):
    """Trim the rows no frame ever wrote to.

    The emulator always keeps a full screen, but a clip that reserves a third
    of its height for blank terminal reads as a mistake in the README.
    """
    used = 1
    for frame in frames:
        for r, row in enumerate(frame):
            if any(cell[0].strip() for cell in row):
                used = max(used, r + 1)
    return [frame[:used] for frame in frames], used


def to_svg(frames, seconds_per_frame, title):
    frames, rows = crop(frames)
    cw, ch, pad = 8.4, 19.0, 18
    w = int(COLS * cw + pad * 2)
    h = int(rows * ch + pad * 2 + 14)
    total = len(frames) * seconds_per_frame

    out = [
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w} {h}" width="{w}" height="{h}" '
        f'font-family="SFMono-Regular,Menlo,Consolas,monospace" font-size="13">',
        "<style>",
        f"  .f {{ opacity: 0; animation: p {total:.2f}s steps(1,end) infinite; }}",
        "  @keyframes p { 0%,100% { opacity: 0 } }",
        # A viewer that does not animate — a static thumbnail, a PDF export,
        # anything sanitising CSS — must still show something worth seeing,
        # so the final frame is the one visible at rest.
        "  .still { opacity: 1 }",
    ]
    # Each frame is visible for exactly its slice of the loop.
    for k in range(len(frames)):
        a = 100.0 * k / len(frames)
        b = 100.0 * (k + 1) / len(frames)
        out.append(
            f"  @keyframes k{k} {{ 0%,{a:.3f}% {{ opacity:0 }} {a:.3f}%,{b:.3f}% {{ opacity:1 }} "
            f"{b:.3f}%,100% {{ opacity:0 }} }}"
        )
        out.append(f"  .f{k} {{ animation-name: k{k} }}")
    out.append("</style>")
    out.append(f'<rect width="{w}" height="{h}" rx="10" fill="#12141a"/>')
    for i, c in enumerate(("#ff5f57", "#febc2e", "#28c840")):
        out.append(f'<circle cx="{pad + 6 + i * 16}" cy="16" r="5.5" fill="{c}"/>')
    out.append(
        f'<text x="{w/2}" y="20" fill="#6b7280" font-size="11" text-anchor="middle">{escape(title)}</text>'
    )

    for k, frame in enumerate(frames):
        still = " still" if k == len(frames) - 1 else ""
        out.append(f'<g class="f f{k}{still}">')
        for r, row in enumerate(frame):
            # Group each run of identical styling into one <text> element.
            runs, cur = [], None
            for c, (chx, fg, bold, dim) in enumerate(row):
                style = (fg, bold, dim)
                if cur and cur[0] == style and cur[2] == c:
                    cur = (style, cur[1], c + 1, cur[3] + chx)
                else:
                    if cur and cur[3].strip():
                        runs.append(cur)
                    cur = (style, c, c + 1, chx)
            if cur and cur[3].strip():
                runs.append(cur)
            for (fg, bold, dim), start, _, text in runs:
                y = pad + 14 + r * ch
                x = pad + start * cw
                extra = ' font-weight="bold"' if bold else ""
                op = ' opacity="0.55"' if dim else ""
                out.append(
                    f'<text x="{x:.1f}" y="{y:.1f}" fill="{fg}"{extra}{op} '
                    f'xml:space="preserve">{escape(text)}</text>'
                )
        out.append("</g>")
    out.append("</svg>")
    return "\n".join(out)


def record(cmd, keys, settle=0.35, run_for=None):
    """Run cmd under a pty, feeding keys when it goes quiet."""
    pid, fd = pty.fork()
    if pid == 0:
        os.environ["TERM"] = "xterm-256color"
        os.environ["COLUMNS"] = str(COLS)
        os.environ["LINES"] = str(ROWS)
        os.execvp("/bin/sh", ["sh", "-c", cmd])
    # The recorded program asks the terminal how wide it is; COLUMNS is not
    # what it reads. Without this the program lays out for 80 columns while the
    # SVG draws 78, and every line is a little truncated for no visible reason.
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", ROWS, COLS, 0, 0))

    out = b""
    i = 0
    deadline = time.time() + (run_for or 30)
    while time.time() < deadline:
        r, _, _ = select.select([fd], [], [], settle)
        if r:
            try:
                chunk = os.read(fd, 65536)
            except OSError:
                break
            if not chunk:
                break
            out += chunk
        elif i < len(keys):
            os.write(fd, keys[i])
            i += 1
        elif run_for is None:
            break
    try:
        os.kill(pid, 9)
    except ProcessLookupError:
        pass
    return out


if __name__ == "__main__":
    import json
    spec = json.loads(sys.argv[1])
    data = record(spec["cmd"], [k.encode() for k in spec.get("keys", [])],
                  run_for=spec.get("run_for"))
    frames = sample(replay(data), spec.get("max_frames", 36))
    svg = to_svg(frames, spec.get("spf", 0.28), spec.get("title", "tiny"))
    with open(spec["out"], "w") as f:
        f.write(svg)
    print(f'{spec["out"]}: {len(frames)} frames, {len(svg)} bytes')
