#!/bin/sh
# Greenter parity check: renders the same documents with Greenter (PHP) and
# sunat-go, then diffs their canonical XML (C14N, blanks removed, signature id
# normalised). It is a manual tool, not part of `go test`.
#
#   GREENTER_AUTOLOAD=/path/to/project/vendor/autoload.php scripts/parity/run.sh
#
# Requires php, xmllint and a Composer project with greenter/lite installed.
# Known, accepted differences are listed in scripts/parity/README.md.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
out=$(mktemp -d)
mkdir -p "$out/php" "$out/go"
php "$here/greenter-sale.php" "$out/php" >/dev/null
php "$here/greenter-batch.php" "$out/php" >/dev/null
(cd "$root" && go run "$here/gen.go" "$out/go")
for f in summary-basic voided-basic reversion-basic retention-basic perception-basic despatch-basic; do
	cp "$root/ubl/testdata/$f.xml" "$out/go/"
done
norm() { xmllint --noblanks "$1" | xmllint --c14n - | sed 's/></>\n</g; s/#GREENTER-SIGN/#SIG/; s/#SUNATGO-SIGN/#SIG/'; }
status=0
for f in "$out"/php/*.xml; do
	name=$(basename "$f")
	norm "$f" > "$out/a"; norm "$out/go/$name" > "$out/b"
	if diff -q "$out/a" "$out/b" >/dev/null; then
		echo "identical  $name"
	else
		echo "differs    $name ($(diff "$out/a" "$out/b" | grep -c '^<') nodes)"; diff "$out/a" "$out/b" || true; status=1
	fi
done
echo "outputs in $out"
exit $status
