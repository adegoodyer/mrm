#!/usr/bin/env python3
"""Draw the README diagrams: how mrm fits together, and what sync does.

    diagrams.py <out dir>

Writes how-it-works.svg and sync-rules.svg. Both follow the reader's light
or dark theme (prefers-color-scheme) and use system fonts only.
"""
import html
import sys

STYLE = """
  .bg{fill:#ffffff;stroke:#d0d7de} .card{fill:#f6f8fa;stroke:#d0d7de} .hi{fill:#ddf4ff;stroke:#54aeff}
  .t{fill:#1f2328} .m{fill:#59636e} .ln{stroke:#8c959f;fill:none} .ah{fill:#8c959f}
  .ok{fill:#1a7f37} .warn{fill:#9a6700} .bad{fill:#cf222e} .acc{fill:#0969da} .neu{fill:#59636e}
  .okb{fill:#dafbe1} .warnb{fill:#fff8c5} .badb{fill:#ffebe9} .neub{fill:#eaeef2} .accb{fill:#ddf4ff}
  @media (prefers-color-scheme: dark){
    .bg{fill:#0d1117;stroke:#30363d} .card{fill:#161b22;stroke:#30363d} .hi{fill:#0c2d4b;stroke:#1f6feb}
    .t{fill:#e6edf3} .m{fill:#9198a1} .ln{stroke:#6e7681} .ah{fill:#6e7681}
    .ok{fill:#3fb950} .warn{fill:#d29922} .bad{fill:#ff7b72} .acc{fill:#58a6ff} .neu{fill:#9198a1}
    .okb{fill:#12261e} .warnb{fill:#2b2111} .badb{fill:#2d1517} .neub{fill:#21262d} .accb{fill:#0c2d4b}
  }
  text{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","Noto Sans",Helvetica,Arial,sans-serif;font-size:14px}
  .mono{font-family:ui-monospace,"SF Mono","Cascadia Mono",Menlo,Consolas,"DejaVu Sans Mono",monospace;font-size:13px}
  .h{font-weight:600} .s{font-size:12.5px} .xl{font-size:16px;font-weight:600}
"""


def esc(s):
    return html.escape(s)


class SVG:
    def __init__(self, w, h, title):
        self.w, self.h, self.title, self.o = w, h, title, []

    def add(self, s):
        self.o.append(s)

    def rect(self, x, y, w, h, cls="card", r=8):
        self.add(f'<rect class="{cls}" x="{x}" y="{y}" width="{w}" height="{h}" rx="{r}"/>')

    def text(self, x, y, s, cls="t", anchor="start"):
        self.add(f'<text class="{cls}" x="{x}" y="{y}" text-anchor="{anchor}">{esc(s)}</text>')

    def rich(self, x, y, parts, anchor="start"):
        """parts: [(text, cls)] on one line."""
        spans = "".join(f'<tspan class="{c}">{esc(t)}</tspan>' for t, c in parts)
        self.add(f'<text x="{x}" y="{y}" text-anchor="{anchor}">{spans}</text>')

    def arrow(self, pts, dashed=False):
        d = "M" + " L".join(f"{x} {y}" for x, y in pts)
        dash = ' stroke-dasharray="5 4"' if dashed else ""
        self.add(f'<path class="ln" d="{d}" stroke-width="1.5"{dash}/>')
        (x1, y1), (x2, y2) = pts[-2], pts[-1]
        if x2 == x1:
            s = 1 if y2 > y1 else -1
            tri = f"{x2},{y2} {x2 - 5},{y2 - 8 * s} {x2 + 5},{y2 - 8 * s}"
        else:
            s = 1 if x2 > x1 else -1
            tri = f"{x2},{y2} {x2 - 8 * s},{y2 - 5} {x2 - 8 * s},{y2 + 5}"
        self.add(f'<polygon class="ah" points="{tri}"/>')

    def pill(self, x, y, w, glyph, label, tone):
        self.rect(x, y, w, 30, tone + "b", 15)
        self.rich(x + 14, y + 20, [(glyph + "  ", tone + " h"), (label, "t")])

    def write(self, path):
        head = (f'<svg xmlns="http://www.w3.org/2000/svg" width="{self.w}" height="{self.h}" '
                f'viewBox="0 0 {self.w} {self.h}" role="img" aria-label="{esc(self.title)}">'
                f"<title>{esc(self.title)}</title><style>{STYLE}</style>"
                f'<rect class="bg" x="0.5" y="0.5" width="{self.w - 1}" height="{self.h - 1}" rx="10"/>')
        open(path, "w", encoding="utf-8").write(head + "\n" + "\n".join(self.o) + "\n</svg>\n")


