# Linux packaging

Linux ships the **CLI only**; there is no `.desktop` entry.

## What ships

- `wspace`: a plain, statically linked (CGO_ENABLED=0) binary; no installer
  required to run it.
- `build-deb.sh` + `deb/control.tmpl`: assembles a `.deb` containing the
  CLI (the "at least one distro package definition" deliverable).

`icons/hicolor/**` holds freedesktop icons rendered from `tray.svg`. They are kept
as assets (see `../icon/README.md`) but no package installs them.

## Manual install (no package manager)

```sh
cp wspace /usr/local/bin/     # or any writable, PATH-listed directory
```

## `.deb` install

```sh
sudo dpkg -i ws_<version>_<arch>.deb
```
