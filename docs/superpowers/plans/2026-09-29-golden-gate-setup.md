# Golden Gate Setup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a usable first-time setup assistant that previews, applies, and verifies the approved Warp/Fish/Zed environment and prepares its recovery records on macOS 27.

**Architecture:** A native Zsh starter prepares source-build prerequisites, then launches a Go/Charm wizard. A shared typed plan drives the interactive, plain, and JSON-preview modes; separate inspection, file, command, and session components preserve existing state and verify each accepted change. This is one integrated implementation plan because these components share the same plan and verification contracts.

**Tech Stack:** Go 1.26.0 or later; Bubble Tea v2.0.10; Lip Gloss v2.0.6; Huh v2.0.3; native Zsh; Apple-silicon Homebrew; standard Go libraries for subprocesses, files, hashing, JSON, and tests.

**Spec:** [Approved design](../specs/2026-09-29-golden-gate-setup-design.md). Read both documents before implementation.

**Status:** Approved by Evelyn on September 29, 2026 for direct implementation in this chat, followed by independent review of the whole change. Tasks are tracked below and in the execution ledger.

## Global Constraints

- The initial target is macOS 27 on Apple silicon with native Homebrew at `/opt/homebrew`; the executable is `/opt/homebrew/bin/brew` and apps use `/Applications`.
- Pin `charm.land/bubbletea/v2` at `v2.0.10`, `charm.land/lipgloss/v2` at `v2.0.6`, and `charm.land/huh/v2` at `v2.0.3`. Bubble Tea requires Go 1.26.0 or later.
- Cap the outer card at 72 columns. Full interaction begins at 48 × 18; smaller terminals show a bounded resize notice and pause hidden input. A zero-sized terminal renders nothing.
- Use pointer-based form ownership, `tea.KeyPressMsg`, and `tea.View`; blocking work belongs in commands, and only one component owns the terminal. No hand-written terminal escape sequences.
- `q` remains normal text while editing. Ctrl+C and Escape request exit; cancellation stops new steps, safely stops or finishes the active child, and records interrupted work honestly.
- `--plan` never modifies the host. Applying changes always requires an accepted plan. A noninteractive apply mode is outside this implementation.
- Existing settings, symlinks, app distribution channels, curated inventories, and pending chezmoi edits are preserved unless a specific change was accepted. Recheck state immediately before writes.
- Applite native app selections, complete Homebrew inventories, and actual data backups remain distinct. Cork uses its official distribution, with no invented cask or license bypass.
- Authentication, remote uploads, undocumented app preferences, backup credentials, disk erasure, OS installation, and restoration over the active home directory are not automatic operations.
- Development tests use temporary homes and fake commands. Do not run the installer against Evelyn’s live home directory as a test. Original guide/PDF/configuration references and synced `sources/` remain unchanged.

## Review Focus

1. A symlinked parent directory or a file edited after preview must not redirect or overwrite an unreviewed target: Task 4 tests both.
2. Custom Fish functions or an existing plugin selection must survive plugin installation: Task 6 tests refusal of conflicting automatic work and preservation of unrelated plugins.
3. Existing casks, unregistered vendor apps, and migrated apps must not be reinstalled automatically: Tasks 1 and 5 test channel preservation and a fresh inspection before each install.
4. Cancellation, an interactive password prompt, or a stale completion record must not create a false success: Tasks 3 and 5 test terminal handoff, interruption, and verification on resume.
5. Paths with spaces, Unicode labels, long lists, and EOF in plain prompts must remain usable: Tasks 3, 7, and 8 test bounded rendering, argument handling, and incomplete input.

## File structure and shared contracts

All paths below are relative to this project’s root. Module name: `golden-gate-setup` (local project; no invented repository owner). Command name: `golden-setup`.

| Location | Responsibility |
|---|---|
| `cmd/golden-setup/main.go` | Flags, dependency wiring, mode selection, final diagnostics, exit codes |
| `internal/domain/types.go` | Host snapshots, options, steps, file decisions, progress events, reports |
| `internal/plan/catalog.go`, `build.go`, `workspace.go`, `manual.go` | Reviewed selections, deterministic dependency ordering, workspace and finish tasks |
| `internal/templates/embed.go`, `assets/`, `manifest.json` | Self-contained reviewed configuration and workspace templates with provenance |
| `internal/inspect/host.go`, `homebrew.go`, `files.go`, `chezmoi.go` | Read-only host, package, app, and source-state inspection |
| `internal/command/runner.go`, `handoff.go` | Argument-array subprocess execution, streamed output, exclusive terminal handoff |
| `internal/files/snapshot.go`, `write.go` | Conflict checks, reviewed backups, atomic writes, permission preservation |
| `internal/apply/execute.go`, `session.go`, `verify.go`, `git.go`, `plugins.go`, `chezmoi.go`, `languages.go`, `recovery.go` | Verified step execution, persistence, and action-specific policies |
| `internal/ui/model.go`, `forms.go`, `view.go`, `theme.go`, `plain.go` | The wizard and equivalent plain prompts |
| `internal/testutil/fixtures.go`, `runner.go` | Temporary-home and fake-command support, shared by behavior tests |
| `scripts/bootstrap.zsh`, `scripts/pty-smoke.py` | Fresh-Mac source startup and isolated terminal smoke checks |
| `docs/manual-acceptance.md`, `docs/verification.md` | Unexecuted real-Mac checks and recorded development evidence |

