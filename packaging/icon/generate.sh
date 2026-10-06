#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Kivora Digital S.L.
#
# generate.sh renders every packaging icon format from the two SVG masters
# in this directory, so the artwork has a single source and no binary is
# ever hand-edited:
#
#   wspace.svg                     -> the coloured application tile
#   tray.svg                       -> the flat menu-bar glyph
#
# Outputs (all committed, because a release build must not depend on an
# image toolchain being installed):
#
#   ../windows/wspace.ico                                 Windows installer + exe
#   ../linux/icons/hicolor/<size>/apps/wspace-tray.png    freedesktop raster
#   ../linux/icons/hicolor/scalable/apps/wspace-tray.svg  freedesktop scalable
#
# Requires ImageMagick (`magick`).
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
packaging="$(cd "$here/.." && pwd)"
app_svg="$here/wspace.svg"
tray_svg="$here/tray.svg"

command -v magick >/dev/null || { echo "magick (ImageMagick) is required" >&2; exit 1; }

render() { # render <svg> <size> <out>
  magick -background none "$1" -resize "${2}x${2}" "PNG32:$3"
}

# --- Windows .ico ----------------------------------------------------------
tmp="$(mktemp -d)"
for size in 16 24 32 48 64 128 256; do
  render "$app_svg" "$size" "$tmp/$size.png"
done
magick "$tmp"/16.png "$tmp"/24.png "$tmp"/32.png "$tmp"/48.png \
       "$tmp"/64.png "$tmp"/128.png "$tmp"/256.png "$packaging/windows/wspace.ico"
rm -rf "$tmp"
echo "wrote $packaging/windows/wspace.ico"

# --- Linux freedesktop icons ----------------------------------------------
# The tray glyph, not the tile: a notification-area icon sits on the panel's
# own background the same way a menu-bar icon does.
for size in 16 22 24 32 48 64 128 256; do
  dir="$packaging/linux/icons/hicolor/${size}x${size}/apps"
  mkdir -p "$dir"
  render "$tray_svg" "$size" "$dir/wspace-tray.png"
done
cp "$tray_svg" "$packaging/linux/icons/hicolor/scalable/apps/wspace-tray.svg"
echo "wrote $packaging/linux/icons/hicolor/*/apps/wspace-tray.*"
