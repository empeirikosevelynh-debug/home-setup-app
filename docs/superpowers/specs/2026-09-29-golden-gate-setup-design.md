# Golden Gate Setup: design proposal

Date: September 29, 2026. Status: approved by Evelyn on September 29, 2026. Implementation has not started; the written implementation plan still requires review.

## Purpose and sources

Create a first-time installer for the environment described in `warp-fish-zed-setup-guide.md`: Warp, Fish, Zed, the supporting command-line tools, optional language workspaces, shared graphical app management, managed configuration, and recovery preparation.

The setup guide defines the installation behavior. `guide-refined.pdf` defines the Go terminal-interface architecture and presentation. Its workspace-name and beverage demonstration is an example, not this product’s feature list. Evelyn’s request takes precedence over both documents.

The initial target is macOS 27 on Apple silicon with native Homebrew at `/opt/homebrew`. Detect the OS major version and hardware rather than matching a marketing name. Other OS versions, Intel Macs, Rosetta execution, or a conflicting Homebrew prefix must produce a clear compatibility result before applying changes.

## Recommended approach

Use a small native Zsh starter followed by a compiled Go/Charm wizard. On a fresh Mac, the starter can explain and arrange the missing Command Line Tools, Homebrew, and Go needed to build the wizard. A source build must clearly identify Go as an installer prerequisite even if the user does not select Go development tools. Homebrew and system authorization prompts must have exclusive terminal ownership. Command Line Tools installation can require a macOS dialog; detect completion before continuing and support rerunning the starter.

A shell-only installer would reduce build dependencies but would not supply the requested Charm interface. A native graphical application would require a different interface and a larger packaging effort. The Zsh-plus-Go approach satisfies both the fresh-Mac problem and the PDF. A downloadable, signed Apple-silicon executable can be a later distribution option; none is published or assumed by this proposal.

The initial source delivery will keep its starter, application, and reviewed configuration templates together. It must work from a directory containing spaces. The starter must support a preview of prerequisites before installing them and must not fetch and execute an unreviewed copy of this project from an invented release URL.

## User journey

1. **Inspect.** Identify the system, native Homebrew, registered packages, existing app bundles, configuration files, and any chezmoi repository. Explain whether this is fresh setup or an existing environment. If the user intends a Time Machine migration, direct them to complete it first and then rerun inspection.
2. **Choose.** Offer a recommended setup with individual choices for applications, plugins, and languages. Describe what each choice adds. Keep developer examples and additional plugins optional.
3. **Preview.** Show missing packages, settings to create or change, files to adopt into chezmoi, prerequisite prompts, and manual tasks. Existing files require a visible diff or a decision to preserve them. Nothing in this screen installs or writes configuration.
4. **Apply.** Run the accepted steps in dependency order with progress, readable status, and a view of diagnostic output. A failure stops dependent steps and leaves a useful recovery report.
5. **Verify.** Check the resulting tools and configuration, then present application, authentication, and backup tasks that still need a person. Distinguish “automated setup verified” from “manual tasks remaining.”

Provide the same decisions in a plain `--accessible` mode. Provide a machine-readable `--plan` mode for inspection and a saved final report; the plan mode must never modify the host. Applying changes always requires an accepted plan. A later noninteractive apply mode can be considered separately.

## Installation choices

The core preset includes `fish`, `starship`, `zoxide`, `chezmoi`, `gh`, `fzf`, `fd`, `bat`, `eza`, `ripgrep`, `git-delta`, and `lazygit`. Warp, Zed, and Applite are recommended application choices, visible and deselectable. GitHub Desktop and KopiaUI are optional. Cork is obtained through its official distribution or source build; do not invent a Homebrew cask or bypass its license.

Offer Fisher and the guide’s four starting plugins as a selectable group. Offer each extra plugin separately. The reference `fish_plugins` includes alternatives and must not become an unconditional install list. Record the chosen plugin selection and any resolved revisions; explain that unpinned plugins and Homebrew inventories do not lock all versions.

Go, Crystal, and Nim development support is optional. Install their selected toolchains and language servers using the guide’s routes. Creating an example workspace requires a user-selected directory and, where applicable, a real module name or entry point. Preview its files and preserve an existing project. Do not create demonstration projects in arbitrary folders.

Probe already installed packages before each step. Skip satisfied selections, including apps restored by Migration Assistant. Do not upgrade or reinstall them merely to make an installation list match. If a required minimum version is unmet, show the needed update in the plan. Keep vendor-installed and App Store apps on their existing channels unless the user explicitly chooses adoption or replacement for that app.

## Configuration and preservation

Bundle reviewed copies of the guide’s Fish, Starship, Zed, lazygit, and workspace examples when implementation starts. Record their source fingerprints. Keep the project self-contained instead of depending at runtime on Evelyn’s original folders or parsing Markdown code blocks as commands.

For missing files, create the chosen configuration. For identical files, do nothing. For different files, offer preservation or a reviewed change. The initial implementation can decline automatic merging where it cannot preserve comments and unrelated settings reliably. In particular, Zed settings may contain JSON comments; do not use a parser that silently strips a user’s comments. Treat symlinks as an explicit conflict unless their target and write policy have been reviewed.

Back up every existing file before an approved change, preserve relevant permissions, write atomically, and record the destination and backup location. Refuse unsafe paths or unexpected file types. Recheck the file after approval so a concurrent application edit is not silently overwritten.

Apply the guide’s Git editor and delta settings only after showing the affected keys. Preserve Git identity, credentials, unrelated configuration, and existing includes. Keep login-shell changes optional. Warp preferences, Zed’s CLI installation and extensions, and terminal shortcut checks remain guided tasks unless a documented, supported automation route is verified.

