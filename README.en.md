# Kuake CLI

[中文说明](README.md) · [License](LICENSE)

A command-line tool for managing files on **Quark Cloud Drive** (夸克网盘).

## Open source notice

This project is licensed under **AGPL-3.0**. **Commercial use** (including SaaS and integration into commercial products) requires a separate license. Source code is available from this repository; run `./build.sh` to build binaries for multiple platforms locally. Issues and pull requests are welcome.

## Table of contents

- [Features](#features)
- [Requirements](#requirements)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Documentation](#documentation)
- [Development](#development)
- [Disclaimer](#disclaimer)
- [License](#license)
- [Star history](#star-history)
- [Contributors](#contributors)

## Features

- User profile, directory listing, file metadata, upload/download, create folder, move/copy/rename/delete
- Create and revoke shares, list shares, save others’ shares to your drive
- JSON output and pipe-friendly mode (e.g. with `jq`); optional **OpenClaw** skill ([openclaw/kuake_skill/](openclaw/kuake_skill/)): install `kuake` from [Releases](https://github.com/zhangjingwei/kuake_cli/releases), add it to `PATH`, set `KUAKE_COOKIE` (see [openclaw/kuake_skill/SKILL.md](openclaw/kuake_skill/SKILL.md) and [docs/cli.md](docs/cli.md))
- **`kuake-mcp` MCP server**: a stdio MCP server exposing the 14 drive operations as MCP tools for clients like Claude Code. An env-driven blacklist (`KUAKE_DENY_OPS` / `KUAKE_DENY_PATHS` / `KUAKE_DENY_EXTS` / `KUAKE_MAX_UPLOAD_MB` / `KUAKE_DOWNLOAD_DIR`) restricts which operations and paths the MCP client can reach (see [.mcp.json.example](.mcp.json.example))

> **v1.5.0 BREAKING:** `config.json` support has been removed; the `-c, --config` flag is gone. Credentials may come from `KUAKE_COOKIE`, `KUAKE_PUS+KUAKE_PUUS`, `-cookies`, or a Cookie previously persisted with `kuake auth save`. Migrate old tokens to one of these sources (a `.env` file works).

For full CLI usage see [docs/cli.md](docs/cli.md). Copy [.env.example](.env.example) as a template for environment variables. If a `.env` file exists, `kuake` loads it from the current working directory after parsing flags (set `KUAKE_LOAD_DOTENV=0` to disable). Values already exported in the shell are not overwritten.

More documentation:

- [specs/architecture/spec.md](specs/architecture/spec.md)
- [openclaw/kuake_skill/SKILL.md](openclaw/kuake_skill/SKILL.md) (OpenClaw skill)

## Requirements

- Linux, macOS, or Windows
- A valid Quark Cloud Drive account and browser Cookie

## Installation

### Build from source

Requires **Go 1.25+** (as in `go.mod`) and Git.

```bash
git clone https://github.com/zhangjingwei/kuake_cli.git
cd kuake_cli
chmod +x build.sh
./build.sh
```

Artifacts are written under `dist/`: CLI as `kuake-{version}-{os}-{arch}` and MCP server as `kuake-mcp-{os}-{arch}` (5 platforms).

### Prebuilt binaries

Download the archive for your OS from [Releases](https://github.com/zhangjingwei/kuake_cli/releases). Exact filenames follow the release page for each version.

Directory downloads support concurrent transfers and resume from identity-checked state in the system user cache (`kuake/downloads`), not sibling `.part` files. Existing `.part` files are left untouched; old unverified partial files are downloaded again. Final installation uses a temporary copy in the destination directory, so additional disk space is required.

## Quick start

1. Copy [.env.example](.env.example) to `.env` and fill in your Quark Cookie as described in the file (e.g. from the browser devtools Network tab). **Do not commit `.env`.** Alternatively, set the Cookie temporarily and run `kuake auth save`; the stored file uses mode `0600` on Linux/macOS and a protected, current-user-only ACL on Windows.
2. In the project directory (adjust the binary name for your build):

```bash
./kuake user
./kuake list "/"
./kuake upload "file.txt" "/file.txt"
```

See [docs/cli.md](docs/cli.md) for flags and credential fallbacks. For running from source during development, see **Development** below.

## Documentation

| Document | Description |
| -------- | ----------- |
| [docs/cli.md](docs/cli.md) | CLI configuration, commands, JSON conventions, examples |
| [docs/CHANGELOG.md](docs/CHANGELOG.md) | Release history |
| [docs/DISCLAIMER.md](docs/DISCLAIMER.md) | Full disclaimer |
| [openclaw/kuake_skill/](openclaw/kuake_skill/) | **OpenClaw users:** add the folder that contains `SKILL.md` to your OpenClaw skill paths; install `kuake` from [Releases](https://github.com/zhangjingwei/kuake_cli/releases) and `PATH`, configure `KUAKE_COOKIE` per [docs/cli.md](docs/cli.md). No other files from this repo are required. |
| [.mcp.json.example](.mcp.json.example) | Claude Code MCP integration template for `kuake-mcp`: shows `KUAKE_COOKIE` plus blacklist/sandbox env vars. Copy to `.mcp.json` with your real cookie (already in `.gitignore`). |

## Development

**Before opening a PR**, run the same checks as in [.github/workflows/ci.yml](.github/workflows/ci.yml) (**lint** / **test**):

```bash
golangci-lint run ./...
go test ./... -count=1
```

Lint rules live in [.golangci.yml](.golangci.yml). Install the linter with e.g. `go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.63.4` and add `$(go env GOPATH)/bin` to `PATH`.

**As a Go module dependency** in another project:

```bash
go get github.com/zhangjingwei/kuake_cli@latest
```

## Disclaimer

This tool is an **unofficial** third-party project and is not affiliated with Quark Cloud Drive. Use may involve account risk, data loss, or breakage when APIs change—**use at your own risk**. By using the software you acknowledge the full terms in [docs/DISCLAIMER.md](docs/DISCLAIMER.md).

## License

This project is under **AGPL-3.0**; see [LICENSE](LICENSE). Derivative works must use the same license. **Commercial use** requires permission from the maintainers.

## Star history

[![Star History Chart](https://api.star-history.com/chart?repos=zhangjingwei/kuake_cli&type=date&legend=top-left)](https://www.star-history.com/?type=date&repos=zhangjingwei%2Fkuake_cli)

## Contributors

Thanks to everyone who participates via issues and pull requests. Full statistics: [Contributors](https://github.com/zhangjingwei/kuake_cli/graphs/contributors).

Issues and pull requests are welcome.
