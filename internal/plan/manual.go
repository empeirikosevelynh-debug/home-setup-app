package plan

import "golden-gate-setup/internal/domain"

func FinishTasks(o domain.Options) []domain.ManualTask {
	tasks := []domain.ManualTask{
		{ID: "font", Title: "Choose a terminal font", Instructions: "Choose and install a Nerd Font if you want the supplied prompt and file icons, then select it in your terminal. Test glyphs at a comfortable size.", URL: "https://www.nerdfonts.com/", Required: false},
		{ID: "github-auth", Title: "Connect your GitHub account", Instructions: "Use gh auth login with HTTPS and the browser, then gh auth setup-git; check authentication. Credentials are not managed by this installer.", URL: "https://cli.github.com/manual/gh_auth_login", Required: true},
	}
	if Has(o.Apps, "warp") {
		tasks = append(tasks, domain.ManualTask{ID: "warp", Title: "Connect Warp to Fish", Instructions: "In Warp settings choose Fish as the startup shell, Shell/PS1 for the prompt, and Zed for file links. Open a fresh tab and check shortcuts.", URL: "https://docs.warp.dev/terminal/shells", Required: true})
	}
	if Has(o.Apps, "zed") || o.ConfigureGit {
		tasks = append(tasks, domain.ManualTask{ID: "zed", Title: "Finish Zed setup", Instructions: "Run cli: install cli binary in Zed if needed; install Fish and any selected language extensions. Test zed --wait and a fresh Fish terminal.", URL: "https://zed.dev/docs", Required: true})
	}
	if Has(o.Apps, "applite") {
		tasks = append(tasks, domain.ManualTask{ID: "applite-shared-brew", Title: "Connect Applite and Cork to shared Homebrew", Instructions: "Before installing or importing in Applite: Settings → Brew Executable Path → confirm Apple Silicon Mac (/opt/homebrew/bin/brew), then Refresh Catalog or relaunch. Keep /Applications. Export any private installation before switching; changing paths does not migrate registrations. Refresh both clients after changes.", URL: "https://applite.app/troubleshooting", Required: true}, domain.ManualTask{ID: "applite-export", Title: "Save Applite’s native app selection", Instructions: "App Migration → Export Apps…: choose registered apps and save the dated export in your recovery folder. It includes selected casks and non-default taps; it omits Applite itself, app data and exact versions. Restore taps first; import only missing apps. Do not import the commented starter Brewfile.", URL: "https://applite.app/faq", Required: true})
	}
	tasks = append(tasks, domain.ManualTask{ID: "cork", Title: "Review Cork’s shared inventory", Instructions: "Obtain Cork from its official licensed distribution or source. In Settings → Homebrew confirm /opt/homebrew/bin/brew, then refresh. Its standard location does not require developer settings. Use one package client at a time.", URL: "https://corkmac.app/", Required: false})
	if o.AdoptChezmoi {
		tasks = append(tasks, domain.ManualTask{ID: "dotfiles-remote", Title: "Save configuration off this Mac", Instructions: "Review chezmoi diff and its Git status, commit the chosen files, connect your chosen remote, and push. Adoption alone is not a remote backup.", URL: "https://www.chezmoi.io/quick-start/", Required: true})
	}
	if o.PrepareRecovery {
		tasks = append(tasks, domain.ManualTask{ID: "time-machine", Title: "Complete and test Time Machine backup", Instructions: "System Settings → General → Time Machine: choose an external volume and encryption. Keep its password available without this Mac. Verify a completed backup and restore sample files elsewhere. For account migration, use Setup Assistant/Migration Assistant before rebuilding the same user’s setup.", URL: "https://support.apple.com/en-us/102551", Required: true})
	}
	if Has(o.Apps, "kopiaui") {
		tasks = append(tasks, domain.ManualTask{ID: "kopia", Title: "Configure and test Kopia", Instructions: "Grant Full Disk Access and restart KopiaUI; choose a repository outside the source, save its password off the Mac, configure the home snapshot/policy, and test a separate-folder restore. Review errors and cloud-only files. Inventory capture is not a data backup.", URL: "https://kopia.io/docs/getting-started/", Required: true})
	}
	for _, lang := range o.Languages {
		if lang == "nim" {
			tasks = append(tasks, domain.ManualTask{ID: "nim-server-compiler", Title: "Review Nim language-server compiler requirements", Instructions: "nimlangserver 1.14.0 requires Nim 2.0.8 to build. Review Nimble's foreground compiler/dependency prompt; it may install that compiler in its managed area. Keep the Homebrew project compiler on its chosen version. Verify Nimble 0.16.1+ and nimsuggest --v3. If build fails, inspect the reported constraint before retrying.", URL: "https://nim-lang.github.io/langserver/", Required: true})
		}
		tasks = append(tasks, domain.ManualTask{ID: "language:" + lang, Title: "Verify " + lang + " in Zed", Instructions: "Install the language extension where required, confirm server startup, format real source, and run its project task.", URL: "https://zed.dev/docs/languages", Required: true})
	}
	tasks = append(tasks, domain.ManualTask{ID: "maintenance", Title: "Review ongoing updates", Instructions: "Use one Homebrew client at a time. Review outdated packages in Cork or Applite, refresh both after changes, and keep vendor/App Store apps on their chosen update channel. Refresh the dated inventories and native Applite export after meaningful changes; keep testing data restores.", URL: "https://docs.brew.sh/Manpage", Required: false})
	return tasks
}

