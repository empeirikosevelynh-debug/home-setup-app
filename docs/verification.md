# Verification — 2026-09-30

Release candidate: **0.1.0-rc.1**. All installation/execution tests use isolated temporary homes and fake external services. Real commands in these tests are limited to local Git configuration editing, shell/terminal fixtures, and build/check tooling. No live setup was applied.

## Environment and dependencies

- Native Darwin arm64; macOS 27.0.1, build 26A434.
- Official Go 1.27.1 Darwin arm64 archive, verified SHA-256 `ee215d57e0ec269c60cc9ceca68e6bda321ba9ee5afe24f4b0988703c2d87d12`; installed in project-only development tooling, not the system PATH.
- Product module floor 1.26.0. Bubble Tea 2.0.10, Huh 2.0.3, Lip Gloss 2.0.6; full graph/checksums in go.mod/go.sum.
- Go vulnerability checker 1.8.0: **no vulnerabilities found** for the production command. Downloaded installations/plugins are separate publisher inputs.

## Completed checks

- Formatting, `go vet ./...`, readonly module-tidiness check, production/fixture builds, and the entire `go test -race -count=1 ./...` suite pass.
- Fresh setup, migrated configuration/app channels, selected workspaces, repeat execution, interruption/resume, and truthful manual completion reports pass as complete cross-component flows.
- Writer checks cover backup before replacement, private permissions, symlinked roots/ancestors, intervening edits, write failure, and exclusive lock release/refusal.
- Git/chezmoi/plugin checks preserve identity/includes, custom functions/manifests, existing managed templates, dirty sources, and disabled automatic Git actions/hooks.
- Diagnostics handle fragmented/multiword credentials, remove controls, and bound output. Local file diffs cannot emit terminal controls.
- Native source-starter syntax and six isolated prerequisite/argument cases pass. No live prerequisite installation was used to test it.
- Four Python release-helper checks pass. Packaging refuses dirty source and existing output, validates version input, and requires dependency license texts.
- **19 real pseudo-terminal/plain cases pass**: eight cases in each light/dark theme (finish, q input, successful foreground handoff, foreground Ctrl+C, child failure, apply cancellation, Escape, Ctrl+C) plus sequential plain answers and two EOF failures. Hidden input remains paused across resize; entered q survives hidden text/paste. Fixtures check restored terminal attributes before exit; macOS's transient PENDIN bit is excluded, while persistent modes and control characters are compared.
- Direct model/layout checks cover both themes, real forms, long Unicode output, 120×40, 80×24, 48×18, 40×12, 20×6, 1×1, and 0×0. The card is bounded and centered.
- The production command's actual read-only preview on this Mac reports supported, 15 planned steps, and no compatibility problems. No plan was accepted or applied. Metadata/help checks do not inspect the host.
- All **16 original reference fingerprints remain unchanged**.

The CI workflow is prepared for Go 1.26.x/1.27.x on macOS. Remote CI has not run; local checks above used Go 1.27.1. CI action pins were verified against [checkout 7.0.1](https://github.com/actions/checkout/releases/tag/v7.0.1) and [setup-go 7.0.0](https://github.com/actions/setup-go/releases/tag/v7.0.0).

## Release review

Local archive verification and a fresh independent whole-branch review are recorded here after they complete. Hardware/application acceptance remains in [manual-acceptance.md](manual-acceptance.md). Fixture success is not evidence of a completed personal-data backup, actual screen-reader usability, signed distribution, or clean-Mac installation.

Pinned optional servers were checked against [gopls 0.23.0 go.mod](https://raw.githubusercontent.com/golang/tools/014f87ff5c01915bc90f4f11a6bb8aea3e0edbd7/gopls/go.mod) and [nimlangserver 1.14.0 manifest](https://raw.githubusercontent.com/nim-lang/langserver/v1.14.0/nimlangserver.nimble). Go 1.26 is sufficient for gopls; Nimlangserver requires Nim 2.0.8 to build. The preview and manual tasks disclose Nimble’s potential compiler preparation; real installation remains pending.
