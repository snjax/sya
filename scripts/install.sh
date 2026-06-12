#!/bin/sh
set -eu

module="github.com/snjax/sya/cmd/sya"
version="${SYA_VERSION:-latest}"
install_dir="${SYA_INSTALL_DIR:-$HOME/.local/bin}"

error() {
	printf '%s\n' "error: $*" >&2
	exit 1
}

info() {
	printf '%s\n' "$*"
}

command -v go >/dev/null 2>&1 || error "go is required; install Go 1.26 or newer from https://go.dev/dl/"

go_version_output=$(go version 2>/dev/null || true)
go_version=$(printf '%s\n' "$go_version_output" | sed -n 's/^go version go\([0-9][0-9.]*\).*/\1/p')
go_major=$(printf '%s\n' "$go_version" | sed -n 's/^\([0-9][0-9]*\).*/\1/p')
go_minor=$(printf '%s\n' "$go_version" | sed -n 's/^[0-9][0-9]*\.\([0-9][0-9]*\).*/\1/p')

if [ -z "$go_major" ] || [ -z "$go_minor" ]; then
	error "could not parse Go version from: $go_version_output"
fi

if [ "$go_major" -lt 1 ] || { [ "$go_major" -eq 1 ] && [ "$go_minor" -lt 26 ]; }; then
	error "go 1.26 or newer is required; found go$go_version. Install from https://go.dev/dl/"
fi

gobin=$(go env GOBIN 2>/dev/null || true)
if [ -z "$gobin" ]; then
	gopath=$(go env GOPATH 2>/dev/null || true)
	[ -n "$gopath" ] || error "could not determine GOPATH"
	case "$gopath" in
		*:*) gopath=${gopath%%:*} ;;
	esac
	gobin="$gopath/bin"
fi

info "Installing sya $version with go install..."
GO111MODULE=on go install "$module@$version" || error "go install failed"

src="$gobin/sya"
[ -x "$src" ] || error "installed binary not found at $src"

mkdir -p "$install_dir" || error "could not create $install_dir"
cp "$src" "$install_dir/sya" || error "could not copy sya to $install_dir"
chmod 755 "$install_dir/sya" || error "could not chmod $install_dir/sya"

case ":$PATH:" in
	*":$install_dir:"*) ;;
	*)
		info ""
		info "$install_dir is not in PATH."
		case "${SHELL:-}" in
			*/bash)
				info "Add it with: printf '%s\n' 'export PATH=\"$install_dir:\$PATH\"' >> ~/.bashrc"
				;;
			*/fish)
				info "Add it with: fish_add_path $install_dir"
				;;
			*/zsh)
				info "Add it with: printf '%s\n' 'export PATH=\"$install_dir:\$PATH\"' >> ~/.zshrc"
				;;
			*)
				info "Add it with: printf '%s\n' 'export PATH=\"$install_dir:\$PATH\"' >> ~/.profile"
				;;
		esac
		;;
esac

info ""
"$install_dir/sya" version
