# hatrack

[![ci](https://github.com/baodq97/hatrack/actions/workflows/ci.yml/badge.svg)](https://github.com/baodq97/hatrack/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/baodq97/hatrack)](https://github.com/baodq97/hatrack/releases/latest)
[![license](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

Switch the account Claude Code is signed in to, without signing out and back in, and manage the MCP
servers it loads: databases, Azure DevOps, GitHub and more.

If you have more than one Claude subscription (personal and work, or two plans), hatrack keeps each
login and swaps the one Claude uses with one click in the Windows tray, or one command on Linux/WSL.
It works for the `claude` CLI and the Claude Code extension for VS Code, since both read the same login.

- **Windows:** a tray app (click the hat, pick an account, switch MCP servers on and off) plus the `hat` CLI.
- **Linux and WSL:** the `hat` CLI.
- **MCP servers:** add them from a catalog (PostgreSQL, MySQL, SQL Server, MongoDB, Redis, Azure DevOps,
  GitHub, Jira…), switch them off without losing their settings, group them into sets that follow
  your account, and test that they start. See [MCP servers](#mcp-servers).

```
$ hat
* personal@gmail.com             personal@gmail.com  personal@gmail.com's Organization
  work@company.com               work@company.com    Company
$ hat use work@company.com
now using work@company.com. If an open Claude session still shows the old account, restart it (VS Code: Reload Window).
```

## Contents

- [Install](#install)
- [Getting started](#getting-started)
- [Commands](#commands)
- [Windows tray app](#windows-tray-app)
- [Adding an account](#adding-an-account)
- [MCP servers](#mcp-servers)
- [VS Code](#vs-code)
- [WSL and Windows](#wsl-and-windows)
- [How it works](#how-it-works)
- [Security and privacy](#security-and-privacy)
- [Limits](#limits)
- [Troubleshooting](#troubleshooting)
- [Uninstall](#uninstall)
- [Build from source](#build-from-source)
- [Disclaimer](#disclaimer)
- [License](#license)

## Install

### Windows

In PowerShell:

```powershell
irm https://raw.githubusercontent.com/baodq97/hatrack/main/install.ps1 | iex
```

This installs `hat.exe` and `hatrack-tray.exe` to `%LOCALAPPDATA%\hatrack`, adds that folder to your
user `PATH`, starts the tray app, and adds it to `shell:startup` so it runs when you sign in to Windows.
Nothing needs admin rights.

Prefer to look first? Download `hat-windows-amd64.exe` and `hatrack-tray-windows-amd64.exe` from the
[latest release](https://github.com/baodq97/hatrack/releases/latest), rename them to `hat.exe` and
`hatrack-tray.exe`, and keep them in the same folder. Read [install.ps1](install.ps1) to see exactly what
the script does.

### Linux and WSL

```sh
curl -fsSL https://raw.githubusercontent.com/baodq97/hatrack/main/install.sh | sh
```

This puts `hat` in `~/.local/bin` (override with `HAT_INSTALL_DIR`). Builds exist for amd64 and arm64.

### Verify a download

Every release lists SHA-256 sums in `checksums.txt`, and each binary has a signed build provenance
attestation that proves it was built by this repository's release workflow:

```sh
gh attestation verify hat-linux-amd64 --repo baodq97/hatrack
```

### Update

Run the install command again. On Windows it stops the running tray app first, since Windows locks a
running exe.

## Getting started

1. Sign in to Claude Code as usual, if you have not already.
2. Keep that login: `hat save` (tray: **Save current account**).
3. Add each other account: `hat add` (tray: **Add account…**). See [Adding an account](#adding-an-account).
4. Switch: `hat use <name>` (tray: click the account).

Profile names default to the account's email. Pass a name to pick your own: `hat save personal`.

## Commands

| Command | What it does |
|---|---|
| `hat` | List saved accounts. `*` marks the one Claude uses now. |
| `hat use <name>` | Switch Claude to that account. |
| `hat add [name]` | Sign in another account. Your current login is not touched. |
| `hat save [name]` | Save the account Claude is signed in to now. |
| `hat rm <name>` | Forget a saved account. Does not sign anything out. |
| `hat mcp …` | Manage MCP servers. See [MCP servers](#mcp-servers). |
| `hat version` | Print the version. |

## Windows tray app

Click the hat icon in the notification area. Windows 11 puts new icons behind the `^` overflow
arrow at first; drag it onto the taskbar to keep it visible.

```
✓ personal@gmail.com  (personal@gmail.com's Organization)
  work@company.com  (Company)
─────────────
Add account…
Save current account
Remove ▸
─────────────
MCP servers ▸
─────────────
Quit
```

- **Click an account** to switch to it. The check mark shows the active one; hover the icon to see it too.
- **Add account…** opens a console that walks you through signing in another account.
- **Save current account** keeps the login Claude has now.
- **Remove** forgets a saved account.
- **MCP servers** lists your user-scope MCP servers: click one to switch it on or off. Your
  [sets](#sets-mcp-servers-that-follow-your-account) are listed below them; click one to use it.
- If an action fails, the error shows at the top of the menu. The app also logs to `%USERPROFILE%\.hatrack\tray.log`.

The menu refreshes every few seconds, so changes made with `hat` show up on their own. Quitting the
tray app changes nothing: Claude keeps using the last account you picked.

## Adding an account

`hat add` (or **Add account…** in the tray) asks how you want to sign in:

```
Add a Claude account

Sign-in happens in a separate folder; the account you use now stays signed in.

  [1] Open the sign-in page in my default browser
  [2] Show me the link - I'll open it in the right browser profile myself

Choose 1 or 2:
```

- **[1]** runs `claude auth login` as usual. Pick this if your default browser is signed in to
  claude.ai with the account you want to add.
- **[2]** prints the sign-in link instead of opening a browser. Open it in the browser profile (or a
  private window) that is signed in to the account you want, approve, then paste the code it shows
  back into the console. On Windows the code shows as `********` and its last four characters.

Do **not** add accounts with Claude's `/login`, `claude auth login`, or the Sign in button in VS Code
while another account is active: those replace the current login without saving it, so you would
have to sign that account in again. If it happens anyway, `hat save` right after keeps the new one.

## MCP servers

`hat mcp` manages the MCP servers Claude Code loads, in all three places Claude keeps them:

| Scope | Where | Loaded |
|---|---|---|
| `user` (default) | `mcpServers` in `~/.claude.json` | in every folder |
| `local` | `projects[<folder>].mcpServers` in `~/.claude.json` | in the current folder, for you only |
| `project` | `.mcp.json` in the current folder | in the current folder, for everyone who checks it out |

Pick a scope with `-s user|local|project`. When two scopes have a server of the same name, Claude uses
the narrowest (local, then project, then user); `hat mcp` shows the others as `hidden`.

```
$ hat mcp
SCOPE    NAME          STATE  TYPE   TARGET
user     azure-devops  on     stdio  npx -y @azure-devops/mcp contoso
user     github        on     http   https://api.githubcopilot.com/mcp/
user     postgres      off    stdio  uvx postgres-mcp --access-mode=restricted
project  team-db       on     stdio  npx -y @bytebase/dbhub --transport stdio --dsn sqlserver://app:****@db:1433/app
```

Restart Claude (VS Code: **Developer: Reload Window**) after a change; Claude reads its MCP servers
when it starts.

### Commands

| Command | What it does |
|---|---|
| `hat mcp` | List servers for the current folder. Secrets are hidden. |
| `hat mcp catalog` | List ready-made servers. |
| `hat mcp add <name> --from <template>` | Add one from the catalog. Asks for what it needs (connection string, organization…), or take them with `--var NAME=value`. |
| `hat mcp add <name> -- <command> [args…]` | Add a local (stdio) server. `-e KEY=value` sets its environment. |
| `hat mcp add <name> --url <url>` | Add a remote server. `--transport sse` for older servers, `-H "Key: value"` for headers. |
| `hat mcp show <name>` | Print its config, secrets hidden. `--reveal` shows them. |
| `hat mcp off <name>` / `on <name>` | Switch a server off and back on. Its config is kept. |
| `hat mcp rm <name>` | Delete a server. |
| `hat mcp test [name]` | Start servers the way Claude would, run the MCP handshake and list their tools. |
| `hat mcp doctor` | Find servers that cannot start (missing `npx`/`uvx`, unfilled values, unset `${VARS}`) without starting them. |
| `hat mcp export [-o file]` | Write a scope's servers to a `.mcp.json`-style file, to back them up or move them to another machine. |
| `hat mcp import <file>` | Add servers from such a file. `--desktop` imports Claude Desktop's servers. |
| `hat mcp set …` | Groups of servers. See below. |

`add` also takes `--test`, to test the server right away, and `--force`, to replace a server of the
same name.

### Catalog

| Category | Templates |
|---|---|
| database | `postgres`, `mysql`, `mssql` (SQL Server, Azure SQL), `sqlserver` (DBHub: SQL Server, PostgreSQL, MySQL, MariaDB, SQLite), `mongodb`, `redis`, `sqlite` |
| devops | `azure-devops`, `azure`, `github`, `atlassian` (Jira, Confluence), `linear`, `sentry` |
| docs | `context7`, `notion` |
| tools | `playwright`, `filesystem`, `fetch`, `memory` |

```
$ hat mcp add ado --from azure-devops
Azure DevOps needs a few values (or pass them with --var NAME=value):
  Organization, e.g. contoso for dev.azure.com/contoso: contoso
added user-scope MCP server ado: npx -y @azure-devops/mcp contoso

$ hat mcp add prod-db -s local --from postgres
PostgreSQL needs a few values (or pass them with --var NAME=value):
  Connection string, e.g. postgresql://user:pass@localhost:5432/db: ********
added local-scope MCP server prod-db: uvx postgres-mcp --access-mode=restricted
```

Database templates are read-only where the server supports it. `mssql` is not: it can run any SQL,
so give it a login that can only read (`db_datareader`). It signs in with a SQL login only, not
Windows authentication. Each server is published by its own
maintainer (see the link `add` prints) and runs with your permissions; hatrack only writes the config
Claude starts it with. Servers started with `npx` need [Node.js](https://nodejs.org), those with `uvx`
need [uv](https://docs.astral.sh/uv/). On Windows, `npx` is started through `cmd /c`, as Claude needs.

**Keep secrets out of the config:** type `${NAME}` instead of a value (`--var 'GITHUB_TOKEN=${GITHUB_TOKEN}'`)
and Claude fills it from its environment when it starts the server. `hat mcp doctor` warns when such a
variable is not set.

**Your team's own templates** go in `~/.hatrack/mcp/catalog.json`. They show in `hat mcp catalog` and
replace a built-in template of the same name:

```json
[
  {
    "name": "erp-db",
    "title": "ERP database (read-only replica)",
    "category": "database",
    "description": "Our ERP SQL Server replica",
    "server": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@bytebase/dbhub", "--transport", "stdio", "--dsn", "sqlserver://{{DB_USER}}:{{DB_PASS}}@erp-replica:1433/erp"]
    },
    "vars": [
      { "name": "DB_USER", "prompt": "Your database user" },
      { "name": "DB_PASS", "prompt": "Password", "secret": true }
    ]
  }
]
```

`{{NAME}}` marks a value asked for when adding; give a `default` to offer one.

### Sets: MCP servers that follow your account

A set is a named group of user-scope servers. Using it switches its servers on and every other
user-scope server off, so Claude only loads what that work needs.

```sh
hat mcp set save work ado erp-db       # or switch servers on/off first and run: hat mcp set save work
hat mcp set save personal github
hat mcp set use personal
hat mcp set link work me@company.com   # hat use me@company.com now also uses set work
hat mcp set link personal me@gmail.com
```

With sets linked, `hat use` (or a click in the tray) switches the account and its MCP servers together.

### How switching off works

Claude has no off switch for user and local servers, so `hat mcp off` moves the server's config to
`~/.hatrack/mcp/disabled.json` and `on` moves it back unchanged. A project server stays in `.mcp.json`,
which belongs to the team; it is switched off the way Claude itself does it, through
`disabledMcpjsonServers` in `~/.claude.json`. As with logins, only the keys being changed are touched,
and every write goes through a temporary file.

## VS Code

The Claude Code extension and the CLI share one login, so `hat use` switches both. A conversation
that is already open may keep the old account until you run **Developer: Reload Window**.

- **Remote - WSL / SSH / containers:** the extension runs on the remote side and uses that side's
  login. Switch with `hat` inside WSL (or on the remote), not with the Windows tray.
- **`CLAUDE_CONFIG_DIR` in `claudeCode.environmentVariables`:** the extension then reads that folder.
  `hat` only sees `CLAUDE_CONFIG_DIR` from its own environment, so set it there too.

## WSL and Windows

Windows and each WSL distro have their own home folder, so each has its own Claude login and its own
saved accounts. Set up accounts on each side you use; switching on one side does not affect the other.

## How it works

A Claude Code login is two things:

- `~/.claude/.credentials.json`: the OAuth tokens.
- the `oauthAccount` block in `~/.claude.json`: who the tokens belong to.

(Both live under `$CLAUDE_CONFIG_DIR` if you set it.)

hatrack keeps a copy of both for each account in `~/.hatrack/claude/<name>/` and swaps them in.
Everything else, like settings, history, projects, MCP servers and plugins, is shared between accounts
and not touched by switching (MCP servers change only through `hat mcp`). Only the `oauthAccount` key of `~/.claude.json` changes; every other key is written
back as it was.

- **Switching** first writes the current login back to its saved copy, because Claude refreshes tokens
  while you work and an old copy would stop working. A login that was never saved is saved under its
  email instead of being lost.
- **Adding** runs `claude auth login` with `CLAUDE_CONFIG_DIR` pointing at a scratch folder, saves the
  result, and deletes the folder. Your active login is never replaced.
- **One copy per account:** saving the same account under a second name is refused, since only one
  copy can stay current.
- Every write goes to a temporary file first and is renamed into place, so a crash never leaves half a
  login file.

## Security and privacy

- hatrack makes **no network requests** of its own. Tokens never leave your machine; only Claude itself
  talks to Anthropic. The one exception is `hat mcp test` on a remote (http) MCP server, which you run
  on purpose: it contacts that server, with the headers you configured, as Claude would. Testing a
  local server runs its command, which may download it (`npx`, `uvx`).
- MCP server configs (connection strings, tokens) stay where Claude keeps them; switched-off ones go to
  `~/.hatrack/mcp/disabled.json`, readable only by you. `hat mcp` and `show` hide secrets unless you
  pass `--reveal`; `export` writes them in full.
- Saved logins are plain files, like Claude's own `.credentials.json`, readable only by your user
  (mode `0600` on Linux; your user profile's permissions on Windows). Anyone who can read your home
  folder can read them, just as with Claude's own login.
- hatrack only reads and writes Claude's login files and `~/.hatrack`. It does not touch Claude's
  traffic, telemetry or anything else.
- No telemetry, no accounts, no server.

Anthropic's terms do not allow sharing one subscription between people. hatrack is for switching
between accounts that are yours. Found a security problem? See [SECURITY.md](SECURITY.md).

## Limits

- **Claude Code only** for now. Codex and other tools may come later. `hat mcp import --desktop` reads
  Claude Desktop's servers but does not change them.
- The tray app shows user-scope MCP servers only; manage local and project ones with `hat mcp` in the folder.
- `hat mcp test` speaks stdio and streamable HTTP; servers on the older `sse` transport show their state in Claude's `/mcp`.
- **No macOS:** Claude keeps its login in the Keychain there, which hatrack does not handle.
- **Open sessions** may keep using the previous account until restarted.
- **Saved logins are not refreshed** while not in use. One left unused long enough expires; add it again with `hat add`.
- Claude Code may change how it stores logins, which can break hatrack until it is updated.

## Troubleshooting

| Problem | Fix |
|---|---|
| `claude is not signed in` | Sign in to Claude Code first, then `hat save`. |
| `profile "x" already holds …` | That name belongs to another account. Pick another name. |
| `… is already saved as "x"` | That account is saved under another name. Use that one, or `hat rm` it first. |
| A switched account says it is logged out | Its saved tokens expired while unused, or another tool signed it out. `hat add` it again. |
| VS Code still shows the old account | Run **Developer: Reload Window**. |
| No tray icon | Check the `^` overflow arrow. If the app is not running, start `hatrack-tray.exe` and check `%USERPROFILE%\.hatrack\tray.log`. |
| An MCP server does not show up in Claude | Restart Claude. Then `hat mcp doctor`, and `hat mcp test <name>` to see the server's own error. |
| `hat mcp test` says the server asks to sign in | It uses OAuth: sign in from Claude with `/mcp`. Or check the token in its headers. |
| `hat` not found right after installing on Windows | Open a new terminal so it picks up the updated `PATH`. |
| Windows SmartScreen warns about the exe | The binaries are not code-signed. Verify them as shown in [Verify a download](#verify-a-download). |

## Uninstall

hatrack never changes Claude's login on its own, so after uninstalling, Claude keeps using whichever
account was active.

**Windows:** quit the tray app, then in PowerShell:

```powershell
Remove-Item "$([Environment]::GetFolderPath('Startup'))\hatrack.lnk", "$env:LOCALAPPDATA\hatrack" -Recurse
Remove-Item "$HOME\.hatrack" -Recurse   # your saved logins, switched-off MCP servers and sets
```

and remove `%LOCALAPPDATA%\hatrack` from your user `PATH` (Settings → System → About → Advanced system
settings → Environment Variables).

**Linux/WSL:**

```sh
rm ~/.local/bin/hat
rm -r ~/.hatrack   # your saved logins, switched-off MCP servers and sets
```

## Build from source

Needs Go (see `go.mod` for the version). No C compiler is needed.

```sh
go test ./...
go build ./cmd/hat                                                         # CLI for this OS
GOOS=windows go build -ldflags -H=windowsgui -o hatrack-tray.exe ./cmd/hatrack-tray  # tray, from any OS
```

Layout:

```
cmd/hat/              CLI
cmd/hatrack-tray/     Windows tray app
internal/claude/      reading, saving and swapping Claude logins
internal/mcp/         MCP servers: scopes, on/off, catalog, sets, test
internal/atomicfile/  crash-safe file writes
```

Releases are built by [.github/workflows/release.yml](.github/workflows/release.yml) when a `v*` tag is
pushed. See [CONTRIBUTING.md](CONTRIBUTING.md).

## Disclaimer

hatrack is an independent, unofficial project. It is **not affiliated with, endorsed by, or supported
by Anthropic**. "Claude", "Claude Code" and related names are trademarks of Anthropic, PBC, used here
only to say what hatrack works with.

hatrack works by reading and rewriting Claude Code's local login files. Their format is not a public
interface and can change without notice. The software is provided **"as is", without warranty of any
kind**, as set out in sections 7 and 8 of the [license](LICENSE). In particular:

- **Use it at your own risk.** A bug, or a change in Claude Code, could sign you out or make a saved
  login unusable. The worst case should be signing in again, but back up `~/.claude/.credentials.json`
  and `~/.claude.json` if losing them would hurt.
- **You are responsible for how you use your accounts.** Using hatrack must comply with
  [Anthropic's terms](https://www.anthropic.com/legal) and your organization's policies. Do not use it
  to share a subscription between people or to get around usage limits or other restrictions.
- The authors are not liable for lost access, suspended accounts, lost work, or any other damage
  arising from using hatrack.

## License

Copyright 2026 baodq97. Licensed under the [Apache License, Version 2.0](LICENSE).
