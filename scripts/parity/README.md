# Greenter parity

`run.sh` renders the same documents with [Greenter](https://github.com/thegreenter/greenter)
and with sunat-go and diffs their canonical XML. Last run: 2026-11-12, greenter/lite 5.x.

| Document | Result |
|---|---|
| Resumen diario (RC), Comunicación de baja (RA), Reversión (RR) | identical |
| Percepción (40), Guía de remisión remitente (09) | identical |
| Retención (20) | Greenter double-escapes `&` in the supplier name (Greenter bug) |
| Factura (full: detraction, credit quotas, ISC, ICBPER, free and exonerated lines, discounts), Boleta, NC, ND | only the accepted differences below |

Accepted differences (all valid for SUNAT; the sunat-go factura with them was accepted in beta):

- Prices are always written with at least two decimals (`118.00`), Greenter trims them (`118`).
- `cac:Signature/cbc:ID` is the RUC; Greenter prefixes `SIGN`.
- Greenter double-escapes `cbc:Note` (`&amp;lt;`); sunat-go escapes once.
- Greenter adds `listID`/`listAgencyName` attributes to the delivery country code.

The Liquidación de compra (04) has no Greenter model; it was compared with the
reference PHP builder whose XML SUNAT accepted: identical except the signature id.