Define these domain types in Task 1; add fields only when a listed behavior requires them:

- `Options`: `Apps`, `Plugins`, `Languages` (`[]string`); `Workspaces` (`[]Workspace`); `ConfigureGit`, `AdoptChezmoi`, `CaptureInventory`, `PrepareRecovery` (`bool`); `FileChoices` (`map[string]FileDecision`).
- `Workspace`: `Language`, `Path`, `Module`, `EntryPoint` (`string`); `Create` (`bool`).
- `Host`: `OS`, `Arch`, `Version`, `Home`, `BrewPath`, `BrewPrefix`, `LazyGitDir`, `ChezmoiDir` (`string`); `Rosetta`, `ChezmoiDirty` (`bool`); `Packages` (`map[string]InstalledPackage`); `Apps` (`[]AppBundle`); `Files` (`map[string]FileState`); `Tools` (`map[string]string`). Package keys include kind, for example `formula:fish` and `cask:zed`.
- `InstalledPackage`: `Version` (`string`). `AppBundle`: `Name`, `Path` (`string`), `Registered`, `StoreReceipt` (`bool`). `FileState`: `Path`, `SHA256` (`string`), `Mode` (`fs.FileMode`), `Exists`, `Symlink` (`bool`), `Contents` (`[]byte`, excluded from JSON).
- `Package`: `Kind`, `Token`, `MinVersion` (`string`). `Command`: `Path`, `Dir` (`string`), `Args`, `Env` (`[]string`), `Interactive` (`bool`). `FileDecision`: `preserve`, `create`, or `replace`; the last requires a reviewed existing-file diff.
- `FileChange`: `Path`, `BeforeSHA256` (`string`), `BeforeExists` (`bool`), `Mode` (`fs.FileMode`), `Desired` (`[]byte`), `Decision` (`FileDecision`). `Check`: `Kind`, `Target`, `Expected` (`string`).
- `Step`: `ID`, `Label`, `Kind` (`string`), `DependsOn` (`[]string`), `Package` (`*Package`), `Command` (`*Command`), `File` (`*FileChange`), `Check` (`Check`). Step kinds select known handlers, not arbitrary executable snippets.
- `Plan`: `SchemaVersion` (`int`, initially 1), `ID` (`string`), `Supported` (`bool`), `Accepted` (`bool`, excluded from JSON and ID hashing), `Problems` (`[]string`), `Options` (`Options`), `Steps` (`[]Step`), `ManualTasks` (`[]ManualTask`). `Accepted` is set only by the in-session confirmation; a saved plan never supplies approval. `ManualTask`: `ID`, `Title`, `Instructions`, `URL` (`string`), `Required` (`bool`).
- `Event`: `StepID`, `Status`, `Text` (`string`). `Report`: `PlanID`, `Status`, `SessionPath` (`string`), `Steps` (`[]StepResult`), `ManualTasks` (`[]ManualTask`). `StepResult`: `ID`, `Status`, `Message`, `BackupPath` (`string`). Statuses distinguish skipped, verified, failed, interrupted, and manual work remaining.
- `Session`: `SchemaVersion` (`int`, initially 1), `Plan` (`Plan`), `Results` (`[]StepResult`). It preserves selected options for resumption; saved before-state and completion records still need fresh inspection.

No host file contents or raw authentication values enter saved JSON reports. File previews are shown locally; persistent records retain the target, intended operation, fingerprints, and result. Plan IDs hash the normalized intended actions and before-state; they do not authorize changes to a later, different state.

## Milestone 1: working selection and preview

### Task 1: reviewed catalog, embedded templates, and pure planner

**Files:** Create `go.mod`, `go.sum`, `.gitignore`; `internal/domain/types.go`; `internal/plan/catalog.go`, `build.go`, `manual.go`, `build_test.go`; `internal/templates/embed.go`, `manifest.json`, `assets/`; `internal/testutil/fixtures.go`.

