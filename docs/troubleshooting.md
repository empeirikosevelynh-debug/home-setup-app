# Troubleshooting

**Missing prerequisites:** inspect with the starter's `--plan`. Complete Apple's pending developer-tools installer before rerunning. Unsupported architecture, OS major, Rosetta, or prefix requires a compatible native environment.

**State changed after preview:** inspect again, review new diffs, and accept a fresh plan. Do not edit saved records to force approval.

**Interrupted run:** read the summary/session path, restart, restore choices if offered, and review. Completed installs/writes remain; cancellation is not rollback. Existing registrations/matching files are skipped. Use reported backups to deliberately restore earlier files if needed.

**Another setup holds the lock:** close it and retry. Exiting releases the lock automatically; the remaining `apply.lock` file is normal. Do not delete it while setup runs. Other Homebrew clients do not share this application lock; stop their active operations too.

**Symlink or oversized configuration:** automatic writing rejects redirected paths, nonregular files, and configuration larger than 1 MiB. Choose a real workspace location or merge manually. Only verified macOS `/var` and `/tmp` aliases are accepted.

**Preserved Git settings:** a symlinked Git configuration, or an included file that sets the editor, pager, or delta options differently, stays as it is and becomes a manual task. Set those values where you manage them.

**Preserved plugins/chezmoi:** custom functions/manifests, legacy state, dirty sources, and existing managed templates require manual review. A source outside the home receives a manual adoption task.

**Custom XDG/tap:** reconcile the custom configuration location or package substitution before applying standard templates. Do not unset a variable merely to hide settings you still use.

**Unreadable saved record:** setup names the file in a notice and continues with default choices. Preserve a copy, inspect the named JSON/permissions, and move the damaged record out of sessions only after review. Review private contents before sharing.

**Partial project:** a nonempty directory is existing user source, even after an interrupted creation. Inspect its manifest/source and complete missing files manually before the project task.

**Partial recovery folder:** export refuses overwrite. Inspect/archive the reported partial folder and retry a freshly accepted plan, preserving useful exports first.

**Plain-input failure:** each answer needs a newline. Installation needs an interactive terminal; buffered pipes cannot transfer ownership for password prompts. Plain prompts do not by themselves prove VoiceOver usability.

## Windows preview

**"Run setup from a terminal opened with Run as administrator":** Chocolatey installs for the whole PC. Open Warp or PowerShell with Run as administrator and inspect again.

**A tool installed during setup is not found:** installers update the saved PATH, not open windows. Setup also searches the saved PATH and Chocolatey's `bin` folder; open a new terminal before using the tools yourself.

**Weaker file checks than macOS:** Windows has no equivalent of the descriptor-based writes used on macOS. Setup refuses links and junctions on the way to every file and checks again just before replacing it, but a link created in between could still redirect a write. Run setup when nothing else is changing your configuration folders.

**Private files:** Windows does not use permission bits. Sessions and backups live in `%LOCALAPPDATA%\Golden Gate Setup` and rely on your profile's default access, which is limited to you and administrators.

**Delve and Crystal:** Chocolatey has no package for either. The summary lists `go install` for Delve; Crystal is not offered on Windows.

**Different app inventories:** confirm executable paths, refresh/relaunch, and remember that changing prefixes does not migrate registrations. Vendor/App Store apps retain their channels. See [recovery](recovery.md).
