# Apps, configuration, and personal-data recovery

## Prepare now

Complete a Time Machine or Kopia backup and restore sample files elsewhere. Save the encryption/repository password somewhere available without this Mac. Inventory exports complement data backups and their credentials.

Recovery preparation creates a dated folder in `~/Golden Gate Recovery/`. With inventory enabled it contains `Homebrew-full.Brewfile` (taps and requested formulae/casks), `installed-apps.json` (all observed registrations, versions/request metadata, application bundles and App Store receipt indicators), and `Recovery-notes.txt` (remaining tasks). Homebrew's dump can omit dependency-only formulae; the JSON records them. Neither locks versions or saves app data.

The curated `~/.Brewfile` is preserved. Review its diff and backup before deliberately updating the desired long-term list, then adopt that reviewed file into chezmoi separately.

## Keep Applite and Cork together

In **Applite Settings → Brew Executable Path**, confirm **Apple Silicon Mac (`/opt/homebrew/bin/brew`)**. Select it if needed, then Refresh Catalog or relaunch. Fresh Applite can detect this prefix automatically; existing users may retain a private installation. Export its old app list first. Switching paths does not migrate registrations or data. Verified in [Applite 1.4.2 bootstrap](https://github.com/milanvarady/Applite/blob/v1.4.2/Applite/Core/Brew/Installation/HomebrewBootstrap.swift) and [path selector](https://github.com/milanvarady/Applite/blob/v1.4.2/Applite/Components/BrewPathSelector/BrewPathSelectorView.swift).

In **Cork Settings → Homebrew**, confirm `/opt/homebrew/bin/brew`. It selects this standard prefix when present unless previously customized. The standard path does not need developer settings. [Cork 2.0.2 path selection](https://github.com/buresdv/Cork/blob/v2.0.2/Modules/Shared/App%20Constants.swift).

Use one package client at a time, then refresh both. A cask registered through either client joins the same shared inventory. Vendor/App Store apps can remain outside it; an Applications icon is not a Homebrew registration. Setup preserves those channels.

In **App Migration → Export Apps…**, select desired apps and save the dated file in the recovery folder. Applite's export includes selected casks/non-default taps; it excludes Applite itself, formulae, app data/settings, and exact installed versions. [Released export implementation](https://github.com/milanvarady/Applite/blob/v1.4.2/Applite/Features/AppMigration/AppMigration.swift).

## After reinstalling

1. Migrate an existing account with Setup Assistant/Migration Assistant before rebuilding it. [Apple recovery instructions](https://support.apple.com/en-us/102551). To start fresh instead, restore or connect the old home folder (an external drive, a share, a Kopia restore, or a mounted Time Machine backup you browse to) and let setup bring over chosen folders, your dotfiles repository and the previous app list; see [Bring over your previous Mac](../README.md#bring-over-your-previous-mac).
2. Open restored files and verify retrieval. Reconnect accounts, licenses, and backup credentials. Compare anything setup saved in `~/Imported conflicts/`.
3. Inspect setup first; saved choices require fresh approval and local replacement reviews.
4. Confirm both clients' shared prefix. Restore needed taps; import only missing apps from Applite's native export. Keep existing vendor/App Store channels. Do not import the commented example Brewfile as a native app list.
5. Review Homebrew records before reinstalling. `brew bundle` can upgrade by default; inventory is not a frozen environment. [Bundle behavior](https://docs.brew.sh/Brew-Bundle-and-Brewfile). Setup's previous app list installs only the entries you tick that are missing, and never upgrades.
6. Review chezmoi source/diff before restoring configuration. Setup restores a chosen repository's plain files through its review and leaves templates, encrypted files and scripts for `chezmoi diff` and `chezmoi apply`. Local adoption alone creates no remote backup; review, commit, connect your chosen remote, and push separately.
7. Finish terminal/editor preferences, CLI/extensions, fonts, authentication, and actual project tasks.
8. Reconnect and test Time Machine/Kopia, including another separate-folder restore.

Kopia's repository belongs outside the snapshot source. Grant required Full Disk Access, keep its password off the Mac, and review exclusions/errors including cloud-only files. [Kopia setup](https://kopia.io/docs/getting-started/).
