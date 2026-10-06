wspace - test build
===================

This archive holds a single wspace binary. Install it for your user (no
administrator rights needed), then connect it to your AI coding agent.

Requirements: git 2.20 or newer on your PATH (check with `git --version`).

Pick the archive for your CPU: amd64 (Intel/AMD) or arm64 (ARM, e.g.
Windows on Snapdragon or a Linux ARM machine).

Verify the download against SHA256SUMS.txt:
  Linux:   sha256sum -c SHA256SUMS.txt --ignore-missing
  Windows: Get-FileHash .\wspace-<version>-windows-<arch>.zip -Algorithm SHA256


Linux and macOS
---------------

  1. Open a terminal in the folder with the binary and run:

       chmod +x ./wspace
       ./wspace install --yes

     This copies wspace to ~/.local/bin and, when that folder is not on
     your PATH yet, adds it in your shell's startup file (~/.bashrc and
     your login file for bash, ~/.zshrc for zsh, config.fish for fish).

  2. Open a NEW terminal and check:

       wspace version

  3. Connect Claude Code (skill and MCP server):

       wspace agents install --agent claude-code --mcp

     Use --agent codex or --agent gemini for those agents. Add --json for
     machine-readable output.


Windows (PowerShell)
--------------------

  1. Open PowerShell in the folder with wspace.exe and run:

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

Run the new binary's install the same way (step 1). On Windows the
previous copy is renamed to wspace.old.exe and deleted on the next run.


Getting started
---------------

  wspace --help
  wspace doctor              check git and the setup
  wspace context create      create a context (where projects and workspaces live)
  wspace create <name>       create a workspace (--copy-node-modules for Node projects)
  wspace list / wspace status

Configuration lives in ~/.config/wspace (Linux) or %APPDATA%\wspace
(Windows). Set WSPACE_CONFIG_HOME to test with a separate folder.


Known limitations of this test build
------------------------------------

- The update notice in `wspace version` cannot reach the release feed
  yet, so it never reports a newer version.
- Binaries are not code-signed.


Uninstalling
------------

  wspace agents uninstall
  wspace install --uninstall

This removes the binary, the PATH entry wspace added and its shell
integration; nothing else in your startup files or Path is touched.