**Interfaces:** Produces `plan.DefaultOptions() domain.Options`, `plan.Build(domain.Host, domain.Options) (domain.Plan, error)`, `templates.Load(name string) ([]byte, error)`, `testutil.FreshHost(home string) domain.Host`, and `testutil.TempHome(t *testing.T) string`. This task consumes only reviewed source assets and the spec.

- [ ] **Preparation: establish the development environment.** Obtain a supported Go toolchain in a task-local location if needed; create the minimal module files with the pinned dependencies and resolve their graph. Initialize a local Git repository if none exists, using existing identity rather than changing global Git configuration. This happens only after execution is authorized and does not install the full setup.
- [ ] **Step 1: Establish the failing planner tests.** `TestFreshPlan` expects the 12 core formulae, casks `warp`, `zed`, `applite`, schema 1, and prerequisite/manual tasks. `TestOptionalSelections` expects no `github`, `kopiaui`, Go/Crystal/Nim tools, or extra plugins unless selected. `TestRegisteredAndVendorApps` expects an installed cask to be skipped and a matching unregistered bundle to produce a manual adoption choice, with no `--force` or implicit `--adopt`. `TestExistingSettingsPreserved` expects differing files to default to preserve, identical files to be skipped, and dirty chezmoi adoption to be deferred. `TestWorkspaceTemplatesEmbedded` loads all six hidden workspace files successfully.

  Assertion example in `TestFreshPlan`, after calling the listed planner interface:
  ```go
  if err != nil || !got.Supported || got.SchemaVersion != 1 || got.Accepted {
      t.Fatalf("fresh preview must be supported and unaccepted: %v, %+v", err, got)
  }
  ```
- [ ] **Step 2: Run `go test ./internal/plan ./internal/templates -v`.** Expect failure because the planner and embedded templates are absent.
- [ ] **Step 3: Implement the listed types, catalog, template loading, and `Build`.** Copy the reviewed Fish/Starship/Zed/lazygit and six hidden `.zed` workspace files into assets; record hashes, source path, and date. Use `//go:embed all:assets` so hidden `.zed` paths are included. Do not copy the all-options `fish_plugins` file or app inventory as an installation manifest. Core formulae are `fish`, `starship`, `zoxide`, `chezmoi`, `gh`, `fzf`, `fd`, `bat`, `eza`, `ripgrep`, `git-delta`, `lazygit`. Recommended plugins are the guide’s four starting plugins, plus Fisher when needed. Extra plugins and languages remain deselected. Set minimum Fish 4 for fzf.fish and the guide’s lazygit 0.65.1 configuration; show incompatible installed versions as reviewed update decisions, not silent upgrades.
- [ ] **Step 4: Run `go test ./internal/plan ./internal/templates -v`.** Expect all cases to pass, including unknown selection rejection and deterministic step ordering/IDs. Assert no shell command text is parsed from the source documents.
- [ ] **Step 5: Commit this deliverable.** Stage only this task’s files and commit `feat: add reviewed setup catalog and planner`.

### Task 2: read-only inspection and JSON preview command

**Files:** Create `internal/command/runner.go`; `internal/inspect/host.go`, `homebrew.go`, `files.go`, `chezmoi.go`, `inspect_test.go`; `internal/testutil/runner.go`; `cmd/golden-setup/main.go`, `main_test.go`.

**Interfaces:** Consumes Task 1 types and `plan.Build`. Produces `command.Runner` with `Run(context.Context, domain.Command, io.Writer) error`; `inspect.Inspector` with `Read(context.Context, domain.Options) (domain.Host, error)`; `main.run(context.Context, []string, io.Reader, io.Writer, io.Writer) int`; and `testutil.FakeRunner` that records calls and supplies scripted output. Inject the runner, home path, and platform probes so tests never depend on the real host. Options identify any selected workspace files that must be inspected before planning their changes.

- [ ] **Step 1: Write `TestPreviewDoesNotWrite`, `TestMissingBrew`, `TestConflictingPrefix`, `TestRosettaAndUnsupportedOS`, `TestMalformedProbeOutput`, and `TestChezMoiDirtySource`.** Assert no command outside the read-only probe list runs, the temporary tree remains byte-for-byte unchanged, OS major 27/arm64/native prefix is accepted, other cases are clearly incompatible, and malformed output cannot become an empty “fresh” installation.

  Assertions in `TestPreviewDoesNotWrite`, with before/after filesystem snapshots and recorded mutating calls:
  ```go
  if len(mutatingCalls) != 0 || !reflect.DeepEqual(before, after) {
      t.Fatal("preview changed the host")
  }
  ```
