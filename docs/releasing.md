# Local release procedure

Packaging creates artifacts without uploading, publishing, signing, or notarizing. `0.1.0-rc.1` identifies the complete implementation while hardware acceptance is pending.

Run README checks, formatting, and `go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./cmd/golden-setup`. Review [manual acceptance](manual-acceptance.md); stable release needs clean-Mac, GUI, VoiceOver, and data-restore evidence. Keep incomplete checks explicit for candidates.

Update version/changelog/docs and commit the reviewed tree. Packaging refuses uncommitted changes so binary and source match:

```sh
python3 scripts/release.py --go go --version 0.1.0-rc.1
```

A compiler path is accepted. Dependencies can download into the selected module cache. Inherited `GOFLAGS`/`GOWORK` are isolated; `GOTOOLCHAIN=local` avoids hidden compiler replacement. The target is Darwin arm64 with CGO disabled, readonly resolution, and trimmed paths.

The new `dist/0.1.0-rc.1/` contains native/source archives, BUILDINFO.json, and SHA256SUMS. Existing output directories are refused. Binary contents include documentation, MIT license, complete dependency texts, and compiler/commit/module provenance. Archive timestamps use commit time; this does not claim independent cross-toolchain reproducibility.

Run `shasum -a 256 -c SHA256SUMS` inside the output directory. Extract the native archive; check `./golden-setup --version`/`--help` against BUILDINFO. Preserve third-party notices with the binary. Do not apply setup on the maintainer's account just to verify packaging.

Public repository identity, support configuration, and signing credentials are not assumed. Choose actual hosting before offering `go install`/remote starter commands. Review historical provenance documents for local path metadata before public source publication. Signed distribution needs the maintainer's Developer ID, notarization, fresh checksums/metadata, and another acceptance run. External publication is a separate maintainer action.
