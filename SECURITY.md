# Security

hatrack copies Claude Code login tokens between files on your machine, so security bugs matter here.

## Reporting

Please report security problems privately through
[GitHub security advisories](https://github.com/phongptn93/hatrack/security/advisories/new),
not in a public issue. Include what you found, how to reproduce it, and what an attacker could do with it.

You will get an answer within a week.

## In scope

- Tokens written somewhere other users can read, or left behind after they should be deleted.
- A way to make hatrack read or write files outside Claude's login files and `~/.hatrack`.
- Anything that sends tokens over the network. hatrack should make no network requests at all.
- Tampered release binaries: check them with `gh attestation verify` first.

## Never send

Your `.credentials.json`, a saved login from `~/.hatrack`, or any token, even an expired one.
