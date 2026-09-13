#!/usr/bin/env bash
# Back up, update, relaunch — the whole of a Hollowmere deploy in one run.
#
# Order matters: the dump happens before anything is fetched, so if a
# migration in the new tree turns out to eat packs, the dump predates it.
# Health is checked after the rebuild, and a world that will not come up
# healthy is rolled back to the commit that was running before.
#
# Run it from the Compose project directory on the live host:
#
#   ./scripts/deploy.sh
#
# Flags:
#   --no-rollback   leave the new tree in place even if health fails
#   --no-pull       rebuild and relaunch what is already checked out
#   --yes           do not ask before restarting the world
set -euo pipefail

# This script is in the tree it updates, and bash reads a script lazily as
# it runs. Pulling a new version mid-run would have bash resume at a byte
# offset into different text, which fails in ways that are hard to read.
# So the first thing we do is run from a copy nothing is going to touch.
if [ -z "${HOLLOWMERE_DEPLOY_COPY:-}" ]; then
  copy="$(mktemp -t hollowmere-deploy.XXXXXX)"
  cat "$0" > "$copy"
  chmod +x "$copy"
  HOLLOWMERE_DEPLOY_COPY="$0" exec "$copy" "$@"
fi
trap 'rm -f "$0"' EXIT
SELF="$HOLLOWMERE_DEPLOY_COPY"

usage() {
  cat >&2 <<'USAGE'
usage: ./scripts/deploy.sh [--no-rollback] [--no-pull] [--yes]

Backs up Postgres, fast-forwards the checkout, rebuilds and relaunches the
Compose stack, then waits for /health. A world that will not come up healthy
is rolled back to the commit that was running before.

  --no-rollback  leave the new tree in place even if health fails
  --no-pull      rebuild and relaunch what is already checked out
  --yes, -y      do not ask before restarting the world

environment:
  HOLLOWMERE_ROOT            checkout to deploy (default: this script's ..)
  HOLLOWMERE_BRANCH          branch to deploy (default: main)
  HOLLOWMERE_REMOTE          remote to fetch (default: origin)
  HOLLOWMERE_HEALTH          local health URL
  HOLLOWMERE_PUBLIC          public health URL
  HOLLOWMERE_KEEP_BACKUPS    dumps to keep (default: 10, 0 keeps all)
  HOLLOWMERE_HEALTH_TIMEOUT  seconds to wait for health (default: 120)
USAGE
}

say()  { printf '\n\033[1m== %s\033[0m\n' "$*" >&2; }
note() { printf '   %s\n' "$*" >&2; }
die()  { printf '\n\033[1;31m!! %s\033[0m\n' "$*" >&2; exit 1; }

compose() {
  if docker compose version >/dev/null 2>&1; then
    docker compose "$@"
  else
    docker-compose "$@"
  fi
}

# Normally the checkout is the directory above this script. The override is
# for the first run, when the script is not in the tree yet.
ROOT="${HOLLOWMERE_ROOT:-$(cd "$(dirname "$SELF")/.." && pwd)}"
cd "$ROOT" || die "no such directory: $ROOT"

BRANCH="${HOLLOWMERE_BRANCH:-main}"
REMOTE="${HOLLOWMERE_REMOTE:-origin}"
HEALTH_URL="${HOLLOWMERE_HEALTH:-http://127.0.0.1:28080/health}"
PUBLIC_URL="${HOLLOWMERE_PUBLIC:-https://2007.gliffy.tv/health}"
KEEP_BACKUPS="${HOLLOWMERE_KEEP_BACKUPS:-10}"
HEALTH_TIMEOUT="${HOLLOWMERE_HEALTH_TIMEOUT:-120}"

ROLLBACK=1
PULL=1
ASSUME_YES=0
for arg in "$@"; do
  case "$arg" in
    --no-rollback) ROLLBACK=0 ;;
    --no-pull) PULL=0 ;;
    --yes|-y) ASSUME_YES=1 ;;
    --help|-h) usage; exit 0 ;;
    *) echo "unknown argument: $arg" >&2; exit 2 ;;
  esac
done


# ---------------------------------------------------------------- preflight
say "Preflight"
[ -f docker-compose.yml ] || die "no docker-compose.yml in $ROOT"
git rev-parse --git-dir >/dev/null 2>&1 || die "$ROOT is not a git checkout"
docker info >/dev/null 2>&1 || die "cannot talk to docker (is the daemon up, and are you in the docker group?)"

PREV="$(git rev-parse HEAD)"
PREV_SHORT="$(git rev-parse --short HEAD)"
note "checkout   $ROOT"
note "running    $PREV_SHORT $(git log -1 --format=%s)"

# Host-local edits are normal here — the compose file carries this box's
# port mapping. They are stashed across the pull and put back after, and
# anything that does not go back cleanly stops the deploy.
DIRTY=0
if ! git diff --quiet || ! git diff --cached --quiet; then
  DIRTY=1
  note "local edits to: $(git diff --name-only HEAD | tr '\n' ' ')"
fi

