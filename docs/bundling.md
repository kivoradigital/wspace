# Bundling wspace in a desktop app

A desktop app that embeds the `wspace` CLI builds it itself and ships it
inside its own bundle. Such a copy is updated only together with the app, so
it must never suggest updating itself on its own. The app declares this at
build time.

## Build

```sh
make build BUNDLED_BY="App Name" VERSION=<cli version>
```

`BUNDLED_BY` is the app's display name; it may contain spaces. Check the
values a build would embed with `make release-vars BUNDLED_BY="App Name"`.
Package-manager builds leave `BUNDLED_BY` empty, and nothing changes for them.

## What changes in a bundled build

- `wspace version` prints `wspace version <version>` on the first line,
  exactly as in any other build, so scripts that parse it keep working. A
  second line reads `(bundled with App Name)`.
- `wspace version --check` makes no network request. It prints
  `wspace <version> is bundled with App Name and updates with it`.
- `engine.checkUpdate` ([RPC](rpc-contract.md)) and the `check_update` MCP
  tool ([MCP](mcp.md)) make no network request either. They return
  `bundledBy: "App Name"` with `available` and `unavailable` both `false`.
- The agent skill tells AI agents never to suggest updating a bundled CLI
  separately.

## Responsibilities of the bundling app

The bundling app owns updates of the CLI it embeds: it picks the CLI
version, rebuilds it, and ships it with each app release. Users should not
replace the bundled binary with a separately installed one.
