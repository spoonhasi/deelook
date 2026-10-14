#!/usr/bin/env bash
# Builds the frontend (web/dist), embeds it into deelook, and puts each
# target in its own folder under bin/ together with the FANUC FOCAS2
# library it loads for sampling, when that is in fwlib/ (it is not in the
# repository: see fwlib/README.md), and a deelook.toml to edit. Copy a
# folder and run: deelook keeps its settings (deelook.toml) and data
# (deelook-data/) in that folder.
# Needs Node (npm) and Go on PATH.
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
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch GOARM=7 go build -trimpath -ldflags "-s -w" -o "$exe" .
  # the settings file, all defaults commented out; never over one in use
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
