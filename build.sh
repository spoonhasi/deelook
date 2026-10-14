#!/usr/bin/env bash
# Builds the frontend (web/dist), embeds it into deelook, and puts each
# target in its own folder under bin/ together with the FANUC FOCAS2
# library it loads for sampling, when that is in fwlib/ (it is not in the
# repository: see fwlib/README.md), and a deelook.toml to edit. Copy a
# folder and run: deelook keeps its settings (deelook.toml) and data
# (deelook-data/) in that folder.
# Needs Node (npm) and Go on PATH (Go fetches go-winres the first time).
#
# The Windows executable carries version information and a manifest
# (winres/winres.json; the version is the date of the commit) and keeps
# its symbols: antivirus heuristics take a stripped Go executable without
# them for malware more readily.
#
# Sampling (cnc_sdt*) is required, so only targets with a FOCAS2 library
# that has it are built:
#
#   windows-amd64   Fwlib64.dll 7.3 and fwlibe64.dll (Ethernet)
#   linux-386       32-bit (also on 64-bit Linux with 32-bit libc, libstdc++)
#   linux-arm       32-bit ARM
set -euo pipefail
cd "$(dirname "$0")"

if [ ! -d web/node_modules ]; then
  (cd web && npm ci)
fi
(cd web && npm run build)

# rsrc_windows_amd64.syso: linked into the windows/amd64 build only
version="$(git log -1 --format=%cd --date=format:%Y.%m.%d 2>/dev/null || echo 0.0.0).0"
trap 'rm -f rsrc_windows_*.syso' EXIT
go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --arch amd64 --out rsrc \
  --product-version "$version" --file-version "$version"

mkdir -p bin
for target in windows/amd64 linux/386 linux/arm; do
  os=${target%/*}
  arch=${target#*/}
  dir="bin/deelook-$os-$arch"
  # replace only what the build puts there: a folder in use keeps its
  # deelook.toml and deelook-data/
  mkdir -p "$dir"
  rm -f "$dir/deelook" "$dir/deelook.exe" "$dir"/*.dll "$dir"/libfwlib32.so*
  exe="$dir/deelook"
  [ "$os" = windows ] && exe="$exe.exe"
  ldflags="-s -w"
  [ "$os" = windows ] && ldflags="" # symbols kept (see above)
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch GOARM=7 go build -trimpath -ldflags "$ldflags" -o "$exe" .
  # the settings file with the default values; never over one in use
  [ -e "$dir/deelook.toml" ] || cp deelook.example.toml "$dir/deelook.toml"
  case $target in
    windows/amd64) libs=(fwlib/windows-x64/Fwlib64.dll fwlib/windows-x64/fwlibe64.dll) ;;
    linux/386) libs=(fwlib/linux-x86/libfwlib32.so*) ;;
    linux/arm) libs=(fwlib/linux-arm/libfwlib32.so*) ;;
  esac
  if [ -e "${libs[0]}" ]; then
    cp "${libs[@]}" "$dir/"
    echo "built $dir"
  else
    echo "built $dir (no FOCAS2 library in ${libs[0]%/*}: put it next to the executable, see fwlib/README.md)"
  fi
done
