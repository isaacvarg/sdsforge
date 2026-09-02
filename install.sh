#!/bin/sh
#
# Install sdsforge from its latest GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/isaacvarg/sdsforge/main/install.sh | sh
#
# Environment:
#   SDSFORGE_VERSION       tag to install (default: the latest release)
#   SDSFORGE_INSTALL_DIR   where to put the binary (default: ~/.local/bin)
#
# No sudo is used, and nothing outside the install directory is touched.

set -eu

REPO="isaacvarg/sdsforge"
BIN="sdsforge"

say() { printf '%s\n' "$*"; }
warn() { printf '\033[33mwarning:\033[0m %s\n' "$*" >&2; }
die() { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }

# --- how we fetch -----------------------------------------------------------

if command -v curl >/dev/null 2>&1; then
	fetch() { curl -fsSL "$1" -o "$2"; }
	# Resolve the latest tag by following the /releases/latest redirect rather
	# than asking api.github.com, which rate-limits unauthenticated callers at
	# 60/hour -- shared networks and CI hit that.
	resolve_latest() {
		curl -fsSLI -o /dev/null -w '%{url_effective}' \
			"https://github.com/$REPO/releases/latest"
	}
elif command -v wget >/dev/null 2>&1; then
	fetch() { wget -qO "$2" "$1"; }
	resolve_latest() {
		wget -q -S -O /dev/null "https://github.com/$REPO/releases/latest" 2>&1 |
			awk '/^[ \t]*Location:/ { url = $2 } END { print url }'
	}
else
	die "this needs curl or wget, and found neither"
fi

# --- what we are running on -------------------------------------------------

os=$(uname -s)
case "$os" in
	Linux) os=linux ;;
	Darwin)
		die "there are no macOS builds yet. Build from source instead:
  git clone https://github.com/$REPO.git && cd sdsforge && go build -o $BIN ." ;;
	*)
		die "unsupported operating system: $os
  Windows users: download the .zip from https://github.com/$REPO/releases" ;;
esac

arch=$(uname -m)
case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) die "unsupported architecture: $arch" ;;
esac

# --- which version ----------------------------------------------------------

version="${SDSFORGE_VERSION:-}"
if [ -z "$version" ]; then
	say "Looking up the latest release..."
	url=$(resolve_latest) || die "could not reach GitHub to find the latest release"
	version=${url##*/}
	case "$version" in
		v*) ;;
		*) die "could not work out the latest version (got \"$version\")" ;;
	esac
fi
# Archive names carry the bare version, tags carry the leading v.
bare=${version#v}

# --- where it goes ----------------------------------------------------------

dir="${SDSFORGE_INSTALL_DIR:-}"
if [ -z "$dir" ]; then
	if [ "$(id -u)" -eq 0 ]; then
		dir=/usr/local/bin
	else
		dir="${XDG_BIN_HOME:-$HOME/.local/bin}"
	fi
fi

# --- fetch, verify, install -------------------------------------------------

archive="${BIN}_${bare}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$version"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

say "Downloading $BIN $version ($os/$arch)..."
fetch "$base/$archive" "$tmp/$archive" ||
	die "could not download $archive
  Check that $version has a build for $os/$arch:
  https://github.com/$REPO/releases/tag/$version"
fetch "$base/checksums.txt" "$tmp/checksums.txt" ||
	die "could not download checksums.txt, so the archive cannot be verified"

# A download that cannot be verified is not installed. Skipping the check when
# no tool is available would be the one case where it silently matters most.
if command -v sha256sum >/dev/null 2>&1; then
	sha="sha256sum -c"
elif command -v shasum >/dev/null 2>&1; then
	sha="shasum -a 256 -c"
else
	die "no sha256sum or shasum, so the download cannot be verified"
fi

say "Verifying checksum..."
(cd "$tmp" && grep " $archive\$" checksums.txt | $sha - >/dev/null 2>&1) ||
	die "checksum mismatch on $archive -- the download is corrupt or tampered with"

tar -xzf "$tmp/$archive" -C "$tmp" ||
	die "could not unpack $archive"
[ -f "$tmp/$BIN" ] || die "$archive did not contain a $BIN binary"

mkdir -p "$dir" || die "could not create $dir"
install -m 755 "$tmp/$BIN" "$dir/$BIN" ||
	die "could not write to $dir
  Pick somewhere else with:  SDSFORGE_INSTALL_DIR=~/bin"

say ""
say "Installed $("$dir/$BIN" --version) to $dir/$BIN"

# --- what is left to do -----------------------------------------------------

case ":$PATH:" in
	*":$dir:"*) ;;
	*)
		say ""
		warn "$dir is not on your PATH. Add it:"
		say ""
		say "    echo 'export PATH=\"$dir:\$PATH\"' >> ~/.zshrc"
		say ""
		;;
esac

# Same list, and the same order, as internal/generation/browser.go.
found=""
for b in chromium chromium-browser google-chrome google-chrome-stable \
	brave brave-browser microsoft-edge microsoft-edge-stable; do
	if command -v "$b" >/dev/null 2>&1; then
		found=$b
		break
	fi
done

if [ -z "$found" ]; then
	say ""
	warn "no Chrome-based browser found, so PDF printing will not work yet."
	say "         Install one (chromium, google-chrome, brave, microsoft-edge),"
	say "         or name yours in the config file. Everything else works now."
fi

say ""
say "Get started with:  $BIN config init"
