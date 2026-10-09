wspace - test build
===================

Each archive holds a single wspace binary. Install it for your user (no
administrator rights needed), then connect it to your AI coding agent.

Requirements: git 2.20 or newer on your PATH (check with `git --version`).

Pick the archive for your system and CPU:

  macOS, Apple Silicon (M1 and later)   wspace_<version>_darwin_arm64.tar.gz
  macOS, Intel                          wspace_<version>_darwin_amd64.tar.gz
  Linux, Intel/AMD                      wspace_<version>_linux_amd64.tar.gz
  Linux, ARM                            wspace_<version>_linux_arm64.tar.gz
  Windows, Intel/AMD                    wspace_<version>_windows_amd64.zip
  Windows, ARM (e.g. Snapdragon)        wspace_<version>_windows_arm64.zip

Not sure? macOS: Apple menu > About This Mac ("Chip: Apple M..." is Apple
Silicon). Linux: `uname -m` (x86_64 = amd64, aarch64 = arm64). Windows:
Settings > System > About > System type.

Verify the download against checksums.txt:
  macOS:   shasum -a 256 -c checksums.txt --ignore-missing
  Linux:   sha256sum -c checksums.txt --ignore-missing
  Windows: Get-FileHash .\wspace_<version>_windows_<arch>.zip -Algorithm SHA256
           (compare with the matching line in checksums.txt)

Every archive also has a signed build provenance attestation. With the
GitHub CLI installed you can check where it was built:
  gh attestation verify <archive> -R kivoradigital/wspace


macOS and Linux
---------------

  1. Open a terminal in the folder with the archive and run:

       tar -xzf wspace_<version>_<os>_<arch>.tar.gz
       chmod +x ./wspace

     macOS only: the binary is not notarized by Apple yet, so macOS
     blocks it ("cannot be opened because the developer cannot be
     verified"). Clear the download mark first:

       xattr -c ./wspace

  2. Install it:

       ./wspace install --yes

     This copies wspace to ~/.local/bin and, when that folder is not on
     your PATH yet, adds it in your shell's startup file (~/.zshrc for
     zsh, the default shell on macOS; ~/.bashrc and your login file for
     bash; config.fish for fish).

  3. Open a NEW terminal and check:

       wspace version

  4. Connect Claude Code (skill and MCP server):

       wspace agents install --agent claude-code --mcp

     Use --agent codex or --agent gemini for those agents. Add --json for
     machine-readable output.


Windows (PowerShell)
--------------------

  1. Extract the zip, open PowerShell in the folder with wspace.exe and
     run:

       Unblock-File .\wspace.exe
       .\wspace.exe install --yes

     The binary is not code-signed yet; Unblock-File clears the
     "downloaded from the internet" mark so SmartScreen doesn't block it.

     This copies wspace.exe to %LOCALAPPDATA%\Programs\wspace and adds
     that folder to your user Path (only your user, not the system).

  2. Open a NEW terminal and check:

       wspace version

  3. Connect Claude Code (skill and MCP server):

       wspace agents install --agent claude-code --mcp

     If registering the MCP server fails, the output ends with the exact
     command to run yourself; paste it into PowerShell.

  Note: on Windows the skill is copied (symbolic links need Developer
  Mode), so after updating wspace run step 3 again to refresh the copy.


Updating
--------

`wspace version --check` tells you when a newer release exists. Install
the new binary the same way (macOS/Linux steps 1-2, Windows step 1). On
Windows the previous copy is renamed to wspace.old.exe and deleted on the
next run.

Releases are also published at https://github.com/kivoradigital/wspace/releases
and through Homebrew (macOS) and Scoop (Windows); see the project README.


Getting started
---------------

  wspace --help
  wspace doctor              check git and the setup
  wspace context create      create a context (where projects and workspaces live)
  wspace create <name>       create a workspace (--copy-node-modules for Node projects)
  wspace list / wspace status

Configuration lives in ~/.config/wspace (macOS and Linux) or
%APPDATA%\wspace (Windows). Set WSPACE_CONFIG_HOME to test with a separate
folder.


Known limitations of this test build
------------------------------------

- The macOS binary is not notarized and the Windows binary is not
  code-signed (see the install steps above).


Uninstalling
------------

  wspace agents uninstall
  wspace install --uninstall

This removes the binary, the PATH entry wspace added and its shell
integration; nothing else in your startup files or Path is touched.
