#!/usr/bin/env bash
# Publishes a version of deelook for Linux PCs: builds the package, adds it
# to the apt repository (the gh-pages branch, which GitHub Pages serves at
# https://spoonhasi.github.io/deelook/), tags the commit v<version> and
# pushes main, gh-pages and the tag together. PCs with deelook installed
# then get it with apt update && apt upgrade.
#
#   ./release.sh 0.1.1     VERSION becomes 0.1.1 (committed), then publish
#   ./release.sh           publish the version in VERSION
#   ./release.sh -n 0.1.1  try it: everything but the commits and the push,
#                          the repository built in dist/apt-try
#
# Runs on the PC with the apt signing key (packaging/linux/apt-repo.sh), from
# Git Bash with WSL on Windows (DEELOOK_WSL names the distribution), and
# needs what build.sh needs. A list of strings that must never be published
# (one per line, kept outside the repository) is checked against the
# program when it is there: DEELOOK_FORBIDDEN, by default
# ~/.deelook-apt/forbidden.txt.
set -euo pipefail
cd "$(dirname "$0")"

die() {
  echo "release: $*" >&2
  exit 1
}

dry=false
if [ "${1:-}" = -n ]; then
  dry=true
  shift
fi
want=${1:-}
[ -z "$want" ] || [[ $want =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "version $want: want x.y.z"

# on Windows the apt tools are in WSL
run_linux() { # script and its arguments, all paths
  if command -v wsl.exe >/dev/null && [ "$(uname -o 2>/dev/null)" = Msys ]; then
    local args=() a
    for a in "$@"; do
      a=$(cd "$(dirname "$a")" && pwd -W)/$(basename "$a")
      args+=("/mnt/$(echo "${a:0:1}" | tr A-Z a-z)${a:2}")
    done
    MSYS_NO_PATHCONV=1 wsl.exe -d "${DEELOOK_WSL:-Ubuntu-24.04}" -- bash "${args[@]}"
  else
    bash "$@"
  fi
}

# a release is of main as pushed, plus at most the version change
[ "$(git rev-parse --abbrev-ref HEAD)" = main ] || die "not on main"
if [ -n "$(git status --porcelain)" ]; then
  $dry || die "uncommitted changes: commit them first"
  echo "release: uncommitted changes (trying only)"
fi
git fetch -q origin main gh-pages
git merge-base --is-ancestor origin/main HEAD || die "origin/main has commits this main has not: pull first"

old=$(tr -d ' \r\n' <VERSION)
if [ -n "$want" ] && [ "$want" != "$old" ]; then
  printf '%s\n' "$want" >VERSION
  if $dry; then
    trap 'printf "%s\n" "$old" >VERSION' EXIT
  else
    git commit -q -m "deelook $want" -- VERSION
  fi
fi
version=$(tr -d ' \r\n' <VERSION)
tag=v$version
deb=deelook_${version}_i386.deb

if git rev-parse -q --verify "refs/tags/$tag" >/dev/null || git ls-remote --exit-code -q --tags origin "$tag"; then
  die "$tag exists: give a new version"
fi

# the apt repository: gh-pages checked out in dist/apt
pages=dist/apt
if [ ! -e "$pages/.git" ]; then
  git worktree add -q "$pages" gh-pages
fi
[ -z "$(git -C "$pages" status --porcelain)" ] || die "$pages has uncommitted changes"
git -C "$pages" merge -q --ff-only origin/gh-pages
pool=$pages/pool/main/d/deelook
newest=$(find "$pool" -name 'deelook_*_i386.deb' 2>/dev/null | sed 's|.*/deelook_||; s|_i386\.deb$||' | sort -V | tail -1)
if [ -n "$newest" ]; then
  if [ "$newest" = "$version" ] || [ "$(printf '%s\n%s\n' "$newest" "$version" | sort -V | tail -1)" != "$version" ]; then
    die "$version is not newer than $newest, the newest in the repository"
  fi
fi

./package-deb.sh

# nothing that must stay private (apt-repo.sh refuses FANUC's libraries)
forbidden=${DEELOOK_FORBIDDEN:-$HOME/.deelook-apt/forbidden.txt}
if [ -s "$forbidden" ]; then
  if grep -aqF -f <(grep -v '^[[:space:]]*$' "$forbidden") bin/deelook-linux-386/deelook deelook.example.toml; then
    die "the package would hold a string from $forbidden"
  fi
else
  echo "release: no $forbidden, the program is not checked for private strings"
fi

if $dry; then
  rm -rf dist/apt-try
  mkdir -p dist/apt-try
  (cd "$pages" && tar --exclude=.git -cf - .) | (cd dist/apt-try && tar -xf -)
  pages=dist/apt-try
  pool=$pages/pool/main/d/deelook
fi
mkdir -p "$pool"
cp "dist/$deb" "$pool/"
run_linux packaging/linux/apt-repo.sh "$pages"

if $dry; then
  echo "release: tried $version: the repository would be $pages (nothing committed or pushed)"
  exit 0
fi

git -C "$pages" add -A
git -C "$pages" commit -q -m "deelook $version"
git tag -a "$tag" -m "deelook $version"
if ! git push -q --atomic origin main gh-pages "refs/tags/$tag"; then
  # nothing went out: as before, but for the version commit, to run again
  git tag -d "$tag" >/dev/null
  git -C "$pages" reset -q --hard origin/gh-pages
  die "push failed"
fi
echo "release: $version pushed ($tag); waiting for GitHub Pages"

# done when Pages serves the new Release
url=https://spoonhasi.github.io/deelook/dists/stable/InRelease
sum=$(sha256sum "$pages/dists/stable/InRelease" | cut -d' ' -f1)
for i in $(seq 1 60); do
  if [ "$(curl -fs "$url?release=$version-$i" | sha256sum | cut -d' ' -f1)" = "$sum" ]; then
    echo "release: deelook $version is on https://spoonhasi.github.io/deelook/ (apt update && apt upgrade)"
    exit 0
  fi
  sleep 5
done
echo "release: Pages does not serve it yet after 5 minutes: see the repository's Actions (pages build)" >&2
exit 1