- [ ] **Step 2: Run `go test ./internal/inspect ./cmd/golden-setup -v`.** Expect the new inspection/CLI assertions to fail.
- [ ] **Step 3: Implement `Inspector.Read` and `main.run`.** Probe OS/version, hardware and translation, `/opt/homebrew/bin/brew --prefix`, installed formula/cask JSON, relevant tool paths, app bundle metadata/receipts, reviewed file paths (including selected workspaces), lazygit’s printed config directory, and chezmoi source/status. Use `Lstat`, bounded reads, `HOMEBREW_NO_AUTO_UPDATE=1` and `HOMEBREW_NO_ANALYTICS=1` for read-only Homebrew probes, `GIT_OPTIONAL_LOCKS=0` for Git status probes, and a command timeout. Missing optional tools differ from unexpected probe failures. `--plan` prints one JSON object to stdout and diagnostics to stderr; it does not persist a plan or create application directories. Exit 2 for unsupported host or invalid options and 1 for an unexpected failure.
- [ ] **Step 4: Run `go test ./internal/inspect ./cmd/golden-setup -v` and `go build ./cmd/golden-setup`.** Expect passing fixtures and a built preview command. Inspection on the current Mac may be exercised read-only; do not call apply.
- [ ] **Step 5: Commit `feat: inspect host and provide read-only setup preview`** with this task’s files.

### Task 3: Charm selection, bounded preview, and plain prompts

**Files:** Create `internal/ui/model.go`, `forms.go`, `view.go`, `theme.go`, `plain.go`, `model_test.go`, `plain_test.go`; modify `cmd/golden-setup/main.go` and its tests.

**Interfaces:** Consumes the inspector/planner and domain records. Produces `ui.Services` containing `Inspect func(context.Context, domain.Options) (domain.Host, error)`, `Build func(domain.Host, domain.Options) (domain.Plan, error)`, `Apply func(context.Context, domain.Plan, func(domain.Event)) (domain.Report, error)`, and optional `LoadLatest func() (domain.Session, error)`; `ui.Run(context.Context, Services, io.Reader, io.Writer) (domain.Report, error)`; and `ui.RunPlain` with the same arguments/result. Reinspect after selecting workspace paths before building a new preview. A nil `Apply` means the milestone is honestly preview-only. `ui.HandoffMsg` carries a `domain.Command` and buffered `Done chan error` for later interactive subprocess handoff.

- [ ] **Step 1: Write `TestSelectionReachesPlan`, `TestCanceledDraftPreservesSelection`, `TestQIsTextWhileEditing`, `TestLayoutBounds`, `TestHiddenInputPaused`, `TestPlainAnswersRemainSeparate`, and `TestPlainEOF`.** Check both themes at 120×40, 80×24, 48×18, 40×12, 20×6, 1×1, 0×0; include wide Unicode labels, long lists, validation text, and paste messages. Plain successive answers remain separate; incomplete input fails; output has no ANSI escapes.

  Assertions in `TestPlainEOF` and `TestLayoutBounds`:
  ```go
  if err == nil || strings.ContainsRune(plainOutput, 0x1b) {
      t.Fatal("incomplete plain input must fail without ANSI output")
  }
  if lipgloss.Width(rendered) > width || lipgloss.Height(rendered) > height {
      t.Fatal("rendered content exceeds the available terminal")
  }
  ```
- [ ] **Step 2: Run `go test ./internal/ui -v`.** Expect failing interface and behavior checks before adding the UI.
- [ ] **Step 3: Implement the model/forms/theme/views and `Run`/`RunPlain`.** Use the PDF’s exact palette and pointer model, fresh draft forms, forwarded child commands, measured frames, scrollable previews/logs, and typed asynchronous result messages. Offer specific file decisions with local diffs, individual app/plugin/language choices, and a migration-first checkpoint. Track the inspected state used for approval. Plain mode is selected before the TUI and uses one shared input reader that prevents per-prompt read-ahead. Ctrl+C/Escape cancel; completed summaries accept q/Enter. Implement handoff with `tea.ExecProcess` and signal `Done` from its completion message; never run another form or terminal reader simultaneously.
- [ ] **Step 4: Run `go test ./internal/ui ./cmd/golden-setup -v` and `go build ./cmd/golden-setup`.** Expect a working selection/preview flow; apply remains visibly unavailable until Task 5 supplies it. `--accessible` follows the same options and decisions.
- [ ] **Step 5: Commit `feat: add responsive setup preview and accessible prompts`** with this task’s files.

## Milestone 2: verified installation and preserved configuration

### Task 4: safe file changes and recovery copies

**Files:** Create `internal/files/snapshot.go`, `write.go`, `write_test.go`; modify planner file-change generation and its tests.