def how_it_works(path):
    s = SVG(880, 430, "How mrm works: workspaces are discovered, one engine runs git, three front ends show it")
    # Column 1: your folders
    s.rect(24, 24, 230, 250)
    s.text(40, 52, "Your folders", "t xl")
    s.text(40, 72, "workspaces in the config", "m s")
    y = 96
    for ws, root, repos in (("work", "~/work", ["services/api", "services/web", "platform/auth", "…"]),
                            ("personal", "~/src", ["dotfiles", "blog"])):
        s.rich(40, y, [(ws, "t h"), ("  " + root, "m mono")])
        for r in repos:
            y += 20
            s.text(56, y, r, "m mono")
        y += 30
    # Column 2: engine
    s.rect(324, 24, 232, 250, "hi")
    s.text(340, 52, "Engine", "t xl")
    s.text(340, 72, "the only part that runs git", "m s")
    for i, line in enumerate(["8 repos at a time", "no prompts, ssh BatchMode", "a timeout on every git call",
                              "fetch + fast-forward only", "one sync per workspace (lock)"]):
        s.text(340, 104 + i * 24, "· " + line, "t")
    s.arrow([(254, 150), (322, 150)])
    s.text(288, 140, "discover", "m s", "middle")
    # Column 3: front ends
    ends = [("mrm status · sync · exec …", "CLI: scripts, cron, --json"),
            ("mrm", "interactive view in the terminal"),
            ("mrm web", "web UI on 127.0.0.1")]
    for i, (cmd, what) in enumerate(ends):
        y = 24 + i * 86
        s.rect(626, y, 230, 78)
        s.text(642, y + 30, cmd, "t mono h")
        s.text(642, y + 54, what, "m s")
        s.arrow([(556, 150), (590, 150), (590, y + 39), (624, y + 39)])
    # Bottom: what is around it, under the column it belongs to
    boxes = [(24, 230, "Around a sync", ["systemd timer runs mrm sync", "post_sync hooks · Gotify / ntfy"]),
             (324, 232, "Cache and history", ["status.json · runs.jsonl · mrm.log", "in your XDG cache and state dirs"]),
             (626, 230, "Forges (optional)", ["read-only API tokens: open PRs,", "CI state, org repo lists"])]
    for x, w, title, lines in boxes:
        s.rect(x, 318, w, 88)
        s.text(x + 16, 346, title, "t h")
        for i, l in enumerate(lines):
            s.text(x + 16, 370 + i * 20, l, "m s")
    s.arrow([(400, 274), (400, 296), (139, 296), (139, 316)])
    s.arrow([(480, 274), (480, 316)])
    s.arrow([(741, 274), (741, 316)], dashed=True)
    s.write(path)


def sync_rules(path):
    rows = [
        ("Fetch fails or times out", ["the reason in plain words: offline?, no access, key rejected"], "✗", "failed", "bad"),
        ("Merge, rebase, cherry-pick or revert in progress", ["finish it in git; mrm doesn't touch it"], "!", "skipped", "warn"),
        ("Detached HEAD", ["fetched only"], "!", "skipped", "warn"),
        ("On another branch", ["the default branch is fast-forwarded in place if behind;",
                               "your branch is left alone (ff_current: true moves it too)"], "✓", "default branch updated", "ok"),
        ("On the default branch, not behind", ["ahead is unpushed work, not a sync problem"], "·", "current", "neu"),
        ("Ahead and behind", ["rebase or merge it yourself"], "!", "skipped: diverged", "warn"),
        ("Tracked files changed", ["untracked files don't count; with ff_dirty, only files",
                                   "the update also changes"], "!", "skipped: dirty", "warn"),
        ("Clean and strictly behind", ["git merge --ff-only"], "✓", "synced", "ok"),
    ]
    gap, top = 16, 126
    heights = [36 + 18 * len(n) for _, n, *_ in rows]
    h = top + sum(heights) + gap * len(rows) + 100
    s = SVG(880, h, "What mrm sync does to each repo, in order")
    s.text(24, 44, "What sync does to each repo", "t xl")
    s.text(24, 66, "8 repos at a time. The first question that matches decides what happens.", "m")
    s.rect(24, 80, 832, 30, "accb", 6)
    s.rich(40, 100, [("git fetch --prune origin", "t mono h"), ("   then origin/HEAD is re-checked at most once a day", "m s")])
    y = top
    for i, ((q, notes, glyph, label, tone), bh) in enumerate(zip(rows, heights)):
        s.rect(24, y, 470, bh)
        s.text(40, y + 24, f"{i + 1}.  {q}", "t h")
        for j, n in enumerate(notes):
            s.text(60, y + 44 + j * 18, n, "m s")
        mid = y + bh / 2
        s.arrow([(494, mid), (548, mid)])
        s.pill(550, mid - 15, 306, glyph, label, tone)
        if i < len(rows) - 1:
            s.arrow([(60, y + bh), (60, y + bh + gap)])
            s.text(70, y + bh + 12, "no", "m s")
        y += bh + gap
    s.rect(24, y + 4, 832, 34, "card", 6)
    s.rich(40, y + 26, [("If HEAD moved: ", "t h"), ("post_sync hooks run (a failing hook is a warning, the sync still counts).", "m")])
    s.rich(24, y + 72, [("Never: ", "bad h"), ("stash · reset · rebase · merge commits · force · branch deletes.", "t"),
                        ("  Exit 0 all good, 2 something needs attention, 1 mrm failed.", "m s")])
    s.write(path)


if __name__ == "__main__":
    out = sys.argv[1] if len(sys.argv) > 1 else "."
    how_it_works(f"{out}/how-it-works.svg")
    sync_rules(f"{out}/sync-rules.svg")
