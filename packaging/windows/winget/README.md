# winget manifest templates

These three files follow the [winget-pkgs manifest
schema](https://github.com/microsoft/winget-pkgs) (version, installer,
locale). They are **templates**, not a submittable manifest: every
`__PLACEHOLDER__` must be filled in with real values before opening a PR
against `microsoft/winget-pkgs`, and none of this is wired into CI —
winget submission is a manual, one-time-per-release step, not something
this repository can automate on its own.

## Before using these

1. Confirm the repository coordinates (`kivoradigital/wspace`) —
   `PackageIdentifier` and every URL below depend on them. `License` is
   already set to `Apache-2.0`.
2. Build `packaging/windows/wspace.iss` into a real signed installer (see that
   script's own doc comment on signing) and publish it as a GitHub Release
   asset.
3. Compute the installer's SHA256 (`certutil -hashfile wspace-setup-X.Y.Z.exe
   SHA256` on Windows, or `shasum -a 256` elsewhere) and fill in
   `InstallerSha256`.
4. Run `winget validate` and `winget install --manifest .` locally against
   the filled-in manifest before submitting.

## Files

| File | Purpose |
|---|---|
| `version.yaml.tmpl` | Declares the package identifier and version. |
| `installer.yaml.tmpl` | Declares the installer URL, SHA256, and switches. |
| `locale.yaml.tmpl` | Declares the package's display metadata (name, description, license, URLs). |