**Interfaces:** Consumes `domain.FileState` and `domain.FileChange`. Produces `files.Manager` with `Inspect(path string) (domain.FileState, error)` and `Apply(change domain.FileChange, backupDir string) (domain.StepResult, error)`. The manager has a reviewed list of roots: the actual home and any individually selected workspace, not arbitrary writable paths.

- [ ] **Step 1: Write `TestCreateAndRepeat`, `TestDeclinedConflictUnchanged`, `TestBackupBeforeReplace`, `TestConcurrentEditRejected`, `TestSymlinkedParentRejected`, and `TestWriteFailurePreservesOriginal`.** Assert second-run no-op, backup bytes/permissions match the original, backups precede replacement, changed fingerprints abort, symlink parents cannot escape approved roots, and failed atomic replacement leaves the live file intact. Include a commented Zed settings file that survives preservation.

  Assertions in `TestBackupBeforeReplace` after reading the recorded backup and target:
  ```go
  if got.BackupPath == "" || !bytes.Equal(backup, original) || !bytes.Equal(live, desired) {
      t.Fatal("replacement must preserve a recovery copy and write the accepted bytes")
  }
  ```
- [ ] **Step 2: Run `go test ./internal/files -v`.** Expect the safety assertions to fail before implementing the writer.
- [ ] **Step 3: Implement `Manager.Inspect` and `Manager.Apply`.** Validate every path component and file type; compare existence and SHA-256 immediately before mutation. Create private backup/session directories (0700), protect saved copies (0600 or stricter), preserve the live file’s relevant mode, and use a same-directory temporary file with sync/close/rename. Preserve differing files by default; do not auto-parse and rewrite JSONC. Errors leave a truthful failed result and a usable backup location when one exists.
- [ ] **Step 4: Run `go test ./internal/files ./internal/plan -v`.** Expect all preservation and repeat-run checks to pass with temporary homes only.
- [ ] **Step 5: Commit `feat: preserve and atomically apply reviewed configuration changes`** with this task’s files.

### Task 5: command execution, verified state, cancellation, and resume

**Files:** Create `internal/command/handoff.go`, `runner_test.go`; `internal/apply/execute.go`, `session.go`, `verify.go`, `execute_test.go`, `session_test.go`; modify UI/CLI wiring and tests.

**Interfaces:** Consumes the runner, inspector, file manager, and plan. Produces `apply.Executor.Execute(context.Context, domain.Plan, func(domain.Event)) (domain.Report, error)` and `apply.SessionStore` with `Load(planID string) (domain.Session, error)`, `Latest() (domain.Session, error)`, and `Save(domain.Session) error`. The executor receives an injectable session store, inspector, runner, file manager, and clock. `command.ProcessRunner` implements `Runner` and accepts a terminal handoff callback for interactive commands.

- [ ] **Step 1: Write `TestFailureStopsDependents`, `TestInstallRechecksRegistration`, `TestCancellationSchedulesNoMoreWork`, `TestStaleCheckpointReverified`, `TestResumeRestoresChoicesWithoutApproval`, `TestUnacceptedPlanRejected`, `TestPlanStateChangedBeforeApply`, and `TestInteractiveHandoffHasOneOwner`.** Assert no later dependent command after failure, a newly installed/migrated cask is skipped, canceled work is not verified, stale session records cannot bypass checks, saved options return without implicit acceptance, accepted before-state must still match, and interactive prompts use one exclusive terminal route. Include malformed/truncated session JSON and command argument values containing spaces.

  Assertions in `TestUnacceptedPlanRejected`, with an unaccepted planner result and recorded mutating calls:
  ```go
  if err == nil || len(mutatingCalls) != 0 {
      t.Fatal("execution requires acceptance of the current plan")
  }
  ```
- [ ] **Step 2: Run `go test ./internal/command ./internal/apply ./internal/ui -v`.** Expect missing execution and resume behavior to fail.
- [ ] **Step 3: Implement execution, verification, and persistence.** Reject unsupported/unaccepted plans, execute in dependency order, and probe the relevant current state immediately before each action. Skip satisfied packages; no automatic upgrade/reinstall/force/adopt. A minimum-version update is a separate accepted step. Verify each action before saving its successful result. Persist schema-versioned session JSON atomically with mode 0600 beneath `~/Library/Application Support/Golden Gate Setup/sessions`; do not create it during preview. Save the selected options and intended plan alongside results. Resumption offers those choices, reinspects and rebuilds the remaining plan, and requires fresh acceptance. Cancel future work, safely interrupt supported children, wait for cleanup, and save interrupted state without pretending to roll packages back. Stream bounded sanitized diagnostics; omit credentials and raw host file contents.
- [ ] **Step 4: Wire `Services.Apply` to `Executor.Execute`.** Run `go test ./internal/command ./internal/apply ./internal/ui ./cmd/golden-setup -v`. Expect fake-runner application and resume tests to pass; generic dependency actions may still expose the specific handlers as unavailable until Tasks 6–7, with no false success.
- [ ] **Step 5: Commit `feat: execute accepted setup plans with verification and resume`** with this task’s files.

