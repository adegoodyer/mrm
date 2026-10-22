#!/usr/bin/env python3
"""Render terminal output as an SVG terminal window for the docs.

Input is either text with ANSI SGR colours (from `script -qc "mrm status"`)
or a JSON cell grid dumped from the TUI's simulated screen. The SVG follows
the reader's light or dark theme (prefers-color-scheme) and positions every
character on the grid, so columns stay aligned whatever monospace font
the reader's system has, and whatever it substitutes for arrows.

    term2svg.py ansi <in.txt> <out.svg> --title "mrm status" [--prompt "mrm status"]
    term2svg.py grid <in.json> <out.svg> --title "mrm"
"""
import argparse
import html
import json
import re

CW, LH, FS, PADX, PADY, BAR = 8.4, 19, 14, 18, 14, 34

# Terminal palette per theme: default fg/bg, then ANSI 0-7 by name.
STYLE = """
  .bg{fill:#ffffff} .bar{fill:#eef1f4} .edge{stroke:#d0d7de} .t{fill:#1f2328}
  .title{fill:#59636e} .dim{fill:#6e7781} .c1{fill:#cf222e} .c2{fill:#1a7f37}
  .c3{fill:#9a6700} .c4{fill:#0969da} .c5{fill:#8250df} .c6{fill:#1b7c83} .c7{fill:#6e7781}
  .rev{fill:#1f2328} .revt{fill:#ffffff} .hdr{fill:#0969da}
  @media (prefers-color-scheme: dark){
    .bg{fill:#0d1117} .bar{fill:#161b22} .edge{stroke:#30363d} .t{fill:#e6edf3}
    .title{fill:#9198a1} .dim{fill:#8b949e} .c1{fill:#ff7b72} .c2{fill:#3fb950}
    .c3{fill:#d29922} .c4{fill:#58a6ff} .c5{fill:#bc8cff} .c6{fill:#39c5cf} .c7{fill:#8b949e}
    .rev{fill:#e6edf3} .revt{fill:#0d1117} .hdr{fill:#58a6ff}
  }
  text{font-family:ui-monospace,"SF Mono","Cascadia Mono","JetBrains Mono",Menlo,Consolas,"DejaVu Sans Mono",monospace;font-size:14px;white-space:pre}
  .b{font-weight:700}
"""


def parse_ansi(text):
    """Text with SGR codes -> rows of (char, cls, bold)."""
    rows, row, col = [], [], 0
    cls, bold = "t", False
    i = 0
    while i < len(text):
        ch = text[i]
        if ch == "\x1b":
            m = re.match(r"\x1b\[([0-9;]*)([A-Za-z])", text[i:])
            if m:
                if m.group(2) == "m":
                    for code in (m.group(1) or "0").split(";"):
                        code = int(code or 0)
                        if code == 0:
                            cls, bold = "t", False
                        elif code == 1:
                            bold = True
                        elif code == 2:
                            cls = "dim"
                        elif 31 <= code <= 37:
                            cls = f"c{code - 30}"
                elif m.group(2) == "K":
                    del row[col:]
                i += len(m.group(0))
                continue
        if ch == "\r":
            col = 0
        elif ch == "\n":
            rows.append(row)
            row, col = [], 0
        else:
            cell = (ch, cls, bold)
            if col < len(row):
                row[col] = cell
            else:
                row.append(cell)
            col += 1
        i += 1
    if row:
        rows.append(row)
    while rows and not rows[-1]:
        rows.pop()
    return rows


def parse_grid(data):
    """JSON grid from the TUI dump -> rows of (char, cls, bold, rev)."""
    names = {1: "c1", 2: "c2", 3: "c3", 4: "c4", 5: "c5", 6: "c6", 7: "c7", 9: "c1", 10: "c2", 11: "c3", 12: "c4", 13: "c5", 14: "c6"}
    rows = []
    for line in data["rows"]:
        row = []
        for c in line:
            cls = names.get(c.get("fg", -1), "t")
            if c.get("dim") and cls == "t":
                cls = "dim"
            row.append((c["r"] or " ", cls, bool(c.get("bold")), bool(c.get("rev"))))
        rows.append(row)
    return rows


