#!/bin/sh
# Install an official Trail Guide release without sudo; set up PATH unless explicitly disabled.
# Downloads and checksums come from the same release; they are not independent signatures.

# Keep execution at the end so a truncated piped download cannot start installation.
main() (
    set -eu
    version=latest
    update_path=true
    install_dir=${HOME:?HOME must be set}/.local/bin
    repository=https://github.com/fabricahq/trailguide
    temporary=
    staging=
    trap 'rm -rf "$temporary" "$staging"' EXIT
    trap 'exit 1' HUP INT TERM

    fail() { printf 'trailguide installer: %s\n' "$*" >&2; exit 1; }
    download() {
        curl --fail --silent --show-error --location --retry 3 \
            --proto '=https' --proto-redir '=https' --tlsv1.2 "$@"
    }
    # Append only our PATH entry, preserving user content and avoiding repeated entries.
    # Do not follow profile symlinks or replace special files; explain manual setup instead.
    append_path() (
        profile=$1
        case "$profile" in /*) ;; *) return 1 ;; esac
        [ ! -L "$profile" ] || return 1
        if [ -e "$profile" ]; then
            [ -f "$profile" ] && [ -r "$profile" ] && [ -w "$profile" ] || return 1
            if grep -Fqx -- "$path_entry" "$profile"; then
                printf 'PATH setup already present in %s\n' "$profile"
                return 0
            fi
        fi
        mkdir -p "${profile%/*}" || return 1
        printf '\n# Added by the Trail Guide installer\n%s\n' "$path_entry" >> "$profile" || return 1
        printf 'Added PATH setup to %s\n' "$profile"
    )
    configure_path() {
        # SHELL identifies the user's shell, not the sh process running this installer.
        shell_name=${SHELL:-}
        case "${shell_name##*/}" in
            zsh) append_path "${ZDOTDIR:-$HOME}/.zshrc" ;;
            bash)
                login_profile=$HOME/.profile
                for candidate in "$HOME/.bash_profile" "$HOME/.bash_login" "$HOME/.profile"; do
                    if [ -e "$candidate" ] || [ -L "$candidate" ]; then
                        login_profile=$candidate
                        break
                    fi
                done
                append_path "$HOME/.bashrc" && append_path "$login_profile" ;;
            sh|dash) append_path "$HOME/.profile" ;;
            fish)
                # Fish single quotes escape both backslashes and single quotes.
                fish_quoted=$(printf '%s' "$install_dir" | sed "s/\\\\/\\\\\\\\/g; s/'/\\\\'/g")
                path_entry="contains -- '$fish_quoted' \$PATH; or set -gx PATH '$fish_quoted' \$PATH"
                append_path "${XDG_CONFIG_HOME:-$HOME/.config}/fish/config.fish" ;;
            *) return 1 ;;
        esac
    }
    while [ "$#" -gt 0 ]; do
        case "$1" in
            --version|--install-dir)
                [ "$#" -ge 2 ] || fail "$1 requires a value"
                case "$1" in
                    --version) version=${2#v} ;;
                    --install-dir) install_dir=$2 ;;
                esac
                shift 2 ;;
            --no-update-path) update_path=false; shift ;;
            --help|-h)
                printf '%s\n' 'Usage: sh install.sh [--version VERSION] [--install-dir DIRECTORY] [--no-update-path]' \
                    'Defaults: latest stable release, ~/.local/bin. Rerun to upgrade.' \
                    'No sudo is used. PATH setup for zsh, bash, sh, or fish is automatic.' \
                    'Use --no-update-path to leave shell configuration unchanged.'
                exit 0 ;;
            *) fail "unknown option: $1 (see --help)" ;;
        esac
    done
    case "$install_dir" in
        /*) ;;
        *) fail '--install-dir must be an absolute path' ;;
    esac
    case "$install_dir" in
        *:*) fail 'installation directory cannot contain a colon (PATH separator)' ;;
        *'
'*) fail 'installation directory cannot contain a newline' ;;
    esac
    for command in curl tar mktemp awk uname chmod mv mkdir grep sed; do
        command -v "$command" >/dev/null 2>&1 || fail "required command not found: $command"
    done
    if command -v sha256sum >/dev/null 2>&1; then
        checksum=sha256sum
    elif command -v shasum >/dev/null 2>&1; then
        checksum=shasum
    else
        fail 'install sha256sum or shasum to verify release downloads'
    fi
    case "$(uname -s)" in
        Darwin) os=darwin ;;
        Linux) os=linux ;;
        *) fail 'supported operating systems are macOS and Linux' ;;
    esac
    case "$(uname -m)" in
        arm64|aarch64) arch=arm64 ;;
        x86_64|amd64) arch=amd64 ;;
        *) fail 'supported processor architectures are arm64 and amd64' ;;
    esac
    if [ "$version" = latest ]; then
        resolved=$(download --output /dev/null --write-out '%{url_effective}' "$repository/releases/latest") ||
            fail 'cannot find the latest release; check GitHub Releases or use --version'
        case "$resolved" in
            "$repository"/releases/tag/v*) version=${resolved##*/v} ;;
            *) fail 'GitHub did not resolve a published release' ;;
        esac
    fi
    # Restrict versions before inserting them into filenames or release URLs.
    printf '%s\n' "$version" | LC_ALL=C grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$' ||
        fail 'version must look like 0.1.0 or 0.2.0-rc.1'
    temporary=$(mktemp -d "${TMPDIR:-/tmp}/trailguide-download.XXXXXX")
    base=$repository/releases/download/v$version
    printf 'Downloading Trail Guide %s for %s/%s...\n' "$version" "$os" "$arch"
    download --output "$temporary/SHA256SUMS" "$base/SHA256SUMS" || fail 'could not download release checksums'
    # Releases from before Code Rules was renamed Trail Guide name their archive and executable code-rules.
    name=trailguide
    if ! awk -v file="trailguide_${version}_${os}_${arch}.tar.gz" '$2 == file { found=1 } END { exit !found }' "$temporary/SHA256SUMS"; then
        name=code-rules
    fi
    archive=${name}_${version}_${os}_${arch}.tar.gz
    download --output "$temporary/archive.tar.gz" "$base/$archive" || fail "could not download $archive"
    expected=$(awk -v file="$archive" '$2 == file { count++; hash=$1 } END { if (count != 1 || length(hash) != 64 || hash ~ /[^0-9a-f]/) exit 1; print hash }' "$temporary/SHA256SUMS") ||
        fail 'release checksums must contain exactly one valid entry for this archive'
    if [ "$checksum" = sha256sum ]; then
        actual=$(sha256sum "$temporary/archive.tar.gz")
    else
        actual=$(shasum -a 256 "$temporary/archive.tar.gz")
    fi
    actual=${actual%% *}
    [ "$actual" = "$expected" ] || fail 'checksum mismatch; existing installation was not changed'

    # Extract only named regular files to stdout, never archive-controlled paths or links.
    tar -tzf "$temporary/archive.tar.gz" > "$temporary/members" || fail 'invalid release archive'
    awk -v name="$name" '($0 != name && $0 != "LICENSE.md" && $0 != "README.txt") || seen[$0]++ { exit 1 } END { if (!seen[name] || !seen["LICENSE.md"]) exit 1 }' "$temporary/members" ||
        fail 'unexpected or missing release archive files'
    tar -tvzf "$temporary/archive.tar.gz" > "$temporary/types" || fail 'invalid release archive'
    awk 'substr($0,1,1) != "-" { exit 1 }' "$temporary/types" || fail 'release archive contains a link or non-regular file'
    [ ! -L "$install_dir/$name" ] || fail 'destination is a symlink; choose another --install-dir'
    [ ! -d "$install_dir/$name" ] || fail 'destination is a directory; choose another --install-dir'
    mkdir -p "$install_dir" || fail 'cannot create installation directory; choose a writable --install-dir'
    staging=$(mktemp -d "$install_dir/.trailguide-install.XXXXXX")
    tar -xzOf "$temporary/archive.tar.gz" "$name" > "$staging/$name"
    tar -xzOf "$temporary/archive.tar.gz" LICENSE.md > "$staging/$name.LICENSE"
    [ -s "$staging/$name" ] && [ -s "$staging/$name.LICENSE" ] || fail 'release executable or license is empty'
    chmod 755 "$staging/$name"
    # Check that the verified executable runs before replacing a working installation.
    "$staging/$name" --version || fail 'downloaded executable cannot run; existing installation was not changed'
    mv -f "$staging/$name" "$install_dir/$name"
    printf 'Installed Trail Guide %s to %s/%s\n' "$version" "$install_dir" "$name"
    quoted=$(printf '%s' "$install_dir" | sed "s/'/'\\\\''/g")
    path_entry="case \":\$PATH:\" in *:'$quoted':*) ;; *) export PATH='$quoted':\"\$PATH\" ;; esac"
    if [ "$update_path" = true ]; then
        if configure_path; then
            printf '\nOpen a new terminal to use Trail Guide.\n'
        else
            printf '\nTrail Guide is installed, but automatic PATH setup could not be completed for %s.\n' "${SHELL:-an unknown shell}" >&2
            printf 'Your shell may be unsupported or its startup file may be linked or unwritable. Add the installation directory manually.\n' >&2
        fi
    fi
    case ":${PATH:-}:" in
        *:"$install_dir":*) ;;
        *)
            printf '\nFor this terminal, run:\n'
            case "${SHELL:-}" in
                */fish)
                    fish_quoted=$(printf '%s' "$install_dir" | sed "s/\\\\/\\\\\\\\/g; s/'/\\\\'/g")
                    printf "fish_add_path --path '%s'\n" "$fish_quoted" ;;
                *) printf "export PATH='%s':\"\$PATH\"\n" "$quoted" ;;
            esac
            if [ "$update_path" = false ]; then
                printf '%s\n' 'Shell configuration was left unchanged (--no-update-path).'
            fi ;;
    esac
    existing=$(command -v "$name" || true)
    if [ -n "$existing" ] && [ "$existing" != "$install_dir/$name" ]; then
        printf 'PATH currently selects %s. Put %s first to use this installation.\n' "$existing" "$install_dir"
    fi
)
main "$@"
