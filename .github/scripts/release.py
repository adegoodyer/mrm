#!/usr/bin/env python3
"""Cut the next release on master - run weekly by .github/workflows/release.yml.

What counts as new since the last vX.Y.Z tag:
  - notes under [Unreleased] in CHANGELOG.md
  - Go module versions in go.mod (Renovate's merged PRs)
  - the Go version in mise.toml (what builds the binaries)
None of those -> nothing to release.

The version: Added, Changed, Deprecated or Removed notes -> minor; anything
else (Fixed, Security, only dependency or Go bumps) -> patch. VERSION=X.Y.Z
chooses it instead (e.g. 1.0.0).

It moves the notes into a dated section, adds the dependency and Go changes
under Changed, commits "release X.Y.Z" and tags vX.Y.Z. Pushing and
publishing are the workflow's steps.

DRY_RUN=1 prints the version and section and changes nothing
(task release:next).
"""

import datetime
import os
import re
import subprocess
import sys

CHANGELOG = "CHANGELOG.md"
ORDER = ["Added", "Changed", "Deprecated", "Removed", "Fixed", "Security"]
MINOR = {"Added", "Changed", "Deprecated", "Removed"}


def git(*args):
    return subprocess.run(["git", *args], check=True, capture_output=True, text=True).stdout.strip()


def at_tag(tag, path):
    """The file's text at the tag, or None if it did not exist there."""
    r = subprocess.run(["git", "show", f"{tag}:{path}"], capture_output=True, text=True)
    return r.stdout if r.returncode == 0 else None


def read(path):
    try:
        with open(path) as f:
            return f.read()
    except FileNotFoundError:
        return None


def requires(gomod):
    """module -> (version, indirect) from go.mod's require lines."""
    mods = {}
    block = False
    for line in (gomod or "").splitlines():
        s = line.strip()
        if s.startswith("require ("):
            block = True
            continue
        if block and s == ")":
            block = False
            continue
        if s.startswith("require "):
            s = s[len("require "):]
        elif not block:
            continue
        m = re.match(r"([^\s/][^\s]*)\s+(v\S+)(\s*//\s*indirect)?", s)
        if m:
            mods[m.group(1)] = (m.group(2), bool(m.group(3)))
    return mods


def go_version(mise_toml):
    m = re.search(r'^go\s*=\s*"([^"]+)"', mise_toml or "", re.M)
    return m.group(1) if m else None


def dependency_notes(tag):
    """Changelog lines for go.mod and Go version changes since the tag."""
    old, new = requires(at_tag(tag, "go.mod")), requires(read("go.mod"))
    direct, indirect = [], 0
    for mod in sorted(set(old) | set(new)):
        o, n = old.get(mod), new.get(mod)
        if o and n and o[0] == n[0]:
            continue
        if (n or o)[1]:
            indirect += 1
        elif o and n:
            direct.append(f"`{mod}` {o[0]} to {n[0]}")
        elif n:
            direct.append(f"`{mod}` {n[0]} (new)")
        else:
            direct.append(f"`{mod}` (removed)")
    notes = []
    if direct or indirect:
        line = ", ".join(direct)
        if indirect:
            more = f"{indirect} indirect module{'s' if indirect != 1 else ''}"
            line = f"{line}, and {more}" if line else more
        notes.append(f"- Dependency updates: {line}.")
    old_go, new_go = go_version(at_tag(tag, "mise.toml")), go_version(read("mise.toml"))
    if old_go and new_go and old_go != new_go:
        notes.append(f"- Built with Go {new_go} (was {old_go}).")
    return notes


def split_sections(body):
    """Preamble lines, then [heading, lines] for each ### section."""
    pre, secs, cur = [], [], None
    for line in body.splitlines():
        h = re.match(r"###\s+(.+?)\s*$", line)
        if h:
            cur = [h.group(1), []]
            secs.append(cur)
        elif cur is None:
            pre.append(line)
        else:
            cur[1].append(line)
    return pre, secs


def trim(lines):
    while lines and not lines[0].strip():
        lines = lines[1:]
    while lines and not lines[-1].strip():
        lines = lines[:-1]
    return lines


def add_changed(secs, notes):
    for name, lines in secs:
        if name == "Changed":
            lines[:] = trim(lines) + notes
            return
    rank = ORDER.index("Changed")
    at = next((i for i, (name, _) in enumerate(secs) if name in ORDER and ORDER.index(name) > rank), len(secs))
    secs.insert(at, ["Changed", notes])


def render(pre, secs):
    blocks = []
    if trim(pre):
        blocks.append("\n".join(trim(pre)))
    for name, lines in secs:
        blocks.append("\n".join([f"### {name}"] + trim(lines)))
    return "\n\n".join(blocks)


def output(**kv):
    path = os.environ.get("GITHUB_OUTPUT")
    if path:
        with open(path, "a") as f:
            for k, v in kv.items():
                f.write(f"{k}={v}\n")


def main():
    dry = bool(os.environ.get("DRY_RUN"))
    tag = git("describe", "--tags", "--abbrev=0", "--match", "v[0-9]*.[0-9]*.[0-9]*")
    major, minor, patch = map(int, tag[1:].split("."))

    text = read(CHANGELOG)
    m = re.search(r"^## \[Unreleased\][^\n]*\n(.*?)(?=^## \[|\Z)", text, re.S | re.M)
    if not m:
        sys.exit(f"{CHANGELOG} has no ## [Unreleased] section")
    pre, secs = split_sections(m.group(1))
    has_notes = bool(trim(pre)) or any(trim(lines) for _, lines in secs)
    secs = [[name, lines] for name, lines in secs if trim(lines)]
    notes = dependency_notes(tag)

    if not has_notes and not notes:
        print(f"Nothing to release since {tag}.")
        output(version="")
        return

    wanted = os.environ.get("VERSION", "").strip().lstrip("v")
    if wanted:
        if not re.fullmatch(r"\d+\.\d+\.\d+", wanted):
            sys.exit(f"VERSION {wanted!r} is not X.Y.Z")
        if tuple(map(int, wanted.split("."))) <= (major, minor, patch):
            sys.exit(f"VERSION {wanted} is not after {tag}")
        version, why = wanted, "chosen"
    elif MINOR & {name for name, _ in secs}:
        version, why = f"{major}.{minor + 1}.0", "minor: " + ", ".join(n for n, _ in secs if n in MINOR) + " notes"
    else:
        version, why = f"{major}.{minor}.{patch + 1}", "patch"

    if notes:
        add_changed(secs, notes)
    date = datetime.datetime.now(datetime.timezone.utc).date().isoformat()
    section = f"## [{version}] - {date}\n\n{render(pre, secs)}"
    text = text[: m.start()] + f"## [Unreleased]\n\n{section}\n\n" + text[m.end():]

    print(f"Next release: {version} ({why}; last {tag})\n\n{section}")
    if dry:
        output(version="")
        return

    with open(CHANGELOG, "w") as f:
        f.write(text)
    git("add", CHANGELOG)
    git("commit", "-q", "-m", f"release {version}")
    git("tag", "-a", f"v{version}", "-m", f"mrm {version}")
    output(version=version)


if __name__ == "__main__":
    main()