def runs(row):
    """Group cells into runs of the same style."""
    out, cur, start = [], None, 0
    for col, cell in enumerate(row):
        if cur and cur[1] == cell[1:]:
            cur[0] += cell[0]
            continue
        if cur:
            out.append((start, cur[0], cur[1]))
        cur, start = [cell[0], cell[1:]], col
    if cur:
        out.append((start, cur[0], cur[1]))
    return out


def render(rows, title, prompt=None, cols=None):
    if prompt is not None:
        rows = [[("$", "dim", False), (" ", "t", False)] + [(c, "t", True) for c in prompt], []] + rows
    cols = cols or max((len(r) for r in rows), default=40)
    w = round(PADX * 2 + cols * CW)
    h = round(BAR + PADY * 2 + len(rows) * LH)
    o = [f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}" role="img" aria-label="{html.escape(title)}">',
         f"<title>{html.escape(title)}</title><style>{STYLE}</style>",
         f'<rect class="bg edge" x="0.5" y="0.5" width="{w - 1}" height="{h - 1}" rx="8"/>',
         f'<path class="bar" d="M8.5 .5h{w - 17}a8 8 0 0 1 8 8V{BAR}H.5V8.5a8 8 0 0 1 8-8z"/>',
         f'<line class="edge" x1="0" y1="{BAR}" x2="{w}" y2="{BAR}"/>']
    for i, fill in enumerate(("#ff5f57", "#febc2e", "#28c840")):
        o.append(f'<circle cx="{20 + i * 18}" cy="{BAR / 2}" r="5.5" fill="{fill}"/>')
    o.append(f'<text class="title" x="{w / 2}" y="{BAR / 2 + 5}" text-anchor="middle">{html.escape(title)}</text>')
    for y, row in enumerate(rows):
        base = BAR + PADY + (y + 1) * LH - 5
        # Reverse video (the TUI's cursor row): one rectangle per span, so
        # differently coloured cells don't leave seams between them.
        rev = "".join("r" if len(c) > 3 and c[3] else " " for c in row)
        for m in re.finditer(r"r+", rev):
            o.append(f'<rect class="rev" x="{PADX + m.start() * CW:.1f}" y="{base - LH + 5}" width="{len(m.group(0)) * CW:.1f}" height="{LH}"/>')
        for col, text, key in runs(row):
            cls, bold = key[0], key[1]
            if len(key) > 2 and key[2]:
                cls = "revt"
            # One text element per word, each character at its own x, so the
            # grid holds whatever the font's advance width is.
            c = cls + (" b" if bold else "")
            for m in re.finditer(r"\S+(?: \S+)*", text):
                xs = " ".join(f"{PADX + (col + m.start() + k) * CW:.1f}" for k in range(len(m.group(0))))
                o.append(f'<text class="{c}" x="{xs}" y="{base}" xml:space="preserve">{html.escape(m.group(0))}</text>')
    o.append("</svg>")
    return "\n".join(o) + "\n"


def main():
    p = argparse.ArgumentParser()
    p.add_argument("mode", choices=["ansi", "grid"])
    p.add_argument("src")
    p.add_argument("out")
    p.add_argument("--title", default="mrm")
    p.add_argument("--prompt")
    p.add_argument("--skip", type=int, default=0, help="drop this many leading rows")
    a = p.parse_args()
    if a.mode == "ansi":
        rows = parse_ansi(open(a.src, encoding="utf-8", newline="").read())
        cols = None
    else:
        data = json.load(open(a.src, encoding="utf-8"))
        rows, cols = parse_grid(data), data["cols"]
    rows = rows[a.skip:]
    open(a.out, "w", encoding="utf-8").write(render(rows, a.title, a.prompt, cols))


if __name__ == "__main__":
    main()