if [ "$PULL" = 1 ]; then
  git fetch --quiet "$REMOTE" "$BRANCH" || die "could not fetch $REMOTE/$BRANCH"
  BEHIND="$(git rev-list --count "HEAD..$REMOTE/$BRANCH")"
  AHEAD="$(git rev-list --count "$REMOTE/$BRANCH..HEAD")"
  if [ "$AHEAD" != 0 ]; then
    die "this checkout has $AHEAD commit(s) $REMOTE/$BRANCH does not. Sort that out by hand."
  fi
  if [ "$BEHIND" = 0 ]; then
    note "already at $REMOTE/$BRANCH — rebuilding anyway"
  else
    note "$BEHIND commit(s) to apply:"
    git --no-pager log --oneline --reverse "HEAD..$REMOTE/$BRANCH" | sed 's/^/     /' >&2
  fi
fi

if [ "$ASSUME_YES" = 0 ]; then
  [ -t 0 ] || die "not a terminal — pass --yes if you mean to deploy unattended"
  printf '\n   This restarts the world. Everyone online is disconnected for the rebuild.\n   Continue? [y/N] ' >&2
  read -r reply
  case "$reply" in y|Y|yes|YES) ;; *) die "nothing done" ;; esac
fi

# ------------------------------------------------------------------- backup
# Before the fetch, so the dump is of the tree that is actually running.
say "Backup"
[ -x ./scripts/pg-backup.sh ] || die "scripts/pg-backup.sh is missing or not executable"
DUMP="$(./scripts/pg-backup.sh)" || die "backup failed — nothing has been changed"
note "dump $DUMP"

if [ "$KEEP_BACKUPS" -gt 0 ]; then
  # Newest first, skip the ones we keep, delete the rest. The -latest
  # symlink is not in the list, so it is never a candidate.
  find backups -maxdepth 1 -name 'hollowmere-*.sql.gz' -type f -printf '%T@ %p\n' 2>/dev/null \
    | sort -rn | tail -n "+$((KEEP_BACKUPS + 1))" | cut -d' ' -f2- \
    | while read -r old; do note "pruning $old"; rm -f "$old"; done
fi

# ------------------------------------------------------------------- update
STASHED=0
if [ "$PULL" = 1 ]; then
  say "Update"
  if [ "$DIRTY" = 1 ]; then
    git stash push --quiet --message "hollowmere-deploy $(date -u +%Y%m%dT%H%M%SZ)" \
      || die "could not stash local edits"
    STASHED=1
    note "stashed local edits"
  fi

  if ! git merge --ff-only --quiet "$REMOTE/$BRANCH"; then
    [ "$STASHED" = 1 ] && git stash pop --quiet || true
    die "$BRANCH would not fast-forward. Look at the history before deploying."
  fi

  if [ "$STASHED" = 1 ]; then
    if ! git stash pop --quiet; then
      die "local edits did not reapply cleanly. The stash is still there:
     git status          # see the conflict
     git stash list      # your edits are the top entry
   The new code is checked out but NOT built, so the running world is untouched."
    fi
    note "local edits reapplied"
  fi
  note "now at $(git rev-parse --short HEAD) $(git log -1 --format=%s)"
fi

# ------------------------------------------------------------------ relaunch
say "Relaunch"
compose up --build -d || die "build or start failed. The old container may still be running — check 'docker compose ps'."

# ------------------------------------------------------------------- health
say "Health"
healthy() {
  local body
  body="$(curl -sf --max-time 5 "$HEALTH_URL" 2>/dev/null)" || return 1
  case "$body" in *'"ok":true'*) echo "$body"; return 0 ;; esac
  return 1
}

BODY=""
for _ in $(seq 1 "$HEALTH_TIMEOUT"); do
  if BODY="$(healthy)"; then break; fi
  sleep 1
done

if [ -n "$BODY" ]; then
  note "local  $BODY"
  if curl -sf --max-time 8 "$PUBLIC_URL" >/dev/null 2>&1; then
    note "public $PUBLIC_URL ok"
  else
    note "public $PUBLIC_URL did NOT answer — the world is up, so look at Caddy"
  fi
  say "Deployed $(git rev-parse --short HEAD)"
  note "was      $PREV_SHORT"
  note "dump     $DUMP"
  note "Hard-refresh the site (Ctrl+Shift+R) so a cached app.js does not linger."
  exit 0
fi

# ---------------------------------------------------------------- it failed
printf '\n\033[1;31m!! %s did not come up healthy in %ss\033[0m\n' "$HEALTH_URL" "$HEALTH_TIMEOUT" >&2
compose logs --tail 50 world >&2 || true

if [ "$ROLLBACK" = 0 ] || [ "$PULL" = 0 ]; then
  die "left as is (--no-rollback). To go back by hand:
     git checkout $PREV_SHORT && docker compose up --build -d"
fi

say "Rolling back to $PREV_SHORT"
if ! git checkout --quiet "$PREV"; then
  die "could not check out $PREV_SHORT. Do it by hand, then: docker compose up --build -d"
fi
compose up --build -d || die "rollback build failed. The hamlet is down. Dump: $DUMP"

for _ in $(seq 1 "$HEALTH_TIMEOUT"); do
  if BODY="$(healthy)"; then break; fi
  sleep 1
done

if [ -n "$BODY" ]; then
  note "back on $PREV_SHORT and healthy: $BODY"
  note "you are on a detached HEAD — 'git checkout $BRANCH' when the fix is in"
  die "deploy failed, rollback succeeded. Nothing was restored from $DUMP; the data is as the new build left it."
fi

die "rollback is also unhealthy. The hamlet is down.
   Data as of before the deploy: $DUMP
   To put it back:  docker compose stop world
                    ./scripts/pg-restore.sh $DUMP --yes
                    docker compose start world"
