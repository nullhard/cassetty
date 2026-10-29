#!/bin/bash
# Builds the release archives: cassetty for macOS and Linux on amd64 and
# arm64, each packed with README.md, LICENSE and NOTICE as
# cassetty_<os>_<arch>.tar.gz, plus checksums.txt. The names carry no
# version, so releases/latest/download/<name> stays a stable link for the
# website and the install one-liner.
#
# Go's linker signs every macOS binary as "a.out", the name macOS shows for
# cassetty in Control Center's Now Playing, so the macOS binaries are
# re-signed (ad hoc, like the linker does) as "cassetty". That needs Apple's
# codesign, so this runs on macOS.
#
# Run by the Release workflow; works locally too.
#
# usage: scripts/release-build.sh <version> <out dir>
set -euo pipefail

if [ $# -ne 2 ]; then
	echo "usage: $0 <version> <out dir>" >&2
	exit 2
fi
if ! command -v codesign >/dev/null; then
	echo "$0: codesign not found; the macOS binaries can only be signed on macOS" >&2
	exit 1
fi
version=$1
mkdir -p "$2"
out=$(cd "$2" && pwd)
repo=$(cd "$(dirname "$0")/.." && pwd)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

cd "$repo"
for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do
	os=${target%/*}
	arch=${target#*/}
	name=cassetty_${os}_${arch}
	echo "building $name"
	mkdir -p "$stage/$name"
	# No cgo: audio goes through purego, so every target cross-compiles.
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
		-ldflags "-s -w -X main.version=$version" -o "$stage/$name/cassetty" .
	if [ "$os" = darwin ]; then
		codesign --force --sign - --identifier cassetty "$stage/$name/cassetty"
		codesign --verify --strict "$stage/$name/cassetty"
	fi
	cp README.md LICENSE NOTICE "$stage/$name/"
	COPYFILE_DISABLE=1 tar -czf "$out/$name.tar.gz" -C "$stage/$name" cassetty README.md LICENSE NOTICE
done

cd "$out"
shasum -a 256 -- cassetty_*.tar.gz >checksums.txt
cat checksums.txt
