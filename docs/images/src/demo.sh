#!/bin/bash
# demo.sh <dir> [github]
#
# Builds the demo fleet the docs images are made from: nine repos with
# made-up names under <dir>/home/{work,src}, their bare remotes under
# <dir>/remotes, and an mrm config, status cache and forge cache under
# <dir>/xdg. Nothing outside <dir> is touched: HOME and git's global config
# point inside it.
#
# Remotes are named git@github.com:acme/<repo>.git and resolve to the local
# bare repos through url.insteadOf, so fetch and sync work offline. With
# "github", the insteadOf rule is dropped and the remotes keep their GitHub
# names, so mrm shows forge links and the seeded PR/CI answers (status, TUI
# and web pictures; nothing fetches then).
set -euo pipefail
D=$1
[ -e "$D" ] && rm -r "$D"
mkdir -p "$D/remotes" "$D/home/work" "$D/home/src" "$D/xdg/config/mrm" "$D/xdg/cache/mrm" "$D/xdg/state"
export HOME=$D/home GIT_CONFIG_NOSYSTEM=1
git config --global user.name "Dev User"
git config --global user.email dev@example.com
git config --global init.defaultBranch main
git config --global advice.detachedHead false
git config --global url."$D/remotes/".insteadOf git@github.com:acme/

ago() { date -d "-$1" --iso-8601=seconds; }
commit() { # <dir> <message> <age>
  echo "$2" > "$1/$RANDOM.txt"
  git -C "$1" add -A
  GIT_AUTHOR_DATE=$(ago "$3") GIT_COMMITTER_DATE=$(ago "$3") git -C "$1" commit -qm "$2"
}
mk() { # <name> <clone path> <age> <subject>
  local bare=$D/remotes/$1.git seed=$D/seed-$1
  git init -q --bare "$bare"
  git clone -q "$bare" "$seed" 2>/dev/null
  commit "$seed" "initial commit" "60 days"
  commit "$seed" "$4" "$3"
  git -C "$seed" push -q origin main
  git clone -q "git@github.com:acme/$1.git" "$2"
}
upstream() { # <name> <message> <age>: someone else pushes
  commit "$D/seed-$1" "$2" "$3"
  git -C "$D/seed-$1" push -q origin main
}

W=$D/home/work
mk api               "$W/services/api"            "2 hours"  "fix: rounding in invoice totals"
mk web               "$W/services/web"            "1 day"    "feat: checkout layout"
mk docs              "$W/services/docs"           "5 days"   "docs: deploy guide"
mk payments          "$W/services/payments"       "4 days"   "fix: retry webhooks"
mk billing-api       "$W/platform/billing-api"    "3 days"   "chore: bump dependencies"
mk auth              "$W/platform/auth"           "6 days"   "feat: rotate signing keys"
mk terraform-network "$W/infra/terraform-network" "3 weeks"  "feat: private subnets"
mk dotfiles          "$D/home/src/dotfiles"       "2 weeks"  "tmux: new status line"
mk blog              "$D/home/src/blog"           "9 weeks"  "post: fast-forward only"

# behind: api, billing-api, docs, blog
upstream api "feat: invoice export" "40 minutes"
upstream api "fix: currency codes" "20 minutes"
upstream api "test: export edge cases" "10 minutes"
upstream billing-api "feat: proration" "1 day"
for i in 1 2 3; do upstream billing-api "chore: deps $i" "$i hours"; done
upstream docs "docs: faq" "3 hours"
upstream blog "post: mass repo managers" "1 day"
# web: on a pushed feature branch, two commits ahead, two files modified
git -C "$W/services/web" switch -q -c feature/checkout
commit "$W/services/web" "wip: checkout layout" "1 day"
git -C "$W/services/web" push -q -u origin feature/checkout
commit "$W/services/web" "wip: address form" "5 hours"
commit "$W/services/web" "wip: totals" "4 hours"
for f in $(git -C "$W/services/web" ls-files | head -2); do echo edit >> "$W/services/web/$f"; done
# payments: unpushed commits on main
commit "$W/services/payments" "feat: refunds api" "3 hours"
commit "$W/services/payments" "test: refunds" "2 hours"
# auth: diverged
upstream auth "fix: token expiry" "2 days"
commit "$W/platform/auth" "feat: key cache" "1 day"
# dotfiles: a stash
f=$(git -C "$D/home/src/dotfiles" ls-files | head -1)
echo x >> "$D/home/src/dotfiles/$f"
git -C "$D/home/src/dotfiles" stash -q

for r in "$W"/*/* "$D"/home/src/*; do git -C "$r" fetch -q; done
rm -r "$D"/seed-*

cat > "$D/xdg/config/mrm/config.yaml" <<'YAML'
workspaces:
  work:
    roots: [~/work]
    depth: 3
  personal:
    roots: [~/src]
repos:
  "~/work/infra/terraform-*":
    tags: [terraform]
forge_tokens:
  github.com:
    token_cmd: echo demo-token
forge_cache: 720h
YAML

# Forge answers, seeded into mrm's cache: with a 720h forge_cache nothing
# asks GitHub, and the acme repos don't exist there anyway.
python3 - "$D/xdg/cache/mrm/forge.json" <<'PY'
import datetime, json, sys
now = datetime.datetime.now(datetime.timezone.utc).isoformat()
info = {"api": {"prs": 2, "ci": "passing"}, "payments": {"prs": 1, "ci": "passing"},
        "billing-api": {"prs": 3, "ci": "running"}, "auth": {"prs": 0, "ci": "passing"},
        "web": {"prs": 0, "ci": "failed", "ci_url": "https://github.com/acme/web/actions/runs/1"},
        "docs": {"prs": 0, "ci": "passing"}, "terraform-network": {"prs": 0, "ci": "none"},
        "dotfiles": {"prs": 0, "ci": "none"}, "blog": {"prs": 0, "ci": "none"}}
json.dump({f"github.com/acme/{k}": dict(v, fetched=now) for k, v in info.items()}, open(sys.argv[1], "w"))
PY

if [ "${2:-}" = github ]; then
  git config --global --remove-section url."$D/remotes/"
  for r in "$W"/*/* "$D"/home/src/*; do
    git -C "$r" remote set-url origin "git@github.com:acme/$(basename "$r").git"
  done
fi