// WindowsFinishTasks is the manual work after a Windows setup. Warp starts
// PowerShell; its profile is edited by hand until setup can review it.
func WindowsFinishTasks(o domain.Options) []domain.ManualTask {
	tasks := []domain.ManualTask{
		{ID: "font", Title: "Choose a terminal font", Instructions: "Choose and install a Nerd Font if you want the supplied prompt and file icons, then select it in your terminal. Test glyphs at a comfortable size.", URL: "https://www.nerdfonts.com/", Required: false},
		{ID: "github-auth", Title: "Connect your GitHub account", Instructions: "Use gh auth login with HTTPS and the browser, then gh auth setup-git; check authentication. Credentials are not managed by this installer.", URL: "https://cli.github.com/manual/gh_auth_login", Required: true},
	}
	if Has(o.Apps, "warp") {
		tasks = append(tasks, domain.ManualTask{ID: "warp", Title: "Set up PowerShell in Warp", Instructions: "Install PowerShell 7 if needed (choco install powershell-core) and choose it as Warp's startup shell. Run notepad $PROFILE and add these two lines: Invoke-Expression (&starship init powershell) and Invoke-Expression (& { (zoxide init powershell | Out-String) }). Open a new tab and check the prompt and the z command.", URL: "https://docs.warp.dev/terminal/shells", Required: true})
	}
	if Has(o.Apps, "zed") || o.ConfigureGit {
		tasks = append(tasks, domain.ManualTask{ID: "zed", Title: "Finish Zed setup", Instructions: "Open Zed once and install any selected language extensions. In a new terminal, check that zed --wait opens a file and waits; Git uses it as the editor.", URL: "https://zed.dev/docs", Required: true})
	}
	tasks = append(tasks, domain.ManualTask{ID: "chocolatey-updates", Title: "Review Chocolatey updates", Instructions: "Run choco outdated to review updates, and upgrade packages deliberately with choco upgrade followed by the package name. Chocolatey GUI (choco install chocolateygui) is an optional graphical view. Keep vendor and Microsoft Store apps on their own update channels.", URL: "https://docs.chocolatey.org/en-us/choco/commands/outdated/", Required: false})
	if o.AdoptChezmoi {
		tasks = append(tasks, domain.ManualTask{ID: "dotfiles-remote", Title: "Save configuration off this PC", Instructions: "Review chezmoi diff and its Git status, commit the chosen files, connect your chosen remote, and push. Adoption alone is not a remote backup.", URL: "https://www.chezmoi.io/quick-start/", Required: true})
	}
	tasks = append(tasks, domain.ManualTask{ID: "windows-backup", Title: "Back up this PC", Instructions: "Set up Windows Backup or File History with a drive or account you can reach without this PC, run a backup, and restore a sample file to another folder. Setup records are not a data backup.", Required: true})
	if Has(o.Apps, "kopiaui") {
		tasks = append(tasks, domain.ManualTask{ID: "kopia", Title: "Configure and test Kopia", Instructions: "Open KopiaUI, choose a repository outside the source, save its password off the PC, configure the home snapshot and policy, and test a separate-folder restore. Review errors and cloud-only files.", URL: "https://kopia.io/docs/getting-started/", Required: true})
	}
	for _, lang := range o.Languages {
		if lang == "go" {
			tasks = append(tasks, domain.ManualTask{ID: "delve", Title: "Install the Go debugger", Instructions: "Chocolatey has no Delve package. Run go install github.com/go-delve/delve/cmd/dlv@latest, then check dlv version in a new terminal.", URL: "https://github.com/go-delve/delve/tree/master/Documentation/installation", Required: false})
		}
		if lang == "nim" {
			tasks = append(tasks, domain.ManualTask{ID: "nim-server-compiler", Title: "Review Nim language-server compiler requirements", Instructions: "nimlangserver 1.14.0 requires Nim 2.0.8 to build. Review Nimble's foreground compiler/dependency prompt; it may install that compiler in its managed area. Verify Nimble 0.16.1+ and nimsuggest --v3. If build fails, inspect the reported constraint before retrying.", URL: "https://nim-lang.github.io/langserver/", Required: true})
		}
		tasks = append(tasks, domain.ManualTask{ID: "language:" + lang, Title: "Verify " + lang + " in Zed", Instructions: "Install the language extension where required, confirm server startup, format real source, and run its project task.", URL: "https://zed.dev/docs/languages", Required: true})
	}
	return tasks
}
