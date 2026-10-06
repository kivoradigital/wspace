# Manual release checklist

Everything below is out of automated scope on purpose (design.md §12's own
"Manual only" row: installers, shell-init in a real shell, PATH/rc
edits). Every platform ships the CLI only. `go test
./...`, `make test-e2e`, and `golangci-lint run ./...` passing green does
not mean any of this is done — this checklist is what actually confirms a
release is ready, and it must be run by a human on each real target
platform.

## Before the first tagged release ever

- [ ] Confirm the module path (`github.com/kivoradigital/wspace`) and the
      update-check coordinates (`REPO_OWNER`/`REPO_NAME` in the `Makefile`
      and `.goreleaser.yaml`) match the public repository.
- [ ] Remove `skip_upload` from the publishers in `.goreleaser.yaml` once
      the repository and the tap/bucket/winget repositories are public.
- [ ] Confirm `AppId` in `packaging/windows/wspace.iss` is intentional (it is
      a fixed GUID chosen for this project; do not regenerate it per
      release — Inno Setup uses it to recognize upgrades of the *same*
      product).
- [ ] Icon assets are listed in `packaging/icon/README.md`.

## Every release

- [ ] **Windows**: compile `packaging/windows/wspace.iss` with a real `iscc`
      on Windows and confirm:
  - [ ] the installer runs to completion without an admin prompt
        (`PrivilegesRequired=lowest`)
  - [ ] a **new** shell (opened after install, not one already open) can
        run `wspace` with no PATH edit (packaging-distribution
        spec: "Installer completes without manual PATH edit")
  - [ ] uninstalling removes the PATH entry (no dangling `{app}` entry
        left behind)
  - [ ] SmartScreen's unsigned-installer warning appears as expected — see
        `wspace.iss`'s own header comment for what real signing would require
- [ ] **Linux**: on at least one real distro,
  - [ ] install via `.deb` (`sudo dpkg -i ...`) or the manual steps in
        `packaging/linux/README.md`
  - [ ] confirm `wspace` is on PATH and `wspace version` reports the tag
- [ ] **CLI, every platform**: `wspace install` places the binary and offers
      shell-rc integration; `wspace shell-init {bash,zsh,sh,fish}` sourced in
      a real shell makes `wspace jump`'s `cd` actually work (design.md §12).
- [ ] Confirm the release build ran green on the tag push and every
      artifact (CLI archives, checksums, Linux `.deb`/`.rpm`, Windows
      installer when built) is attached to the GitHub Release.

## Optional, cheap to skip a given release

- [ ] Submit/update the `winget` manifest (`packaging/windows/winget/`)
      against `microsoft/winget-pkgs` once a real installer URL and SHA256
      exist.
