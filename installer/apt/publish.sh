#!/usr/bin/env bash
# Rebuilds the APT repository from the .deb files in $DEB_DIR and the packages
# already published, then signs it.
#
# Run from the root of a checkout of the apt repository. Expects:
#   DEB_DIR   directory holding the new .deb files
#   APT_URL   public base URL of the repository, used in the instructions page
#
# The repository keeps every version ever published: apt clients rely on the
# pool staying put, and a downgrade is sometimes the only way out of a bad
# release.

set -euo pipefail

DEB_DIR="${DEB_DIR:?DEB_DIR is required}"
APT_URL="${APT_URL:?APT_URL is required}"

SUITE=stable
COMPONENT=main
# Debian's own names, which is what nfpm stamps into the packages.
ARCHES=(amd64 arm64 armhf i386)
KEYRING=pcpl2lab.gpg

echo "==> adding new packages"
mkdir -p "pool/$COMPONENT/s/sshforward"
cp -v "$DEB_DIR"/*.deb "pool/$COMPONENT/s/sshforward/"

echo "==> indexing"
for arch in "${ARCHES[@]}"; do
	dir="dists/$SUITE/$COMPONENT/binary-$arch"
	mkdir -p "$dir"
	# Paths inside Packages must be relative to the repository root, so this
	# has to run from there.
	apt-ftparchive --arch "$arch" packages pool >"$dir/Packages"
	gzip -9 -k -f "$dir/Packages"
	printf 'Archive: %s\nComponent: %s\nArchitecture: %s\n' \
		"$SUITE" "$COMPONENT" "$arch" >"$dir/Release"
done

echo "==> writing the Release file"
apt-ftparchive \
	-o "APT::FTPArchive::Release::Origin=pcpl2lab" \
	-o "APT::FTPArchive::Release::Label=sshforward" \
	-o "APT::FTPArchive::Release::Suite=$SUITE" \
	-o "APT::FTPArchive::Release::Codename=$SUITE" \
	-o "APT::FTPArchive::Release::Components=$COMPONENT" \
	-o "APT::FTPArchive::Release::Architectures=${ARCHES[*]}" \
	-o "APT::FTPArchive::Release::Description=sshforward packages" \
	release "dists/$SUITE" >"dists/$SUITE/Release"

echo "==> signing"
# InRelease is the inline-signed file modern apt prefers; Release.gpg keeps
# older clients working.
rm -f "dists/$SUITE/InRelease" "dists/$SUITE/Release.gpg"
gpg --batch --yes --clearsign --output "dists/$SUITE/InRelease" "dists/$SUITE/Release"
gpg --batch --yes --armor --detach-sign --output "dists/$SUITE/Release.gpg" "dists/$SUITE/Release"

echo "==> exporting the public key"
# Dearmored, because that is the form `signed-by` expects in a keyring path.
gpg --export >"$KEYRING"

echo "==> writing the cache policy"
# Served by Cloudflare Pages, so caching is controlled with _headers.
#
# The distinction matters: a stale Release served alongside a fresh Packages is
# exactly what makes apt report "Hash Sum mismatch". Pool filenames carry the
# version and never change once published, so those can be cached forever.
cat >_headers <<'HEADERS'
/dists/*
  Cache-Control: no-cache

/pool/*
  Cache-Control: public, max-age=31536000, immutable

/pcpl2lab.gpg
  Cache-Control: public, max-age=3600
HEADERS

echo "==> writing the instructions page"
cat >index.html <<HTML
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>sshforward APT repository</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 46rem; margin: 3rem auto; padding: 0 1rem; line-height: 1.6; }
  pre { background: #f4f4f5; padding: 1rem; overflow-x: auto; border-radius: 6px; }
  code { font-family: ui-monospace, monospace; }
</style>
</head>
<body>
<h1>sshforward APT repository</h1>
<p>Debian and Ubuntu packages for
<a href="https://github.com/pcpl2lab/sshforward">sshforward</a>,
an SSH port forwarding manager.</p>

<h2>Add the repository</h2>
<pre><code>curl -fsSL $APT_URL/$KEYRING | sudo tee /usr/share/keyrings/$KEYRING >/dev/null
echo "deb [signed-by=/usr/share/keyrings/$KEYRING] $APT_URL $SUITE $COMPONENT" \\
  | sudo tee /etc/apt/sources.list.d/pcpl2lab.list
sudo apt update
sudo apt install sshforward</code></pre>

<h2>Later updates</h2>
<pre><code>sudo apt update &amp;&amp; sudo apt upgrade sshforward</code></pre>

<p>Architectures: ${ARCHES[*]}.</p>
</body>
</html>
HTML

echo "==> done"