Initialize chezmoi only when there is no existing source repository. Adopt only the individually reviewed configuration files and selected plugin manifest. Inspect existing source changes before add/re-add operations, which can replace source edits. Repository creation, authentication, commits, and remote uploads are separate user actions; adoption alone is not an off-device backup.

## Applite, Cork, and recovery records

Use `/opt/homebrew/bin/brew` consistently, with apps in `/Applications`. Applite normally uses a private Homebrew installation, so the finish checklist must require selection of **Apple Silicon Mac** in its Brew settings before any import or app installation. Preserve an existing private installation and explain that changing the selected path does not migrate its registrations. Cork and Applite should show packages from the shared installation after refresh.

Do not blindly set undocumented application preferences. For the initial version, shared-path selection and verification are guided checkpoints. Automatic preference adapters would require separate, version-specific verification.

Maintain two distinct recovery records: the complete Homebrew inventory and Applite’s selected-app native export. Generate package inventories into a dated recovery location and never overwrite a curated `~/.Brewfile` or replace it with an apps-only list. Offer creation or a reviewed update of `~/.Brewfile`, with chezmoi adoption as a separate choice.

Guide **App Migration → Export Apps…**, saving the native file in the recovery folder. Its list omits Applite itself and does not contain application data, licenses, or passwords. On restoration, restore any required taps before importing, use the shared Homebrew, and deselect apps already restored. Applite 1.4.2 can recognize commented cask declarations and its batch import uses replacement behavior, so the bundled commented starter Brewfile is not a native-import artifact. The observed app inventory is a checklist, not an automatic install or adoption manifest.

The finish flow explains Time Machine, Setup Assistant/Migration Assistant, and optional Kopia backups. It can prepare the recovery folder and offer the guide’s conservative `.kopiaignore` after review. Disk selection, encryption passwords, Full Disk Access, repository credentials, snapshot policy, and representative test restores remain user-controlled. Report backup setup as unverified until those checks are completed. The installer never erases disks, reinstalls macOS, or restores snapshots over the active home directory. Its optional import copies chosen folders from a previous home folder without replacing or deleting anything; differing files are saved in a dated conflicts folder.

## Application architecture

Separate the host inspector, declarative catalog, planner, executor, file manager, verification checks, and presentation. The planner produces typed steps with dependencies, current and desired state, commands or file diffs, and verification criteria. Both interactive and plain modes use that same plan. Use argument arrays for subprocesses rather than constructing shell commands from user text.

Use a pointer-based Bubble Tea v2 model, stable Huh field bindings, `tea.KeyPressMsg`, and `tea.View` with declarative alternate-screen behavior. Run disk, network, and subprocess work in `tea.Cmd` functions that return typed messages. Preserve commands returned by embedded forms. Only one component owns the terminal; pause or release the parent interface for any genuine interactive subprocess. Keep credentials out of saved reports.

Record step state in a private, versioned local session file only after verification. Reruns inspect current state rather than trusting an old checkpoint. Cancellation stops new work; request a safe stop of the active child and wait for cleanup where necessary. Show an interrupted step honestly and probe it on resume. Installed packages are not automatically rolled back. Print diagnostics after restoring the terminal, use stderr for failures, and distinguish cancellation, failure, completed checks, and pending manual tasks.

## Interface contract from the PDF

Pin `charm.land/bubbletea/v2` at `v2.0.10`, `charm.land/lipgloss/v2` at `v2.0.6`, and `charm.land/huh/v2` at `v2.0.3`. Bubble Tea requires Go 1.26.0 or later. Record direct and transitive dependencies in the module files during implementation; verify against a supported Go toolchain instead of assuming the PDF’s test results apply to this product.

Use rounded cards, the PDF’s explicit light/dark palette, and labels that remain meaningful without color. Cap the outer card at 72 columns. Full interaction begins at 48 × 18; smaller terminals show a bounded resize notice and pause hidden input. A zero-sized terminal renders nothing. Measure terminal cell width and height, budget help and validation rows, and use scrolling for long lists and output. No hand-written terminal escape sequences.

Ctrl+C and Escape request exit, with active execution governed by the cancellation rule above. `q` remains normal text while editing and can exit the completed summary. Plain mode is selected before starting the parent interface, uses an uncolored theme, preserves successive piped answers, and rejects incomplete input. Screen-reader usability must be tested separately; library support alone is not a verified accessibility result.

## Acceptance criteria and verification boundary

- A preview records proposed changes without installing packages, writing user files, or changing repositories.
- A fresh temporary home receives only selected configuration; an unchanged second run proposes no duplicate edits.
- Existing settings, symlinks, curated inventories, and pending chezmoi edits are preserved unless a specific change was accepted.
- Failed commands, interrupted steps, and changed files are never reported as successful; resuming rechecks actual state.
- The Applite/Cork workflow uses the shared Homebrew and keeps native app selections separate from complete inventories and data backups.
- Optional languages and workspaces do not appear unless selected. Remaining GUI, authentication, and backup work is visible in the final report.
- Run meaningful planner/executor tests using fake commands and temporary homes, then formatting, static checks, build, and race tests. Exercise light/dark layouts at the PDF’s seven sizes, long labels, editing `q`, resize input suppression, cancellation, plain-mode answer separation, EOF, and terminal restoration in a pseudo-terminal.

These checks cannot prove a clean macOS installation, actual Warp/Zed interaction, successful Time Machine/Kopia recovery, or screen-reader usability. Record those as separate manual acceptance checks. During project development, do not run the setup against Evelyn’s live home directory merely to validate the installer.

## Review decision

Evelyn approved this design and preparation of the implementation plan on September 29, 2026. The first milestone is a functioning preview and selection interface with isolated tests; package and configuration application follows with preservation and resume checks. No product code, executable starter, or dependencies have been created at this stage.
