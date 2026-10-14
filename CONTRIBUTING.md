# Contributing

Issues and pull requests are welcome.

## Before you open a pull request

```sh
gofmt -l .        # prints nothing
go vet ./...
go test ./...
GOOS=windows go vet ./...   # the tray app only builds for Windows
GOOS=darwin go vet ./cmd/hat ./internal/claude ./internal/gui ./internal/mcp ./internal/usage ./internal/notify   # the menu bar app needs a Mac
```

CI runs the same on Linux, Windows and macOS. On a Mac, `HATRACK_KEYCHAIN_TEST=1 go test ./...` also
round-trips a throwaway item through the real login keychain.

## Ground rules

- hatrack handles login tokens and MCP server secrets. Changes must not send anything over the network,
  log them, or write them anywhere but Claude's own files and `~/.hatrack`. The one exception is
  `hat mcp test`, which contacts the MCP server the user asked to test, as Claude would.
- The user stays in control: no hidden steps. If hatrack does something on the user's behalf, it says so.
- Logic that touches login files goes in `internal/claude`, MCP config logic in `internal/mcp`, usage
  and limits in `internal/usage`; each comes with a test. Menu items both trays show go in
  `internal/traymenu`, so the Windows build checks them for the Mac too.
- Keep changes small and dependencies few.

## Releasing

Maintainers tag a version and push it; the release workflow tests, builds, attests and publishes:

```sh
git tag v0.1.0 && git push origin v0.1.0
```