### Task 6: Git, Fisher, and chezmoi actions

**Files:** Create `internal/apply/git.go`, `plugins.go`, `chezmoi.go`, `configuration_test.go`; modify catalog/planner/verification handlers.

**Interfaces:** Consumes `Executor`, `Runner`, `Manager`, and the accepted plan. Produces `apply.ConfigureGit(context.Context, domain.Host, command.Runner) error`, `apply.InstallPlugins(context.Context, domain.Host, []string, command.Runner) error`, and `apply.AdoptChezmoi(context.Context, domain.Host, []string, command.Runner) error`, registered by known step kind. Planner preconditions expose conflicts before these handlers run; handlers recheck them.

- [ ] **Step 1: Write `TestGitPreservesIdentityAndIncludes`, `TestFisherBootstrapBeforeManifest`, `TestOnlySelectedPluginsInstalled`, `TestCustomFishFilesPreserved`, `TestExistingPluginsNotRemoved`, `TestDirtyChezMoiDeferred`, and `TestAdoptionNamesOnlyChosenFiles`.** Assert only the four guide Git keys change, unselected plugins never install, custom functions and manifest entries survive, existing source edits are untouched, and no recursive Fish implementation-file adoption or remote push occurs.

  Assertions in `TestGitPreservesIdentityAndIncludes`, using the read-back identity/include values:
  ```go
  if gotName != originalName || gotEmail != originalEmail || gotInclude != originalInclude {
      t.Fatal("guide Git settings changed unrelated identity or includes")
  }
  ```
- [ ] **Step 2: Run `go test ./internal/apply -run 'Git|Fisher|Plugins|Fish|ChezMoi|Adoption' -v`.** Expect missing policy handlers to fail.
- [ ] **Step 3: Implement the handlers and matching checks.** Git changes affect `core.editor`, `core.pager`, `interactive.diffFilter`, `delta.navigate`; account for existing includes and multi-valued keys, back up the affected local file, and reject a concurrent edit. Install Fisher before creating/restoring its chosen manifest. Review the actual tagged source during implementation; fetch a recorded revision without piping an unchecked response straight into the live shell. Fresh plugin setup is automatic; ambiguous existing functions/completions/conf.d conflicts become preservation/manual tasks rather than unconditional `fisher update`. Install only missing selected plugins and retain unrelated installed entries. For chezmoi, retain an existing source; use individual `add --follow --recursive=false` only after clean source-state checks, and show diff/status afterward. Initialization/adoption never creates a remote, authenticates, commits user dotfiles, or pushes them.
- [ ] **Step 4: Run `go test ./internal/apply ./internal/plan -v`.** Use fake Fisher/chezmoi for action tests and a temporary Git configuration for actual key preservation where Git is available. Expect selected files and checks to match the guide; no real plugin/network installation in tests.
- [ ] **Step 5: Commit `feat: configure git and safely adopt selected shell settings`** with this task’s files.

## Milestone 3: optional tools, recovery preparation, and source startup

### Task 7: optional languages, workspaces, and recovery preparation

**Files:** Create `internal/plan/workspace.go`, `workspace_test.go`; `internal/apply/languages.go`, `recovery.go`, `recovery_test.go`; extend `internal/templates/assets/`, planner manual tasks, and action verification.

**Interfaces:** Consumes `Options.Workspaces`, language selections, reviewed templates, the runner and file manager. Produces `plan.WorkspaceChanges(domain.Host, domain.Workspace) ([]domain.FileChange, error)`, `apply.InstallLanguageTools(context.Context, domain.Host, []string, command.Runner) error`, and `apply.PrepareRecovery(context.Context, domain.Host, string, command.Runner) (string, error)`. Workspace planning uses the inspected before-state rather than reading or writing disk in the planner. The recovery argument is a new dated directory chosen under the user’s recovery folder; the result names the generated full inventory.

