# Source review and verification limits

Historical design record: September 29, 2026. Product implementation and current evidence are in [verification](verification.md); statements about missing code below describe this earlier project start.

## Reference material

- [Terminal-application PDF](~/Desktop/guide-refined.pdf): all 14 pages were extracted and read, including the complete sample. Pages covering architecture, resizing, palette, and verification were also inspected as rendered pages.
- [Current setup guide](../../warp-fish-zed-setup-guide.md): the functional source for this project, including Applite/Cork and native macOS recovery.
- [Configuration examples](../../warp-fish-zed-config/README.md): reviewed starter configurations, with optional choices retained.
- [App recovery inventory](../../warp-fish-zed-config/app-recovery-inventory.md): observed applications and candidate routes; never an unconditional package list.

Exact file sizes and SHA-256 hashes are recorded in [source-fingerprints.json](source-fingerprints.json). These paths identify the reviewed originals; they are development references rather than runtime requirements for the future installer. The original documents and configuration files have not been modified.

## Correctness findings used in the design

1. The PDF specifies Charm v2, and its sample is a form demonstration. It does not implement package installation, configuration preservation, backup setup, or recovery. Those require product-specific implementation and tests.
2. The tagged module manifests confirm the PDF's module paths and compatibility requirement: [Bubble Tea v2.0.10](https://github.com/charmbracelet/bubbletea/blob/v2.0.10/go.mod) declares Go 1.26.0; [Huh v2.0.3](https://github.com/charmbracelet/huh/blob/v2.0.3/go.mod) declares Go 1.25.8 and Charm v2 dependencies; [Lip Gloss v2.0.6](https://github.com/charmbracelet/lipgloss/blob/v2.0.6/go.mod) declares Go 1.25.0. The higher direct pins must still be resolved and tested in this product's eventual module graph.
3. A clean Mac cannot be assumed to contain Go or Homebrew. The source-build starter therefore has its own prerequisite preview and system-prompt handling. No prebuilt or signed release currently exists for this project.
4. The guide's Apple-silicon prefix is `/opt/homebrew`. Applite 1.4.2 can detect /opt/homebrew on first setup; an existing private selection can remain active, and changing its selected path does not migrate old registrations. Shared-path selection needs verification before imports. See the tagged [Applite Homebrew paths](https://github.com/milanvarady/Applite/blob/v1.4.2/Applite/Core/Brew/BrewPaths.swift).
5. Applite's native app list and the complete Brewfile serve different purposes. Its parser can recognize commented cask declarations; its batch imports can replace existing bundles. Preserve the starter's optional choices and the distribution channel of existing apps. See [native migration](https://github.com/milanvarady/Applite/blob/v1.4.2/Applite/Features/AppMigration/AppMigration.swift) and [installation service](https://github.com/milanvarady/Applite/blob/v1.4.2/Applite/Core/Brew/BrewService.swift).
6. A reinstall list is not a data backup. Time Machine migration precedes rebuilding a migrated user's environment; personal-file recovery, permissions, credentials, and test restores remain visible manual tasks. See [Apple's Time Machine restore instructions](https://support.apple.com/en-us/102551).
7. Chezmoi add/re-add can replace pending source content. Existing repositories need inspection before adoption. See [chezmoi add](https://www.chezmoi.io/reference/commands/add/) and [re-add](https://www.chezmoi.io/reference/commands/re-add/).

## Checks completed for this project start

The local system reports macOS 27.0.1, build 26A434. `/opt/homebrew` exists. No `go` executable was found on the current PATH; this is a prerequisite observation rather than proof that no Go installation exists anywhere on disk.

The source PDF and current guide were reviewed, the dependency minimum was checked against tagged upstream manifests, and the proposal maps the guide's functional sections and the PDF's interface contracts. Project documentation links and reference fingerprints are checked in the saved design-review record.

No executable starter, Go module, or installer code has been created. No product dependencies, application installations, live settings changes, authentication, backup configuration, migrations, or recovery operations were run. The PDF's reference-program test results and the guide's earlier configuration checks are not this project's test results.

## Required later validation

Implementation must supply fresh evidence for planning, selected installation, preserving existing files, cancellation/resume, accessible prompts, terminal dimensions, and truthful reports. The build and behavior checks in the design must pass before claiming the installer works. A clean-Mac exercise, actual Warp/Zed behavior, screen-reader use, and real backup retrieval remain distinct manual acceptance checks.
