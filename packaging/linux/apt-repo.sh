#!/usr/bin/env bash
# Rebuilds the apt repository in DIR (the gh-pages branch, which GitHub
# Pages serves at https://spoonhasi.github.io/deelook/) from the packages in
# DIR/pool, and signs it with the deelook apt key. Runs on Debian or Ubuntu
# (apt-utils, gpg); release.sh runs it in WSL.
#
#   packaging/linux/apt-repo.sh DIR
#
# The key is in GNUPGHOME (default ~/.gnupg-deelook-apt); its public half is
# deelook-archive-keyring.asc next to this script, which the package
# installs for apt (deelook.sources).
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
dir=$1
export GNUPGHOME=${GNUPGHOME:-$HOME/.gnupg-deelook-apt}
pub="$here/deelook-archive-keyring.asc"
fpr=$(gpg --show-keys --with-colons "$pub" | awk -F: '/^fpr/ {print $10; exit}')

cd "$dir"
# FANUC's FOCAS2 libraries are not ours to publish (fwlib/README.md)
for deb in $(find pool -name '*.deb'); do
  if dpkg-deb -c "$deb" | awk '{print $NF}' | grep -qE '\.(so|dll)(\.|$)'; then
    echo "apt-repo: $deb holds a shared library (FOCAS2?): not published" >&2
    exit 1
  fi
done

dist=dists/stable
bin=$dist/main/binary-i386
mkdir -p "$bin/by-hash/SHA256"
rm -f "$bin/Packages" "$bin/Packages.gz" "$dist/Release" "$dist/InRelease" "$dist/Release.gpg"
apt-ftparchive --arch i386 packages pool >"$bin/Packages"
gzip -9nk "$bin/Packages"
# the indexes also by hash (Acquire-By-Hash): a Release cached by Pages
# still finds the indexes it names after the next release
for f in "$bin/Packages" "$bin/Packages.gz"; do
  cp "$f" "$bin/by-hash/SHA256/$(sha256sum "$f" | cut -d' ' -f1)"
done

release=$(mktemp)
# SHA256 only, as Debian's: apt asks by-hash for the strongest hash listed
apt-ftparchive \
  -o APT::FTPArchive::Release::MD5=false \
  -o APT::FTPArchive::Release::SHA1=false \
  -o APT::FTPArchive::Release::SHA512=false \
  -o APT::FTPArchive::Release::Origin=deelook \
  -o APT::FTPArchive::Release::Label=deelook \
  -o APT::FTPArchive::Release::Suite=stable \
  -o APT::FTPArchive::Release::Codename=stable \
  -o APT::FTPArchive::Release::Architectures=i386 \
  -o APT::FTPArchive::Release::Components=main \
  -o APT::FTPArchive::Release::Acquire-By-Hash=yes \
  -o "APT::FTPArchive::Release::Description=deelook (https://github.com/spoonhasi/deelook)" \
  release "$dist" >"$release"
mv "$release" "$dist/Release"
chmod 644 "$dist/Release"
gpg --batch --yes --local-user "$fpr" --digest-algo SHA512 --clearsign -o "$dist/InRelease" "$dist/Release"
gpg --batch --yes --local-user "$fpr" --digest-algo SHA512 --armor --detach-sign -o "$dist/Release.gpg" "$dist/Release"
cp "$pub" deelook-archive-keyring.asc
touch .nojekyll # files served as they are
# and kept as they are (the signatures cover them): no CRLF on Windows
echo '* -text' >.gitattributes

# the page: how to install the newest package
deb=$(find pool -name 'deelook_*_i386.deb' | sort -V | tail -1)
version=${deb##*/deelook_}
version=${version%_i386.deb}
sed -e "s|@VERSION@|$version|g" -e "s|@DEB@|$deb|g" -e "s|@DEBNAME@|${deb##*/}|g" \
  -e "s|@FINGERPRINT@|$(echo "$fpr" | sed 's/..../& /g; s/ $//')|g" "$here/index.html" >index.html

# what apt will do: check the signature against the public key alone
keyring=$(mktemp)
gpg --dearmor <"$pub" >"$keyring"
gpgv --keyring "$keyring" "$dist/InRelease" 2>/dev/null
gpgv --keyring "$keyring" "$dist/Release.gpg" "$dist/Release" 2>/dev/null
rm -f "$keyring"
echo "apt repository in $dir: $(grep -c '^Package:' "$bin/Packages") packages, signed by $fpr"