- [ ] **Step 1: Write `TestLanguageSelectionIsIndependent`, `TestWorkspaceRequiresRealPathAndName`, `TestWorkspacePreservesExistingProject`, `TestWorkspacePathsAreArguments`, `TestNimBinaryAndFormatter`, `TestInventoryNeverOverwritesCuratedBrewfile`, and `TestNativeExportRemainsSeparate`.** Assert no unselected toolchains, no placeholder module or arbitrary workspace, protected existing `.zed` files, correctly handled spaces/Unicode, Nim binary manifest and `--stdin` formatter, a dated complete inventory, and no apps-only replacement of `~/.Brewfile`.

  Assertions in `TestInventoryNeverOverwritesCuratedBrewfile`:
  ```go
  if !bytes.Equal(curatedBefore, curatedAfter) || inventoryPath == curatedPath {
      t.Fatal("dated inventory overwrote the curated Brewfile")
  }
  ```
- [ ] **Step 2: Run `go test ./internal/plan ./internal/apply -run 'Language|Workspace|Nim|Inventory|NativeExport' -v`.** Expect the optional/recovery behavior to fail before implementation.
- [ ] **Step 3: Implement language choices and workspace planning.** Go uses `go`, `golangci-lint`, `delve`, and `go install golang.org/x/tools/gopls@latest`; Crystal uses `crystal`, `crystalline`, `ameba`; Nim uses `nim` and a terminal-owned `nimble install nimlangserver`. Respect custom GOBIN/GOPATH/Nimble locations and verify the resulting server path. For optional new workspaces, generate a minimal runnable application and the reviewed `.zed` settings/tasks only after validating a user-selected unoccupied path and actual module/entry point. Use Go module initialization with argument arrays; Crystal’s source/target must agree; a Nim manifest must explicitly describe the intended binary, never rely on `nimble init -y` defaults. For existing projects, limit writes to individually reviewed `.zed` files. Validate entry-point interpolation rather than inserting shell metacharacters into saved tasks.
- [ ] **Step 4: Implement recovery preparation and finish instructions.** Run a complete Homebrew dump to a new dated path without `--force`; offer a separately reviewed full `~/.Brewfile` update and chezmoi adoption. Prepare the recovery folder and optional reviewed `.kopiaignore`. Include exact manual tasks for Applite’s shared path, native export/import and taps, Cork distribution/refresh, Zed CLI/extensions, Warp settings, GitHub sign-in, Time Machine encryption/migration/test restore, and optional Kopia access/repository/policy/test restore. Include the guide’s maintenance links. Never call cleanup, import the commented sample in Applite, or mark a data backup verified from an inventory. Run `go test ./internal/plan ./internal/apply -v`; expect all optional/recovery fixtures to pass.
- [ ] **Step 5: Commit `feat: add selected language workspaces and recovery records`** with this task’s files.

### Task 8: native source-build starter

**Files:** Create `scripts/bootstrap.zsh`, `scripts/bootstrap_test.go`; update `.gitignore` and `README.md` with the actual starter contract.

**Interfaces:** Consumes the compiled command entry point and pinned module files. Produces `zsh scripts/bootstrap.zsh --plan` (prerequisite preview), `zsh scripts/bootstrap.zsh --build-only` (accepted prerequisite setup/build without running the wizard), and default startup (build, then launch). Flags after `--` are passed to the wizard as an argument array. Build output is `bin/golden-setup`; there is no assumed hosted binary.

- [ ] **Step 1: Write `TestBootstrapPreviewNoMutation`, `TestBootstrapDirectoryWithSpaces`, `TestBootstrapMissingCLTPauses`, `TestBootstrapRequiresNativePrefix`, `TestBootstrapGoFloor`, and `TestBootstrapPassesArguments`.** Use fake native commands/PATH and temporary directories. Assert preview creates nothing, missing CLT waits for the system installation then supports rerun, Rosetta/conflicting prefix refuses apply, Go below 1.26.0 is not used, and arguments remain separate.

  Assertions in `TestBootstrapPreviewNoMutation`, for a compatible host missing prerequisites:
  ```go
  if exitCode != 0 || len(mutatingCalls) != 0 || !reflect.DeepEqual(before, after) {
      t.Fatal("prerequisite preview must succeed without installation or writes")
  }
  ```
- [ ] **Step 2: Run `go test ./scripts -v`.** Expect failure because the starter does not exist; do not bootstrap the actual Mac to make tests pass.
- [ ] **Step 3: Implement the Zsh starter.** Resolve its own directory safely with quoted paths, inspect native macOS/CLT/Homebrew/Go, print the proposed prerequisites, and require explicit confirmation before installing missing ones. Use the official Homebrew installer only for the missing native installation, record the fetched source revision/checksum, and give its system prompts exclusive terminal access. Recheck CLT, prefix, and Go version afterward; fail clearly rather than continuing after an incomplete installer. Build the local source using the pinned module files, without changing global shell startup files. Reuse satisfied prerequisites and do not change a working default login shell.
- [ ] **Step 4: Run `zsh -n scripts/bootstrap.zsh` and `go test ./scripts -v`.** Expect syntax success and all fake-command starter cases to pass. The real fresh-Mac prerequisite route stays unexecuted and is listed for manual acceptance.
- [ ] **Step 5: Commit `feat: add fresh-mac source build starter`** with this task’s files.

