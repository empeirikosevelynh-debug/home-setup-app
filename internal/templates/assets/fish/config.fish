if test -x /opt/homebrew/bin/brew
    /opt/homebrew/bin/brew shellenv fish | source
end
fish_add_path --path --append /usr/local/bin $HOME/go/bin $HOME/.nimble/bin

# Help Crystal and other build tools find Homebrew's OpenSSL metadata.
if test -d /opt/homebrew/opt/openssl@3/lib/pkgconfig
    if not contains -- /opt/homebrew/opt/openssl@3/lib/pkgconfig $PKG_CONFIG_PATH
        set -gx PKG_CONFIG_PATH /opt/homebrew/opt/openssl@3/lib/pkgconfig $PKG_CONFIG_PATH
    end
end

set -gx EDITOR 'zed --wait'
set -gx VISUAL 'zed --wait'

if status is-interactive
    set -g fish_greeting
    abbr -a e zed
    abbr -a fishconf 'chezmoi edit ~/.config/fish/config.fish'
    abbr -a zedconf 'zed ~/.config/zed/settings.json'

    if type -q eza
        abbr -a ls 'eza --icons'
        abbr -a ll 'eza -la --icons --git'
        abbr -a tree 'eza --tree --icons'
    end

    if type -q zoxide
        zoxide init fish | source
    end
    if type -q starship
        starship init fish | source
    end

    if type -q fzf_configure_bindings
        if test "$TERM_PROGRAM" = WarpTerminal
            fzf_configure_bindings --history=
        else
            fzf_configure_bindings
        end
    end
end
