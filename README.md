# Golden Gate Setup

A Go application for first-time setup of Warp, Fish, Zed, development tools, and recovery records on **native Apple-silicon macOS 27 Golden Gate**.

Choose your tools, review the actual changes, then accept the plan. Existing apps and configuration are preserved by default. Applite, Cork, and command-line package management share Homebrew at `/opt/homebrew`.

**0.1.0-rc.1 · MIT licensed.** This complete release candidate includes integration, race, and terminal verification. Clean-Mac installation, actual GUI behavior, VoiceOver, and data retrieval still need [manual acceptance](docs/manual-acceptance.md). Release artifacts are unsigned and not notarized; no external publication has occurred.

## Start from source

Open a terminal in the project directory:

```sh
zsh scripts/bootstrap.zsh --plan  # Inspect prerequisites without writes/downloads
zsh scripts/bootstrap.zsh         # Confirm prerequisites, build, open wizard
```

The starter checks macOS, architecture, Command Line Tools, native Homebrew, and Go 1.26+. Missing prerequisites require an explicit `yes`. If Apple’s developer-tools installer opens, finish it and rerun. The Homebrew installer is pinned and checksum verified. Confirmed preparation can deliberately install or update Go.

For plain prompts use `zsh scripts/bootstrap.zsh -- --accessible`. `--build-only` builds without opening the wizard, but can still prepare confirmed prerequisites. The starter’s `--plan` is its read-only mode.

## Use the application

```sh
./bin/golden-setup --plan       # Read-only JSON plan; no saved session
./bin/golden-setup              # Interactive wizard
./bin/golden-setup --accessible
./bin/golden-setup --help
./bin/golden-setup --version
```

The starter builds `bin/golden-setup`; the repository does not include prebuilt executables. In an extracted binary archive use `./golden-setup`, without `bin/`. A prebuilt executable needs no Go compiler. Installation still needs native Homebrew and Command Line Tools; the source starter can prepare them.

Use arrows and Space to select options; Enter advances. Preview/summary pages scroll with arrows or Page Up/Page Down. `e` edits a preview; `a` accepts its supported plan. Each differing file shows a local diff: `y` approves replacement with a private backup; `n` or Enter preserves it. Escape/Ctrl+C request exit or cancellation. `q` remains ordinary input text and closes a completed summary. At least 48 × 18 cells are required for interaction; smaller windows pause input. Light/dark styling follows the terminal background.

Plain mode emits no ANSI. Lists accept comma-separated choices or `none`; questions accept `yes`/`no`. Incomplete input fails clearly. Use a real terminal for installation: piped buffered answers cannot hand terminal ownership to password prompts.

Exit codes: `0` for completed automated work, a declined preview, or metadata; `1` for operational failure/cancellation; `2` for invalid usage or an unsupported JSON plan. The starter returns `3` while developer-tools installation is pending. Automated completion still lists manual work.

## Included work

- Missing core tools: Fish, Starship, zoxide, chezmoi, GitHub CLI, fzf, fd, bat, eza, ripgrep, delta, lazygit.
- Selectable Warp, Zed, Applite, GitHub Desktop, and KopiaUI. Cork’s official licensed distribution/source remains a manual choice.
- Reviewed Fish, Starship, Zed, and lazygit configuration, with file-hash checks and Fish syntax verification. Git changes only its editor and three delta display settings.
- Individually selected Fish plugins through a pinned Fisher bootstrap. Custom functions/manifests and legacy state receive manual merge tasks.
- Local chezmoi adoption of chosen files with hooks and automatic Git actions disabled. Existing managed entries and pending source edits are preserved.
- Optional Go/Crystal/Nim toolchains, servers, and new workspaces using actual project/module names. Existing source and manifests survive.
- Dated Homebrew reinstall records, exhaustive installed-package/app metadata, and recovery instructions. Native Applite export remains a visible GUI step.

Missing Go/Nim servers use gopls 0.23.0 and nimlangserver 1.14.0. Nimble may request its required Nim 2.0.8 build compiler in its managed area; review the foreground prompt. Recorded Homebrew versions are metadata, not an installation lock. Required old Fish/lazygit/Go versions need deliberate updates. Custom tap substitutions or `XDG_CONFIG_HOME` require reconciliation before automatic setup; a chezmoi source outside the home is adopted manually.

## Preserve and recover

Replacements use an atomic write, an existing-state recheck, and a private backup first. Symlink targets/ancestors are preserved or rejected; only verified macOS `/var` and `/tmp` aliases are accepted. Sessions/backups live in `~/Library/Application Support/Golden Gate Setup/sessions/`; exports live in `~/Golden Gate Recovery/`.

Cancellation stops new steps and records interrupted work. Start again, restore saved choices if offered, inspect and approve a new plan. Existing registrations and matching files are skipped. Partial projects/recovery exports can need manual review. See [troubleshooting](docs/troubleshooting.md).

App lists, inventories, and local chezmoi adoption each save different parts of the setup. **None backs up personal data.** Complete and test Time Machine or Kopia separately. After reinstalling macOS, migrate an existing account before rebuilding it. See [recovery](docs/recovery.md).

## Development and release

Go 1.26+ is required; Charm dependencies and embedded assets are pinned. Tests use temporary homes and fake external services. The fixture executable is separate from the shipped application.

```sh
go test -race -count=1 ./...
go vet ./...
go build -o bin/golden-setup ./cmd/golden-setup
go build -o bin/golden-setup-fixture ./internal/testutil/cmd/fixture
python3 scripts/pty-smoke.py --fixture bin/golden-setup-fixture
python3 -m unittest discover -s scripts -p '*_test.py'
zsh -n scripts/bootstrap.zsh
```

### Develop in Zed

Clone and open the project in Zed 0.219.4 or later in one step:

```sh
open 'zed://git/clone?repo=https://github.com/empeirikosevelynh-debug/home-setup-app.git'
```

You can also use **git: clone** from the command palette, or run `zed .` in an existing clone. Zed opens a new project in Restricted Mode: review `.zed/`, then trust the project to turn on its settings, the Go language server and its tasks.

- **Tasks** (`task: spawn`): tests, race tests, vet, the gofmt check, build, the read-only `--plan` preview, the wizard and plain prompts against the sandbox fixture, terminal smoke tests, and script checks. No task runs the real wizard. Task commands work in fish, zsh and bash; the sandbox tasks need macOS.
- **Debugging** (Delve, `brew install delve`): the read-only preview, the tests in the current file's package, and any test or `main` from the gutter.
- **Formatting:** Go files are formatted with gofmt on save. Other files, including the embedded templates, are left as written.

See [verification](docs/verification.md), [contributing](CONTRIBUTING.md), [security](SECURITY.md), and [local release packaging](docs/releasing.md). Packaging creates a native archive, matching source archive, build/module inventory, dependency licenses, and SHA-256 checksums from a clean commit. It never publishes. The module has a local name; choose a real public repository identity before offering `go install` instructions.

Application code uses the [MIT license](LICENSE); dependencies retain their terms in [third-party notices](THIRD_PARTY.md). The original guide, PDF, and configuration examples are read-only references and are not runtime requirements.