### Task 9: complete wiring, isolated end-to-end checks, and delivery

**Files:** Modify `cmd/golden-setup/main.go`, `README.md`, `docs/requirements.md`; create `internal/integration/setup_test.go`, `internal/testutil/cmd/fixture/main.go`, `scripts/pty-smoke.py`, `docs/manual-acceptance.md`, `docs/verification.md`.

**Interfaces:** Consumes all previous task interfaces. The final entry point exposes default interactive mode, `--accessible`, `--plan`, `--help`, and `--version`; applying requires the in-session accepted preview, and resuming still reinspects the host. `--version` identifies the local build and approved dependency family without pretending it is signed or published.

- [ ] **Step 1: Write `TestFreshSetupRepeatAndResume`, `TestMigratedSetupPreserved`, `TestOnlySelectedWorkspaces`, and `TestFinishReportsManualWork`.** In a temporary home with fake package/tool responses, apply selected changes, rerun inspection, and assert no duplicate writes/installations; interrupt midway and assert resumed checks determine remaining work. Migrated app/config fixtures remain intact. A report with uncompleted GUI/backups says manual tasks remain, never fully restored or backed up.

  Assertions in `TestFreshSetupRepeatAndResume`, with second-run mutating calls and a still-pending backup task:
  ```go
  if len(secondInstallCalls) != 0 || len(secondFileWrites) != 0 || len(report.ManualTasks) == 0 {
      t.Fatal("repeat setup duplicated changes or hid remaining manual work")
  }
  ```
- [ ] **Step 2: Run `go test ./internal/integration -v`.** Expect any missing handler or incorrect cross-component wiring to fail.
- [ ] **Step 3: Complete wiring and terminal smoke support.** Add a separate fixture entry point at `internal/testutil/cmd/fixture/main.go` for pseudo-terminal runs so smoke checks never reach real apply. Keep fake-host behavior out of the production executable. Exercise resize, submission, editing q, completed-summary exit, Ctrl+C/Escape, terminal restoration, plain sequential answers, and EOF in both themes. Publish only actual running instructions and actual supported behavior in the README; mark unfinished manual checks explicitly.
- [ ] **Step 4: Run the required verification once the final code is in place.** Run `gofmt -l cmd internal scripts`; `go vet ./...`; `go build -o bin/golden-setup ./cmd/golden-setup`; `go build -o bin/golden-setup-fixture ./internal/testutil/cmd/fixture`; `go test -race -count=1 ./...`; `zsh -n scripts/bootstrap.zsh`; and `python3 scripts/pty-smoke.py --fixture bin/golden-setup-fixture`. Expected: no formatting findings, successful static checks/build, passing behavior/race/starter cases, and successful recorded smoke cases. Store concise evidence and toolchain versions in `docs/verification.md`; failures require fixes before a completion claim. Screen-reader use, actual GUI behavior, clean macOS installation, and real backup retrieval remain unchecked manual items in `docs/manual-acceptance.md`.
- [ ] **Step 5: Review the whole implementation against the approved spec and plan.** Resolve material findings, rerun affected checks, then commit `feat: deliver verified first-time setup assistant`. Show the local project and preview to Evelyn; running the real installation on this Mac is a separate action requiring that actual plan’s acceptance.

## Plan self-review and handoff

Coverage: Tasks 1–3 deliver the selection/preview milestone and the PDF’s interface contracts. Tasks 4–6 implement preservation, apply/verify/resume, Git, plugins, and chezmoi. Tasks 7–8 supply optional development tools, recovery records, native source startup, and guided application/backup work. Task 9 provides cross-component evidence and explicit manual acceptance limits. The five Review Focus conditions each have named owning tests.

Exact implementation-time values still requiring source verification are Fisher’s pinned bootstrap revision, optional language-server resolved versions, and the official Homebrew installer revision. They are recorded during the owning task rather than fabricated in this document; they do not change the approved interface or product scope. Do not reuse the PDF’s or earlier guide’s test results as this product’s verification.

Before implementation, Evelyn reviews this plan and chooses direct implementation here or task-by-task agent implementation/review. Direct implementation is recommended because the components share tightly related plan and execution types and benefit from continuous integration. Both methods finish with a whole-change review; choose reviewers using the active tool/model allowlist at execution time. Do not dispatch workers during planning.
