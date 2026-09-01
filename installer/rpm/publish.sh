#!/usr/bin/env bash
# Rebuilds the RPM repository from the .rpm files in $RPM_DIR and the packages
# already published, then signs its metadata.
#
# Run from the root of a checkout of the rpm repository. Expects:
#   RPM_DIR   directory holding the new .rpm files
#   RPM_URL   public base URL of the repository
#
# The packages themselves are already signed at build time by nfpm, which is
# what lets clients run with gpgcheck=1. This script signs the repository
# metadata as well, for repo_gpgcheck=1.

set -euo pipefail

RPM_DIR="${RPM_DIR:?RPM_DIR is required}"
RPM_URL="${RPM_URL:?RPM_URL is required}"

KEYRING=pcpl2lab.gpg

echo "==> adding new packages"
mkdir -p packages
cp -v "$RPM_DIR"/*.rpm packages/

echo "==> building metadata"
# Rebuilt from scratch rather than --update, so a repository that ever went
# inconsistent repairs itself on the next release.
rm -rf repodata
createrepo_c --general-compress-type=gz .

echo "==> signing the metadata"
rm -f repodata/repomd.xml.asc
gpg --batch --yes --armor --detach-sign --output repodata/repomd.xml.asc repodata/repomd.xml

echo "==> exporting the public key"
gpg --armor --export >"$KEYRING"

echo "==> writing the .repo file"
# Users drop this into /etc/yum.repos.d/. gpgcheck verifies each package,
# repo_gpgcheck verifies the index that lists them; both matter.
cat >pcpl2lab.repo <<REPO
[pcpl2lab]
name=pcpl2lab - sshforward
baseurl=$RPM_URL
enabled=1
gpgcheck=1
repo_gpgcheck=1
gpgkey=$RPM_URL/$KEYRING
REPO

echo "==> writing the cache policy"
# A stale repomd.xml pointing at metadata files that have already been replaced
# is the dnf equivalent of apt's "Hash Sum mismatch". Package filenames carry
# the version, so those never change.
cat >_headers <<'HEADERS'
/repodata/*
  Cache-Control: no-cache

/packages/*
  Cache-Control: public, max-age=31536000, immutable

/pcpl2lab.repo
  Cache-Control: no-cache

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
<title>sshforward RPM repository</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 46rem; margin: 3rem auto; padding: 0 1rem; line-height: 1.6; }
  pre { background: #f4f4f5; padding: 1rem; overflow-x: auto; border-radius: 6px; }
  code { font-family: ui-monospace, monospace; }
</style>
</head>
<body>
<h1>sshforward RPM repository</h1>
<p>Fedora, RHEL and openSUSE packages for
<a href="https://github.com/pcpl2lab/sshforward">sshforward</a>,
an SSH port forwarding manager.</p>

<h2>Add the repository</h2>
<pre><code>sudo curl -fsSL -o /etc/yum.repos.d/pcpl2lab.repo $RPM_URL/pcpl2lab.repo
sudo dnf install sshforward</code></pre>

<p>On openSUSE:</p>
<pre><code>sudo zypper addrepo -f $RPM_URL pcpl2lab
sudo zypper install sshforward</code></pre>

<h2>Later updates</h2>
<pre><code>sudo dnf upgrade sshforward</code></pre>

<p>Packages and repository metadata are both signed.</p>
</body>
</html>
HTML

echo "==> done"
