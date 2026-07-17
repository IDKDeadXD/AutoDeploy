#!/usr/bin/env sh
set -eu

version="${1:?pass a release version}"
base_url="${DEPLOY_AGENT_RELEASE_BASE_URL:?set DEPLOY_AGENT_RELEASE_BASE_URL to the release asset base URL}"
arch="$(uname -m)"
case "$arch" in
  x86_64) asset="deploy-linux-amd64" ;;
  aarch64|arm64) asset="deploy-linux-arm64" ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
curl -fsSLo "$tmp/$asset" "$base_url/$version/$asset"
curl -fsSLo "$tmp/checksums.txt" "$base_url/$version/checksums.txt"
(cd "$tmp" && sha256sum -c checksums.txt --ignore-missing)
install -m 0755 "$tmp/$asset" /usr/local/bin/deploy
echo "Installed deploy. Run: sudo deploy install"
