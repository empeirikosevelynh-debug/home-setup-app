#!/bin/zsh
# Source-build starter; never changes global startup files or the login shell.
emulate -LR zsh
setopt errexit nounset pipefail
readonly project_dir="${0:A:h:h}"
readonly installer_revision='04dfcac13ead62adfc864260c5d5f9404d145d50'
readonly installer_sha256='fa4ed743b4ca38316c8f32fd6623baa5bbb928fd4a47ad9f49c2be78ea833449'
preview=false
build_only=false
typeset -a wizard_args
while (( $# )); do
  case "$1" in
    --plan) preview=true; shift ;;
    --build-only) build_only=true; shift ;;
    --) shift; wizard_args=("$@"); break ;;
    -h|--help) print -r -- 'Usage: zsh scripts/bootstrap.zsh [--plan | --build-only] [-- wizard arguments]'; exit 0 ;;
    *) print -u2 -r -- "Unknown starter option: $1. Put wizard options after --."; exit 2 ;;
  esac
done
if $preview && $build_only; then print -u2 -- 'Choose --plan or --build-only.'; exit 2; fi
arch=$(uname -m)
version=$(sw_vers -productVersion)
translated=$(sysctl -in sysctl.proc_translated 2>/dev/null || true)
if [[ "$arch" != arm64 || "${version%%.*}" != 27 || "$translated" == 1 ]]; then
  print -u2 -- 'This starter requires native Apple-silicon macOS 27.'; exit 2
fi
clt_ready=false
clt_path=$(xcode-select -p 2>/dev/null || true)
if [[ -n "$clt_path" && -d "$clt_path" ]] && clang --version >/dev/null 2>&1; then clt_ready=true; fi
brew_path=${commands[brew]:-}
if [[ -z "$brew_path" && -x /opt/homebrew/bin/brew ]]; then brew_path=/opt/homebrew/bin/brew; fi
brew_ready=false
if [[ -n "$brew_path" ]]; then
  prefix=$(HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_ANALYTICS=1 "$brew_path" --prefix)
  if [[ "$prefix" != /opt/homebrew ]]; then
    print -u2 -- 'A conflicting Homebrew prefix is active. Select /opt/homebrew and run again.'; exit 2
  fi
  brew_ready=true
fi
go_path=${commands[go]:-}
if [[ -z "$go_path" && -x /opt/homebrew/bin/go ]]; then go_path=/opt/homebrew/bin/go; fi
go_ready=false
check_go() {
  [[ -n "$go_path" ]] || return 1
  local description
  description=$(GOTOOLCHAIN=local "$go_path" version 2>/dev/null) || return 1
  [[ "$description" == *' darwin/arm64' ]] || return 1
  [[ "$description" =~ '^go version go([0-9]+)\.([0-9]+)\.[0-9]+ ' ]] || return 1
  (( match[1] > 1 || match[1] == 1 && match[2] >= 26 ))
}
if check_go; then go_ready=true; fi
print -r -- "Golden Gate Setup · macOS $version · $arch"
print -r -- "Project: $project_dir"
if $clt_ready; then print -- 'Command Line Tools: ready'; else print -- 'Command Line Tools: system installation required'; fi
if $brew_ready; then print -- 'Homebrew: native prefix ready'; else print -- 'Homebrew: official native installation required'; fi
if $go_ready; then print -- 'Go: compatible native compiler ready'; else print -- 'Go: native Go 1.26.0 or later required'; fi
print -- 'After prerequisites: build the local source, then inspect and review the wizard plan.'
if $preview; then
  print -- 'Preview complete. Nothing was installed, downloaded, or written.'
  exit 0
fi
if ! $clt_ready || ! $brew_ready || ! $go_ready; then
  print -- 'If reinstalling macOS, restore your account/files first with Migration Assistant.'
  print -n -- 'Prepare the listed missing prerequisites? Type yes to proceed: '
  IFS= read -r answer || { print -u2 -- 'No confirmation received.'; exit 1; }
  if [[ "$answer" != yes ]]; then print -- 'No prerequisites were changed.'; exit 1; fi
fi
if ! $clt_ready; then
  xcode-select --install || { print -u2 -- 'Complete the system Command Line Tools installation, then run again.'; exit 3; }
  print -- 'Complete the system Command Line Tools installation, then run again.'
  exit 3
fi
if ! $brew_ready; then
  command git --version >/dev/null || { print -u2 -- 'Git is unavailable after Command Line Tools setup; run again after repairing them.'; exit 3; }
  temporary_dir=$(mktemp -d "${TMPDIR:-/tmp}/golden-gate-bootstrap.XXXXXXXX")
  trap 'rm -rf -- "$temporary_dir"' EXIT HUP INT TERM
  installer="$temporary_dir/install.sh"
  curl -q --fail --location --proto '=https' --tlsv1.2 --retry 2 \
    "https://raw.githubusercontent.com/Homebrew/install/$installer_revision/install.sh" -o "$installer"
  actual=$(shasum -a 256 "$installer")
  if [[ "${actual%% *}" != "$installer_sha256" ]]; then print -u2 -- 'Homebrew installer checksum did not match; it was not executed.'; exit 1; fi
  print -r -- "Verified official Homebrew installer: $installer_revision · SHA-256 $installer_sha256"
  /bin/bash "$installer"
  brew_path=/opt/homebrew/bin/brew
  [[ -x "$brew_path" ]] || { print -u2 -- 'Native Homebrew installation is incomplete; repair it and run again.'; exit 1; }
  [[ "$($brew_path --prefix)" == /opt/homebrew ]] || { print -u2 -- 'Homebrew prefix validation failed.'; exit 1; }
  rm -rf -- "$temporary_dir"
  trap - EXIT HUP INT TERM
fi
if ! $go_ready; then
  # The explicit prerequisite confirmation also covers a deliberate Go update.
  if HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_ANALYTICS=1 "$brew_path" list --versions go >/dev/null 2>&1; then
    HOMEBREW_NO_INSTALL_CLEANUP=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_ANALYTICS=1 "$brew_path" upgrade --formula go
  else
    HOMEBREW_NO_INSTALL_CLEANUP=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_ANALYTICS=1 "$brew_path" install --formula go
  fi
  go_path=/opt/homebrew/bin/go
  if ! check_go; then print -u2 -- 'The required native Go compiler is still unavailable. Repair it and run again.'; exit 1; fi
fi
[[ "$(xcode-select -p 2>/dev/null)" == "$clt_path" ]] || { print -u2 -- 'The selected developer tools changed. Inspect them and run again.'; exit 1; }
[[ "$(HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_ANALYTICS=1 "$brew_path" --prefix)" == /opt/homebrew ]] || exit 2
check_go || { print -u2 -- 'Go validation failed before the build.'; exit 1; }
cd -- "$project_dir"
mkdir -p -- bin
GOTOOLCHAIN=local GOWORK=off GOFLAGS='' "$go_path" build -mod=readonly -trimpath -o "$project_dir/bin/golden-setup" ./cmd/golden-setup
print -r -- "Built: $project_dir/bin/golden-setup"
if $build_only; then exit 0; fi
exec "$project_dir/bin/golden-setup" "${wizard_args[@]}"
