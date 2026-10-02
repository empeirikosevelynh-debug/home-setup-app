# Security

Run as the current user, not root. Individual package installers may request authorization through their foreground prompt.

An accepted plan has exact changes and a snapshot fingerprint. Execution re-inspects, rechecks actions, backs up replacements, rejects symlink traversal, and holds a per-account installer lock. Saved records cannot grant approval. Automatic chezmoi hooks and Git actions are disabled during adoption. Setup stores no account credentials.

Private sessions include choices, paths, proposed configuration, and results. Backups can include secrets from the original file. Review them before sharing. Diagnostics are bounded and remove controls/common credential forms, but cannot detect every secret.

A clean Go vulnerability scan does not verify all later-downloaded packages, plugins, or apps. Those follow their selected publishers' distribution.

Report suspected vulnerabilities privately to the project owner. When hosting is configured, use its private vulnerability-reporting feature. This local project has no invented support address or reporting endpoint. Avoid posting private session contents publicly.

Current artifacts are unsigned and not notarized. Do not disable Gatekeeper globally. Signed distribution requires the maintainer's Developer ID, notarization, and separate fresh-Mac acceptance.
