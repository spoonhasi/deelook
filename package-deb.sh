#!/usr/bin/env bash
# Makes the Debian package of deelook for x86 Linux PCs,
# dist/deelook_<VERSION>_i386.deb (packaging/linux/nfpm.yaml says what is
# in it), from build.sh's bin/deelook-linux-386. Needs what build.sh needs;
# Go fetches nfpm the first time. Install it on the PC with
#
#   sudo dpkg --add-architecture i386 && sudo apt update   (64-bit Linux, once)
#   sudo apt install ./deelook_<VERSION>_i386.deb
set -euo pipefail
cd "$(dirname "$0")"

./build.sh
version="$(tr -d ' \r\n' < VERSION)"

rm -rf dist/stage
mkdir -p dist/stage
cp bin/deelook-linux-386/deelook dist/stage/deelook
# the settings with their defaults for the service: the data folder in
# /var/lib/deelook, no browser
sed -e '1s|.*|# deelook 설정 파일 (/etc/deelook/deelook.toml): 고친 뒤 sudo systemctl restart deelook|' \
  -e 's|^data = .*|data = "/var/lib/deelook"|' \
  -e 's|^open = .*|open = false|' \
  deelook.example.toml >dist/stage/deelook.toml.default
grep -q '^data = "/var/lib/deelook"$' dist/stage/deelook.toml.default
grep -q '^open = false$' dist/stage/deelook.toml.default

# nfpm 2.46 is the last that builds with Go 1.25
VERSION="$version" go run github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.46.3 \
  pkg --config packaging/linux/nfpm.yaml --packager deb --target dist/
rm -rf dist/stage
