# Changelog

## Unreleased

- Fisher plugin state is saved and detected: plugins install with fish's universal variables enabled, and existing state is read from `fish_variables`.
- A symlinked Git configuration, or an include that overrides the Git settings, becomes a manual task instead of stopping the apply.
- Command output keeps non-ASCII text intact; failures report the last output line, and long output shows its final lines.
- Unreadable saved sessions, invalid workspace choices, and plan errors no longer end the wizard.
- The apply screen follows new output; file replacements are asked again after edits; restored choices use today's recovery date.
- Plain mode no longer defaults project names to `none`.
- Steps that are already satisfied no longer re-inspect the Mac, a vendor app found during apply is reported once, and session saves no longer accumulate backups.
- Build outputs and local machine files are no longer tracked.
- The repository opens in Zed with project settings, tasks and debug setups. The installer's workspace templates are stored outside `.zed/` folders, so Zed no longer loads them as live configuration.

## 0.1.0-rc.1 — 2026-09-30

First complete release candidate: native macOS 27 inspection, selectable installation plan, configuration diff/backup, verified execution/resume, Fish/Git/chezmoi setup, optional Go/Crystal/Nim workspaces, recovery records, and Applite/Cork guidance. Includes themed/plain interfaces, isolated terminal fixtures, integration/race checks, source startup, CI, MIT licensing, and local release packaging.

Clean-Mac, actual GUI, VoiceOver, and data restore acceptance remain pending. Artifacts are unsigned and not notarized.
