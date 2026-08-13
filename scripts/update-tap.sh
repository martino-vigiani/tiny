#!/usr/bin/env bash
# Rewrite homebrew-tap/Formula/tiny.rb for a tagged release.
# Usage: scripts/update-tap.sh v0.4.0 [/path/to/homebrew-tap]
set -euo pipefail

tag="${1:?usage: update-tap.sh vX.Y.Z [tap-dir]}"
tap="${2:-$(cd "$(dirname "$0")/../../homebrew-tap" && pwd)}"
formula="$tap/Formula/tiny.rb"
version="${tag#v}"

sums="$(gh release download "$tag" -R martino-vigiani/tiny -O - --pattern SHA256SUMS)"
sha() { printf '%s\n' "$sums" | awk -v f="$1" '$2==f {print $1; exit}'; }

arm="$(sha tiny-darwin-arm64.tar.gz)"
intel="$(sha tiny-darwin-amd64.tar.gz)"
linux_arm="$(sha tiny-linux-arm64.tar.gz)"
linux_amd="$(sha tiny-linux-amd64.tar.gz)"

test -n "$arm" && test -n "$intel" && test -n "$linux_arm" && test -n "$linux_amd"

python3 - "$formula" "$version" "$arm" "$intel" "$linux_arm" "$linux_amd" <<'PY'
import pathlib, sys
path, version, arm, intel, larm, lamd = sys.argv[1:]
text = pathlib.Path(path).read_text()
# bump the version string and the four tarball hashes in document order
import re
text = re.sub(r'version "[^"]+"', f'version "{version}"', text, count=1)
text = re.sub(r'v[0-9]+\.[0-9]+\.[0-9]+/tiny-', f'v{version}/tiny-', text)
hashes = [arm, intel, larm, lamd]
def repl(m, i=[0]):
    h = hashes[i[0]]; i[0] += 1
    return f'sha256 "{h}"'
text = re.sub(r'sha256 "[0-9a-f]+"', repl, text)
pathlib.Path(path).write_text(text)
print(f"updated {path} to {version}")
PY
