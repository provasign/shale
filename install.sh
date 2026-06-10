#!/bin/sh
# Shale installer: curl -fsSL https://get.shale.dev | bash
# Downloads the latest release binary for this OS/arch into /usr/local/bin
# (or ~/.local/bin when not writable).
set -eu

REPO="provasign/shale"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$(uname -m)" in
  x86_64|amd64)  arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "shale: unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac

tag=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" |
  grep '"tag_name"' | head -1 | cut -d'"' -f4)
[ -n "$tag" ] || { echo "shale: could not resolve latest release" >&2; exit 1; }

url="https://github.com/${REPO}/releases/download/${tag}/shale_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading shale ${tag} (${os}/${arch})..."
curl -fsSL "$url" -o "$tmp/shale.tar.gz"

if curl -fsSL "https://github.com/${REPO}/releases/download/${tag}/checksums.txt" -o "$tmp/checksums.txt" 2>/dev/null; then
  (cd "$tmp" && grep "shale_${os}_${arch}.tar.gz" checksums.txt | sha256sum -c - >/dev/null 2>&1) ||
  (cd "$tmp" && grep "shale_${os}_${arch}.tar.gz" checksums.txt | shasum -a 256 -c - >/dev/null 2>&1) ||
    { echo "shale: checksum verification failed" >&2; exit 1; }
fi

tar -xzf "$tmp/shale.tar.gz" -C "$tmp" shale

dest=/usr/local/bin
if [ ! -w "$dest" ]; then
  dest="$HOME/.local/bin"
  mkdir -p "$dest"
fi
install -m 0755 "$tmp/shale" "$dest/shale"
echo "Installed: $dest/shale"
echo "Next: cd your-repo && shale init   (5 minutes, no account)"
