#!/bin/sh
# Rebuilds the Alpine repository from the .apk files in $APK_DIR and the
# packages already published, then signs each architecture's index.
#
# Must run inside Alpine: apk-tools and abuild-sign exist nowhere else. The
# release workflow invokes it through a container for that reason.
#
# Run from the root of a checkout of the apk repository. Expects:
#   APK_DIR       directory holding the new .apk files
#   APK_URL       public base URL of the repository
#   APK_KEY_FILE  RSA private key, the same one nfpm signed the packages with
#
# Alpine does not use PGP anywhere: packages and indexes are signed with RSA,
# and clients trust a public key dropped into /etc/apk/keys.

set -eu

: "${APK_DIR:?APK_DIR is required}"
: "${APK_URL:?APK_URL is required}"
: "${APK_KEY_FILE:?APK_KEY_FILE is required}"

KEYNAME=pcpl2lab.rsa

echo "==> preparing the signing key"
# abuild-sign derives the public key's name from the private key's filename, and
# that name has to match the one embedded in the packages by nfpm.
work=$(mktemp -d)
cp "$APK_KEY_FILE" "$work/$KEYNAME"
chmod 600 "$work/$KEYNAME"
openssl rsa -in "$work/$KEYNAME" -pubout -out "$work/$KEYNAME.pub" 2>/dev/null

echo "==> adding new packages"
arches=""
for f in "$APK_DIR"/*.apk; do
	# Read these from the package itself rather than its filename, so a change
	# in naming cannot silently misfile a package.
	info=$(tar -xzOf "$f" .PKGINFO 2>/dev/null)
	arch=$(printf '%s\n' "$info" | awk -F' = ' '/^arch/ {print $2; exit}')
	name=$(printf '%s\n' "$info" | awk -F' = ' '/^pkgname/ {print $2; exit}')
	ver=$(printf '%s\n' "$info" | awk -F' = ' '/^pkgver/ {print $2; exit}')
	if [ -z "$arch" ] || [ -z "$name" ] || [ -z "$ver" ]; then
		echo "error: cannot read arch/pkgname/pkgver from $f" >&2
		exit 1
	fi
	mkdir -p "$arch"
	# apk builds the download URL from the index's name and version, not from
	# whatever the file happens to be called, so the file has to be named the
	# way apk will ask for it.
	cp -v "$f" "$arch/$name-$ver.apk"
	case " $arches " in
	*" $arch "*) ;;
	*) arches="$arches $arch" ;;
	esac
done

echo "==> trusting the key locally"
# apk index refuses to read a package whose signature it cannot verify, so the
# public key has to be trusted before indexing. Doubling as a check: if this key
# did not sign the packages, indexing fails here rather than shipping an index
# nobody can install from. Safe because this script only ever runs in a
# disposable container.
cp "$work/$KEYNAME.pub" /etc/apk/keys/

echo "==> indexing and signing"
for arch in $arches; do
	(
		cd "$arch"
		# --rewrite-arch keeps the index honest if a package was built for a
		# differently named but compatible arch.
		apk index --rewrite-arch "$arch" -o APKINDEX.unsigned.tar.gz ./*.apk
		abuild-sign -k "$work/$KEYNAME" APKINDEX.unsigned.tar.gz
		mv APKINDEX.unsigned.tar.gz APKINDEX.tar.gz
	)
done

echo "==> publishing the public key"
cp "$work/$KEYNAME.pub" "$KEYNAME.pub"
rm -rf "$work"

echo "==> writing the cache policy"
# A cached index keeps a new release invisible to apk until it expires.
#
# The rules are written out one architecture at a time, as literal paths.
# Cloudflare allows only one splat per pattern and matches it greedily, so
# neither "/*/APKINDEX.tar.gz" nor "/*/*.apk" matches anything here - and a
# rule that never fires looks exactly like a rule that works.
#
# Package files are left on the default policy: their names carry the version,
# they never change, and the default revalidates rather than serving blind.
{
	for arch in $arches; do
		printf '/%s/APKINDEX.tar.gz\n  Cache-Control: no-cache\n\n' "$arch"
	done
	printf '/%s.pub\n  Cache-Control: public, max-age=3600\n' "$KEYNAME"
} >_headers

echo "==> writing the instructions page"
cat >index.html <<HTML
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>sshforward Alpine repository</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 46rem; margin: 3rem auto; padding: 0 1rem; line-height: 1.6; }
  pre { background: #f4f4f5; padding: 1rem; overflow-x: auto; border-radius: 6px; }
  code { font-family: ui-monospace, monospace; }
</style>
</head>
<body>
<h1>sshforward Alpine repository</h1>
<p>Alpine packages for
<a href="https://github.com/pcpl2lab/sshforward">sshforward</a>,
an SSH port forwarding manager.</p>

<h2>Add the repository</h2>
<pre><code>curl -fsSL $APK_URL/$KEYNAME.pub | sudo tee /etc/apk/keys/$KEYNAME.pub >/dev/null
echo "$APK_URL" | sudo tee -a /etc/apk/repositories
sudo apk update
sudo apk add sshforward</code></pre>

<h2>Later updates</h2>
<pre><code>sudo apk add --upgrade sshforward</code></pre>

<p>Architectures:$arches. Packages and indexes are signed.</p>
</body>
</html>
HTML

echo "==> done"
