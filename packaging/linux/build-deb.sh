#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Kivora Digital S.L.
#
# build-deb.sh assembles a .deb package containing the wspace CLI, using
# only `dpkg-deb` (part of every Debian/Ubuntu base install — no new
# dependency).
#
# Linux ships the CLI only. The freedesktop icons under packaging/linux/icons/ are kept as assets but are
# not installed by this package, since a CLI has no desktop entry.
#
# This is the "at least one distro package definition as a build target"
# deliverable (design.md §11). An .rpm equivalent is not included: one
# distro package format is the stated minimum.
#
# The CLI binary must already exist; it is a pure-Go CGO_ENABLED=0 build,
# so `make dist` on any host produces it. dpkg-deb itself is only
# available on Debian/Ubuntu (or a Linux CI runner).
#
# Usage: packaging/linux/build-deb.sh <version> <arch> <cli-bin>
#   <arch> is a Debian architecture name (amd64, arm64).
set -eu

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
version=${1:?"usage: build-deb.sh <version> <arch> <cli-bin>"}
arch=${2:?"usage: build-deb.sh <version> <arch> <cli-bin>"}
cli_bin=${3:?"usage: build-deb.sh <version> <arch> <cli-bin>"}

if ! command -v dpkg-deb >/dev/null 2>&1; then
    echo "build-deb.sh: requires dpkg-deb (Debian/Ubuntu); not available on this machine" >&2
    exit 1
fi
if [ ! -x "$cli_bin" ]; then
    echo "build-deb.sh: $cli_bin not found or not executable" >&2
    exit 1
fi

pkg_root="$root_dir/build/linux/ws_${version}_${arch}"
rm -rf "$pkg_root"
mkdir -p "$pkg_root/DEBIAN" "$pkg_root/usr/bin"

cp "$cli_bin" "$pkg_root/usr/bin/wspace"

sed -e "s/__VERSION__/$version/g" -e "s/__ARCH__/$arch/g" \
    "$root_dir/packaging/linux/deb/control.tmpl" >"$pkg_root/DEBIAN/control"

out="$root_dir/dist/ws_${version}_${arch}.deb"
mkdir -p "$root_dir/dist"
dpkg-deb --build --root-owner-group "$pkg_root" "$out"

echo "built $out"
