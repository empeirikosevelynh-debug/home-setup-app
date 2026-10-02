# Contributing

Develop on macOS with Go 1.26+. Run README verification before submitting changes. CI checks the minimum/current compiler families without applying setup to its runner.

Keep inspection read-only, planning deterministic for a snapshot, and execution dependent on fresh acceptance. Put system calls behind the runner and use temporary homes with explicit fake responses. Never test by applying setup to a developer account. The separate fixture exercises terminal behavior; no fake-host switch is linked into production.

Preservation/cancellation changes require behavior tests: declined replacements, source conflicts, symlink ancestors, registrations changing after preview, interrupted work, exclusive terminal ownership, and repeat runs. Always report remaining manual work separately from automated completion.

Update asset provenance when templates change. Installer/bootstrap revisions require source review, checksum updates, and retained notices. Dependency changes require the complete race/terminal checks and a vulnerability scan.

Record real-Mac acceptance separately from fixture success. Follow [release packaging](docs/releasing.md). Contributions are provided under this repository's MIT license.
