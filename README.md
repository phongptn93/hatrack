# hatrack

[![ci](https://github.com/baodq97/hatrack/actions/workflows/ci.yml/badge.svg)](https://github.com/baodq97/hatrack/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/baodq97/hatrack)](https://github.com/baodq97/hatrack/releases/latest)
[![license](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

A toolkit for Claude Code: switch the account it is signed in to without signing out, manage the MCP
servers it loads (databases, Azure DevOps, GitHub and more), see the tokens you use per account, and
keep its setup healthy.

If you have more than one Claude subscription (personal and work, or two plans), hatrack keeps each
login and swaps the one Claude uses with one click in the Windows tray, or one command on macOS,
Linux and WSL.
It works for the `claude` CLI and the Claude Code extension for VS Code, since both read the same login.

- **Windows:** a tray app (click the hat, pick an account, switch MCP servers on and off) plus the `hat` CLI.
- **macOS:** a menu bar app with a dashboard window, plus the `hat` CLI.
- **Linux and WSL:** the `hat` CLI.
- **MCP servers:** add them from a catalog (PostgreSQL, MySQL, SQL Server, MongoDB, Redis, Azure DevOps,
  GitHub, Jira…), switch them off without losing their settings, group them into sets that follow
  your account, and test that they start. See [MCP servers](#mcp-servers).
- **Usage:** tokens by day, project, model and account, and the 5-hour window your limits count in.
  See [Usage](#usage).
- **Upkeep:** `hat doctor` checks the whole setup; `hat clean` forgets deleted folders Claude still
  remembers. See [Doctor and clean](#doctor-and-clean).

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
- [macOS menu bar app](#macos-menu-bar-app)
- [Adding an account](#adding-an-account)
- [MCP servers](#mcp-servers)
- [Usage](#usage)
- [Doctor and clean](#doctor-and-clean)
- [VS Code](#vs-code)
- [WSL and Windows](#wsl-and-windows)
- [macOS](#macos)
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

### macOS, Linux and WSL

```sh
curl -fsSL https://raw.githubusercontent.com/baodq97/hatrack/main/install.sh | sh
```

This puts `hat` in `~/.local/bin` (override with `HAT_INSTALL_DIR`). Builds exist for amd64 and arm64
(Apple silicon and Intel Macs).

### macOS menu bar app

Download `Hatrack-macos.zip` from the [latest release](https://github.com/baodq97/hatrack/releases/latest),
unzip it and move `Hatrack.app` to Applications. It runs on Apple silicon and Intel Macs (macOS 11
or later). The app is not notarized by Apple, so macOS blocks the first launch. Either run

```sh
xattr -d com.apple.quarantine /Applications/Hatrack.app
```

or open it once, then go to System Settings → Privacy & Security and click **Open Anyway**. (On
macOS 14 and earlier, right-click the app and choose **Open** also works.)

Or build it yourself (needs Go and the Xcode command line tools): `./build-mac-app.sh`.

The `hat` CLI from the command above works alongside the app; both use the same saved accounts.

### Verify a download

Every release lists SHA-256 sums in `checksums.txt`, and each binary has a signed build provenance
attestation that proves it was built by this repository's release workflow:

```sh
gh attestation verify hat-linux-amd64 --repo baodq97/hatrack
```

The macOS app ships as `Hatrack-macos.zip`, with its own `Hatrack-macos.zip.sha256` and attestation.

### Update

Run the install command again. On Windows it stops the running tray app first, since Windows locks a
running exe. For the macOS app, quit Hatrack from the menu bar and replace `Hatrack.app` with the one
from the new release.

## Getting started

1. Sign in to Claude Code as usual, if you have not already.
2. Keep that login: `hat save` (tray: **Save current account**).
3. Add each other account: `hat add` (tray: **Add account…**). See [Adding an account](#adding-an-account).
4. Switch: `hat use <name>` (tray: click the account).

Profile names default to the account's email. Pass a name to pick your own: `hat save personal`.

## Commands

| Command | What it does |
|---|---|
| `hat` | List saved accounts and their plan. `*` marks the one Claude uses now. |
| `hat use <name>` | Switch Claude to that account. |
| `hat add [name]` | Sign in another account. Your current login is not touched. |
| `hat save [name]` | Save the account Claude is signed in to now. |
| `hat rm <name>` | Forget a saved account. Does not sign anything out. |
| `hat mcp …` | Manage MCP servers. See [MCP servers](#mcp-servers). |
| `hat usage` | Tokens used. See [Usage](#usage). |
| `hat doctor` | Check Claude Code's setup. See [Doctor and clean](#doctor-and-clean). |
| `hat clean` | Forget deleted folders Claude still remembers. |
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
Today: 15.5M tokens · window resets 21:00
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

## macOS menu bar app

Hatrack.app puts a hat in the menu bar, with no Dock icon:

- **Click an account** to switch to it. The check mark shows the active one.
- **Open Full Dashboard** opens a window to switch, save, rename and remove accounts. It opens as an
  app window if Chrome is installed, otherwise in your default browser.
- **Add Account…** opens Terminal and runs `hat add` there, so you can pick the browser to sign in
  with and paste the code if asked. The new account shows up in the menu when sign-in finishes.
- **Save Current Account** keeps the login Claude has now.
- **Today: … tokens** shows today's usage and when the current 5-hour window resets (see [Usage](#usage)).
- **MCP servers** lists your user-scope MCP servers: click one to switch it on or off, or pick a
  [set](#sets-mcp-servers-that-follow-your-account). A set linked to an account follows it here too.

The dashboard is served only on `127.0.0.1`. Each launch makes a random token that only the window
Hatrack opens receives, and every action must carry it, so other web pages cannot switch or remove
accounts through it. If the dashboard says it has expired, open it again from the menu bar. To start
Hatrack when you log in, add it in System Settings → General → Login Items.

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
user     azure-devops  on     http   https://mcp.dev.azure.com/contoso
user     github        on     http   https://api.githubcopilot.com/mcp/
user     postgres      off    stdio  npx -y @toolbox-sdk/server --prebuilt postgres --stdio
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
| `hat mcp doctor` | Find servers that cannot start (missing `npx`/`uvx`, unfilled values, unset `${VARS}`, retired `sse` endpoints) without starting them. |
| `hat mcp export [-o file]` | Write a scope's servers to a `.mcp.json`-style file, to back them up or move them to another machine. |
| `hat mcp import <file>` | Add servers from such a file. `--desktop` imports Claude Desktop's servers. |
| `hat mcp set …` | Groups of servers. See below. |

`add` also takes `--test`, to test the server right away, and `--force`, to replace a server of the
same name.

### Catalog

Each template points at a server that is published by its vendor where one exists, still
maintained, and was started and listed with `hat mcp test` when the template was written; database
templates were also run against a real database. `hat mcp catalog` shows who publishes each one.

| Template | Server | Publisher | Notes |
|---|---|---|---|
| `postgres` | [MCP Toolbox](https://mcp-toolbox.dev) `--prebuilt postgres` | Google | 29 tools: queries, tables, query plans, locks, health |
| `mssql` | [mssql-mcp](https://github.com/BYMCS/mssql-mcp) | community | SQL Server and Azure SQL, SQL login only (no Windows authentication) |
| `mysql` | [mcp-server-mysql](https://github.com/benborla/mcp-server-mysql) | community | read-only unless writes are allowed |
| `sqlite` | MCP Toolbox `--prebuilt sqlite` | Google | |
| `mongodb` | [mongodb-mcp-server](https://github.com/mongodb-js/mongodb-mcp-server) `--readOnly` | MongoDB | |
| `redis` | [mcp-redis](https://github.com/redis/mcp-redis) | Redis | needs uv |
| `dbhub` | [DBHub](https://github.com/bytebase/dbhub) | Bytebase | any SQL database from one DSN; URL-encode special characters in the password |
| `azure-devops` | [Remote MCP Server](https://learn.microsoft.com/azure/devops/mcp-server/remote-mcp-server) | Microsoft | hosted, the one Microsoft recommends; sign in with `/mcp` in Claude |
| `azure-devops-local` | [azure-devops-mcp](https://github.com/microsoft/azure-devops-mcp) | Microsoft | runs locally; Linux and WSL need `libsecret` (`sudo apt install libsecret-1-0`) |
| `azure` | [Azure MCP](https://github.com/microsoft/mcp) | Microsoft | uses your Azure CLI login |
| `github` | [GitHub MCP](https://github.com/github/github-mcp-server) | GitHub | hosted; takes a personal access token |
| `atlassian` | [Atlassian MCP](https://github.com/atlassian/atlassian-mcp-server) `/v2/mcp` | Atlassian | Jira and Confluence; sign in with `/mcp` |
| `linear`, `sentry`, `notion` | the vendor's hosted server | vendor | sign in with `/mcp` |
| `context7` | hosted | Upstash | library docs |
| `playwright` | [Playwright MCP](https://github.com/microsoft/playwright-mcp) | Microsoft | |
| `filesystem`, `fetch`, `memory` | [reference servers](https://github.com/modelcontextprotocol/servers) | MCP project | `fetch` needs uv |

```
$ hat mcp add ado --from azure-devops
Azure DevOps needs a few values (or pass them with --var NAME=value):
  Organization, e.g. contoso for dev.azure.com/contoso: contoso
added user-scope MCP server ado: https://mcp.dev.azure.com/contoso

$ hat mcp add erp -s local --from mssql
SQL Server needs a few values (or pass them with --var NAME=value):
  Server, e.g. localhost or myserver.database.windows.net: sql01
  Port [1433]:
  Database: ERP
  SQL login (best a read-only one): app_ro
  Password: ********
  Trust a self-signed server certificate? true/false [false]: true
added local-scope MCP server erp: npx -y mssql-mcp@latest
```

**Give database servers a read-only login.** Only `mysql` and `mongodb` refuse writes themselves;
the others run whatever SQL Claude sends, within what the login may do (`db_datareader` on SQL
Server, `GRANT SELECT` on PostgreSQL). Servers that connect when they start (`postgres`, `sqlite`,
`dbhub`) hold up Claude's start while their database is unreachable: switch them off with
`hat mcp off` when you are away from it.

Each server is published by its own maintainer (see the link `add` prints) and runs with your
permissions; hatrack only writes the config Claude starts it with. Templates follow the latest
release of each server, so run `hat mcp test` after adding one. Servers started with `npx` need
[Node.js](https://nodejs.org), those with `uvx` need [uv](https://docs.astral.sh/uv/). On Windows, `npx`
is started through `cmd /c`, as Claude needs.

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

## Usage

Claude Code records the tokens of every response in its transcripts. `hat usage` adds them up:

```
$ hat usage
  DAY             RESPONSES  INPUT  OUTPUT  CACHE WRITE  CACHE READ   TOTAL
  2026-10-12 Mon        212    904  310.2K       1.02M      38.11M  39.44M
  2026-10-13 Tue         84    168  139.5K      216.2K      15.18M  15.53M
  TOTAL                 296   1072  449.7K       1.24M      53.29M  54.97M

5-hour window 16:00–21:00 (ends in 3h15m) for work@company.com: 15.53M tokens, 139.5K output, 84 responses
```

- `--days 30` looks further back; `--by project|model|account|session` groups differently.
- **By account:** hat records when each account becomes active (`hat use`, `hat save`, the tray) in
  `~/.hatrack/switches.jsonl`, and counts each response for the account in use at the time. Usage from
  before hat recorded anything shows as `(before hat)`.
- **5-hour window:** subscription limits are counted in 5-hour windows. hat shows the open window of
  each account and when it ends, estimated from your responses the way the limits reset.
- It reads only this machine's transcripts, so work on other machines is not counted, and Claude
  deletes transcripts after `cleanupPeriodDays` (30 by default). The limits themselves weigh tokens in
  ways Anthropic does not publish, so hat shows tokens, not a percentage of your limit.

The tray menu shows today's tokens and when the current window resets.

## Doctor and clean

`hat doctor` checks everything hat knows about, and changes nothing:

```
$ hat doctor
✓ Claude Code 2.1.296 (Claude Code)
✓ signed in as work@company.com (saved as work@company.com, max)
✓ 2 saved accounts
✓ ~/.claude.json: 1.2 MB, 48 folders remembered
✓ ~/.claude/settings.json is valid; Claude keeps transcripts 30 days
✓ transcripts: 412.0 MB in ~/.claude/projects
! 9 deleted folders still remembered: `hat clean` shows them
✓ MCP: 4 servers on, 2 off, no problems found
```

It flags a login hat has not saved, login files other users can read, a broken `settings.json` (which
Claude then ignores silently), a `~/.claude.json` large enough to slow Claude down, and MCP servers
that cannot start.

Claude remembers every folder it ran in, with its trust settings, history and local MCP servers, in
`~/.claude.json`, and never forgets them. `hat clean` lists the ones that were deleted:

```sh
hat clean                          # show them; changes nothing
hat clean --apply                  # forget them in ~/.claude.json
hat clean --apply --transcripts    # and delete their transcripts too
```

A folder counts as deleted only when its parent folder still exists, so a project on an unplugged
drive or a disconnected share is left alone.

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

## macOS

On macOS Claude Code keeps its tokens in the login **Keychain**, not in a file, and hatrack follows
the same order Claude does:

1. **Keychain first.** Claude reads the generic password `Claude Code-credentials` (account: your
   user name). If you set `CLAUDE_CONFIG_DIR`, the item is `Claude Code-credentials-<8 hex>` instead,
   where the suffix is the start of the SHA-256 of that folder path.
2. **`~/.claude/.credentials.json` only as a fallback**, when there is no Keychain item (for example
   if the Keychain was locked or unavailable when Claude signed in).

So on a Mac:

- An old `.credentials.json` next to a Keychain item is ignored by Claude, and by hatrack too: it
  saves and switches the Keychain item. Do not treat that file as your current login.
- Switching always writes the Keychain, never the file. If the login was only in the file, hatrack
  moves it into the Keychain and removes the file, the same thing Claude does when its Keychain
  write succeeds. hatrack never falls back to writing tokens to a plain file on macOS; if the
  Keychain write fails, the switch fails and says so.
- `hat add` signs in with a scratch `CLAUDE_CONFIG_DIR`, so Claude puts the new tokens in their own
  Keychain item. hatrack copies them out and deletes that scratch item.
- hatrack calls `/usr/bin/security`, the same tool Claude uses, so macOS does not ask for Keychain
  access. If a prompt does appear, it names `security`; allow it.
- Saved logins in `~/.hatrack` are files (mode `0600`), not Keychain items.

## How it works

A Claude Code login is two things:

- the OAuth tokens: `~/.claude/.credentials.json`, or on macOS the Keychain (see [macOS](#macos)).
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
- Claude rewrites `~/.claude.json` while it runs. If it changes while hat is editing it, hat redoes its
  edit on the new content instead of overwriting Claude's. Before each change, hat keeps a copy of the
  file in `~/.hatrack/backups` (the last 10).
- `CLAUDE_SECURESTORAGE_CONFIG_DIR`, which moves only where Claude keeps tokens, is honored too, and is
  left out of the environment of the sign-in that `hat add` runs.

## Security and privacy

- hatrack makes **no network requests** of its own. Tokens never leave your machine; only Claude itself
  talks to Anthropic. The one exception is `hat mcp test` on a remote (http) MCP server, which you run
  on purpose: it contacts that server, with the headers you configured, as Claude would. Testing a
  local server runs its command, which may download it (`npx`, `uvx`).
- MCP server configs (connection strings, tokens) stay where Claude keeps them; switched-off ones go to
  `~/.hatrack/mcp/disabled.json`, readable only by you. `hat mcp` and `show` hide secrets unless you
  pass `--reveal`; `export` writes them in full.
- Saved logins are plain files, like Claude's own `.credentials.json`, readable only by your user
  (mode `0600` on Linux and macOS; your user profile's permissions on Windows). Anyone who can read
  your home folder can read them, just as with Claude's own login. On macOS this means a saved login
  is less protected than the active one, which Claude keeps in the Keychain.
- hatrack reads and writes Claude's login files (on macOS, Claude's Keychain items), its MCP server
  settings and `~/.hatrack`. `hat usage` only reads the token counts in Claude's transcripts, not what
  you wrote; `hat clean` changes only what it listed, and only with `--apply`. It does not touch
  Claude's traffic, telemetry or anything else.
- No telemetry, no accounts, no server. The macOS dashboard is a local page on `127.0.0.1` that
  needs a per-launch token, loads nothing from the internet, and stops when you quit Hatrack.

Anthropic's terms do not allow sharing one subscription between people. hatrack is for switching
between accounts that are yours. Found a security problem? See [SECURITY.md](SECURITY.md).

## Limits

- **Claude Code only** for now. Codex and other tools may come later. `hat mcp import --desktop` reads
  Claude Desktop's servers but does not change them.
- The Windows tray and the macOS menu bar show user-scope MCP servers only; manage local and project ones with `hat mcp` in
  the folder. The same goes for the macOS menu bar.
- `hat mcp test` speaks stdio and streamable HTTP; servers on the older `sse` transport show their state in Claude's `/mcp`.
- **macOS app is not notarized**, so Gatekeeper asks before the first launch.
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
| `keychain …` error on macOS | Unlock the login Keychain (Keychain Access, or `security unlock-keychain`) and retry. Over SSH the Keychain is usually locked. |
| `hat` not found right after installing on Windows | Open a new terminal so it picks up the updated `PATH`. |
| Windows SmartScreen warns about the exe | The binaries are not code-signed. Verify them as shown in [Verify a download](#verify-a-download). |

## Uninstall

hatrack never changes Claude's login on its own, so after uninstalling, Claude keeps using whichever
account was active.

**Windows:** quit the tray app, then in PowerShell:

```powershell
Remove-Item "$([Environment]::GetFolderPath('Startup'))\hatrack.lnk", "$env:LOCALAPPDATA\hatrack" -Recurse
Remove-Item "$HOME\.hatrack" -Recurse   # saved logins, switched-off MCP servers, sets, switch history, backups
```

and remove `%LOCALAPPDATA%\hatrack` from your user `PATH` (Settings → System → About → Advanced system
settings → Environment Variables).

**macOS, Linux and WSL:** on macOS, quit Hatrack from the menu bar first.

```sh
rm ~/.local/bin/hat
rm -rf /Applications/Hatrack.app ~/Library/Caches/hatrack   # macOS app
rm -r ~/.hatrack   # saved logins, switched-off MCP servers, sets, switch history, backups
```

## Build from source

Needs Go (see `go.mod` for the version). No C compiler is needed, except for the macOS app.

```sh
go test ./...
go build ./cmd/hat                                                         # CLI for this OS
GOOS=windows go build -ldflags -H=windowsgui -o hatrack-tray.exe ./cmd/hatrack-tray  # tray, from any OS
./build-mac-app.sh                                                         # Hatrack.app, on a Mac
```

Layout:

```
cmd/hat/              CLI
cmd/hatrack-tray/     Windows tray app
cmd/hatrack-gui/      macOS menu bar app
internal/claude/      Claude logins: save, swap, switch history, forgetting deleted folders
internal/mcp/         MCP servers: scopes, on/off, catalog, sets, test
internal/usage/       token usage from Claude's transcripts
internal/atomicfile/  crash-safe file writes
internal/gui/         the macOS dashboard: a local page and its API
internal/tray/        the macOS menu bar
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
