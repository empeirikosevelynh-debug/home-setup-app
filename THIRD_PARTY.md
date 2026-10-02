# Third-party notices

The application's MIT license does not replace dependency, runtime, or embedded-source licenses.

Go's standard library and the modules pinned in `go.mod`/`go.sum` include Bubble Tea 2.0.10, Huh 2.0.3, Lip Gloss 2.0.6, Charm support libraries, and `golang.org/x/sys`. Their complete original license/copyright texts are collected from downloaded sources in **THIRD_PARTY_LICENSES.txt** in every binary archive. Packaging fails if a dependency lacks an identifiable license. **BUILDINFO.json** records resolved versions and checksums. Go runtime license/patents notices are included where present.

Embedded Fisher 4.4.8: commit `a04308be92daa6cfecdbb0ca58b1e8508664cff2`, SHA-256 `0fb6c81ae3003e95b5671766fa6c25c3597066e29965b7772f6c1b007387356d`. Its source notice and [MIT license](internal/apply/assets/FISHER-LICENSE.md) are retained. [Upstream source](https://github.com/jorgebucaran/fisher/tree/a04308be92daa6cfecdbb0ca58b1e8508664cff2).

The starter downloads Homebrew's official installer only when needed and confirmed: commit `04dfcac13ead62adfc864260c5d5f9404d145d50`, SHA-256 `fa4ed743b4ca38316c8f32fd6623baa5bbb928fd4a47ad9f49c2be78ea833449`. It is verified before execution and is not bundled. [Upstream source/license](https://github.com/Homebrew/install/tree/04dfcac13ead62adfc864260c5d5f9404d145d50).

Applications, fonts, language tools, and optional plugins are requested from their publishers rather than bundled. Their own terms apply, including Cork's distribution/license requirements.
