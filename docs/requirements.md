# Requirements checklist

Status: implemented release candidate 0.1.0-rc.1. Automated evidence is recorded in [verification](verification.md); hardware, GUI, VoiceOver, and data restore remain in [manual acceptance](manual-acceptance.md). Source section names refer to the current Warp + Fish + Zed guide. PDF page numbers refer to the 14-page `guide-refined.pdf`.

## Setup-guide coverage

| Source | Intended automated work | Guided or conditional work |
|---|---|---|
| Before you begin; 1. Install the tools | Inspect macOS/architecture/prefix; prepare a selected package plan; install missing core tools | Command Line Tools and Homebrew system prompts; Nerd Font choice |
| 2. Configure Fish | Preview and install the reviewed Fish configuration; verify syntax and paths | Resolve existing-file conflicts; optional login-shell change |
| 3. Configure Starship | Preview and install the TOML; verify availability | Confirm prompt in actual terminal sessions |
| 4. Connect Warp, Zed, and Git | Select Warp/Zed; preview user settings and Git keys; optional lazygit configuration | Warp preferences; Zed CLI/extensions; authentication; editor and shortcut checks |
| 5. Choose Fish plugins | Install selected Fisher/plugins in the correct order; record the chosen manifest | Extra plugins are individual choices; verify terminal-specific typing and bindings |
| 6. Manage configuration with chezmoi | Inspect/init an appropriate local source; adopt only selected files; query status without storing private file diffs; clone a chosen dotfiles repository and restore its plain files through the file review, without storing their contents | Existing source conflicts; GitHub sign-in; remote selection, commit, push; templates, encrypted files and scripts through `chezmoi apply` |
| 7. Manage apps with Applite and Cork | Install selected available casks; inspect shared registrations; generate a dated full inventory; reinstall chosen missing entries from a previous Mac's inventory | Cork distribution/license; Applite shared-path setting; native export/import; app-specific adoption decisions; App Store and other inventory lines |
| 8. Back up and recover | Prepare recovery folder; optionally install KopiaUI; preview conservative exclusions; copy chosen folders from a previous home folder without replacing or deleting anything | Time Machine disk/encryption; migration before rebuild; Full Disk Access; repository/schedule; test restores; comparing saved conflicts; iCloud-only files |
| 9. Daily reference | Link the relevant maintenance instructions in the finish report | Later daily operations are outside a first-time installation run |
| 10. Go, Crystal, and Nim workspaces | Install selected toolchains/servers; optionally create reviewed workspace files | Actual project names/entry points; extension installation; real project tests |
| Check the setup | Check installed commands, package prefix, file syntax, and selected toolchain availability | Warp/Zed behavior, GitHub, shared GUI inventory refresh, backup retrieval |

## PDF contracts

| PDF pages | Project requirement | Evidence required during implementation |
|---|---|---|
| 1 | User request defines product behavior; sample remains a reference | Installer decisions match the setup guide; no beverage/name demo presented as product functionality |
| 2 | Pinned Charm v2 dependencies; Go minimum from Bubble Tea | Module files, verified dependency graph, successful build |
| 3 | Stable model bindings, asynchronous work, one terminal owner | Typed result messages; form lifecycle and subprocess ownership tests |
| 4 | Bounded resizing, keyboard policy, meaningful plain mode | Seven terminal sizes; hidden input paused; piped answers and EOF checks |
| 5 | Explicit readable light/dark palette and shape/text status cues | Light/dark visual checks; usable labels without color |
| 6 | Correct Huh integration and fresh form state when editing | Preserve returned commands; cancel does not apply draft selections |
| 7–12 | Working reference, adapted to installer domain | Product-specific implementation and tests rather than assumed sample coverage |
| 13 | Evidence before completion claims | Formatting, static checks, build, race/behavior tests, pseudo-terminal checks; separate manual acceptance record |
| 14 | Primary-source API review | Verify referenced tagged source when implementing and record any dependency changes |

## Required preservation cases

- Existing vendor and App Store app bundles stay intact unless that specific app is deliberately adopted or replaced.
- Existing Fish, Zed, Git, lazygit, Starship, and backup settings survive a declined change.
- An existing chezmoi repository and its pending edits are inspected before importing live files.
- A full or curated Brewfile survives native Applite export and inventory generation.
- The commented example Brewfile and observed app checklist are never treated as unconditional native-import or installation manifests.
- Importing from a previous home folder never replaces, merges into or deletes an existing file, folder bundle or link target; differing files are saved in a dated conflicts folder, and a repeated import saves nothing twice.
- A chezmoi source holding other dotfiles is never replaced, and files a chosen dotfiles repository manages are not copied by the folder import.
- A failed or canceled run leaves a truthful report and enough state to inspect and resume.
- Previewing, testing, and developing the project do not apply the setup to the current Mac.
