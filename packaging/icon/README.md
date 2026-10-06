# Icon assets

Every icon asset in this directory tree is kept on purpose. They are the
source artwork for wspace packaging and for desktop clients built on top of
the CLI. Do not delete them.

| Asset | What it is |
|---|---|
| `packaging/icon/wspace.svg` | Master of the coloured application tile. |
| `packaging/icon/tray.svg` | Master of the flat menu-bar / notification-area glyph. |
| `packaging/icon/generate.sh` | Renders every raster format below from the two masters (needs ImageMagick). |
| `packaging/windows/wspace.ico` | Windows icon, rendered from `wspace.svg`. |
| `packaging/linux/icons/hicolor/<size>/apps/wspace-tray.png` | freedesktop raster icons (16, 22, 24, 32, 48, 64, 128, 256), rendered from `tray.svg`. |
| `packaging/linux/icons/hicolor/scalable/apps/wspace-tray.svg` | freedesktop scalable icon, a copy of `tray.svg`. |

Linux and Windows ship the CLI only, so no current package installs the
freedesktop icons.
