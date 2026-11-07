#!/bin/sh
# Download XSDs used by `go test ./ubl -run TestSummaryPerceptionXSD` into
# ./testdata-xsd (git-ignored). Idempotent: files already present are kept.
#
# Layout (the layout the test expects):
#
#   testdata-xsd/2.1/common/    OASIS UBL 2.1 common components + xmldsig
#   testdata-xsd/2.1/maindoc/   OASIS Invoice, CreditNote, DebitNote, DespatchAdvice
#   testdata-xsd/2.0/common/    OASIS UBL 2.0 components + SUNAT SunatAggregateComponents
#   testdata-xsd/2.0/maindoc/   SUNAT SummaryDocuments, VoidedDocuments, Retention, Perception
#
# Sources:
#   2.1  https://docs.oasis-open.org/ubl/os-UBL-2.1/xsd/        (official OASIS release)
#   2.0  https://github.com/thegreenter/ubl-validator            (copies of the SUNAT schemas
#        distributed by Greenter; the SUNAT-specific documents are built on UBL 2.0)
#
# Usage: scripts/fetch-xsd.sh [target-dir]      (default: <repo root>/testdata-xsd)
# Override the pinned Greenter revision with UBL_VALIDATOR_REF=<commit|branch>.

set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
dest=${1:-$root/testdata-xsd}

oasis=https://docs.oasis-open.org/ubl/os-UBL-2.1/xsd
ref=${UBL_VALIDATOR_REF:-980bad9412eab51516e4879b06d8d2e723c62f0d}
greenter=https://raw.githubusercontent.com/thegreenter/ubl-validator/$ref/src/xsd/2.0

fetch() { # fetch <url> <file>
	if [ -s "$2" ]; then
		return 0
	fi
	mkdir -p "$(dirname "$2")"
	echo "fetch $2"
	curl --fail --silent --show-error --location --retry 3 --output "$2.part" "$1"
	mv "$2.part" "$2"
}

for f in \
	CCTS_CCT_SchemaModule-2.1.xsd \
	UBL-CommonAggregateComponents-2.1.xsd \
	UBL-CommonBasicComponents-2.1.xsd \
	UBL-CommonExtensionComponents-2.1.xsd \
	UBL-CommonSignatureComponents-2.1.xsd \
	UBL-CoreComponentParameters-2.1.xsd \
	UBL-ExtensionContentDataType-2.1.xsd \
	UBL-QualifiedDataTypes-2.1.xsd \
	UBL-SignatureAggregateComponents-2.1.xsd \
	UBL-SignatureBasicComponents-2.1.xsd \
	UBL-UnqualifiedDataTypes-2.1.xsd \
	UBL-XAdESv132-2.1.xsd \
	UBL-XAdESv141-2.1.xsd \
	UBL-xmldsig-core-schema-2.1.xsd; do
	fetch "$oasis/common/$f" "$dest/2.1/common/$f"
done

for doc in Invoice CreditNote DebitNote DespatchAdvice; do
	fetch "$oasis/maindoc/UBL-$doc-2.1.xsd" "$dest/2.1/maindoc/UBL-$doc-2.1.xsd"
done

for f in \
	CCTS_CCT_SchemaModule-2.0.xsd \
	CodeList_CurrencyCode_ISO_7_04.xsd \
	CodeList_LanguageCode_ISO_7_04.xsd \
	CodeList_MIMEMediaTypeCode_IANA_7_04.xsd \
	CodeList_UnitCode_UNECE_7_04.xsd \
	UBL-CommonAggregateComponents-2.0.xsd \
	UBL-CommonBasicComponents-2.0.xsd \
	UBL-CommonExtensionComponents-2.0.xsd \
	UBL-CoreComponentParameters-2.0.xsd \
	UBL-ExtensionContentDatatype-2.0.xsd \
	UBL-QualifiedDatatypes-2.0.xsd \
	UBLPE-SunatAggregateComponents-1.0.xsd \
	UBLPE-SunatAggregateComponents-1.1.xsd \
	UnqualifiedDataTypeSchemaModule-2.0.xsd \
	xmldsig-core-schema.xsd; do
	fetch "$greenter/common/$f" "$dest/2.0/common/$f"
done

for doc in SummaryDocuments VoidedDocuments Retention Perception; do
	fetch "$greenter/maindoc/UBL-$doc-2.0.xsd" "$dest/2.0/maindoc/UBL-$doc-2.0.xsd"
done

echo "XSD set ready in $dest"
