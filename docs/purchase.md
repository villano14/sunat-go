# Liquidación de compra (04)

The buyer (the issuer, with RUC) issues a Liquidación de compra for purchases
from natural persons without RUC (DNI or carné de extranjería). SUNAT expects
UBL 2.1 `SelfBilledInvoice` (customization 2.0), not `Invoice`: the buyer is
`AccountingCustomerParty` and precedes the seller (`AccountingSupplierParty`).

```go
res, err := calc.Settlement(calc.SettlementInput{
    IncomeTaxPercent: decimal.RequireFromString("1.5"), // caller policy, see below
    Lines: []calc.SettlementLineInput{{
        SunatProductCode: "10191509", Description: "CAFE EN GRANO",
        Quantity: decimal.NewFromInt(100), UnitValue: decimal.NewFromInt(10),
    }},
})
ps := res.Apply(model.PurchaseSettlement{
    Series: "L001", Number: 1, IssueDate: now,
    Company:  company,
    Seller:   model.PurchaseSeller{IdentityType: catalog.DNI, Number: "12345678", Name: "JUAN PEREZ",
        Ubigeo: "080101", Line: "CHACRA S/N", AddressType: "01"},
    Location: model.PurchaseLocation{Type: "01", Ubigeo: "080101", Line: "CHACRA S/N"},
})
result, err := client.SendPurchaseSettlement(ctx, ps) // billService, like invoices
```

- Series: `L` plus 3 uppercase alphanumerics. Operation types `0501` (compra
  interna, default), `0502` (anticipos), `0503` (oro; add `model.GoldAttributes`).
- Lines are taxed (10), exonerated (20) or unaffected (30). The document
  `TaxTotal` amount is the IGV; the income tax retained (scheme 3000 `IR`/`TOX`)
  is informative and deducted from `Payable`.
- `calc.Settlement` follows SUNAT rules 4287, 4288, 3103, 2464, 4290, 4346,
  4310 and 4312. The document IGV is computed from the summed taxed base.
- The retention rate is a legal decision left to the caller: 1.5 % in general,
  4 % for gold, 0 when the operation does not exceed S/ 700 or for primary
  agricultural products while their exclusion lasts (RS 234-2005/SUNAT).

Evidence: the canonical XML is identical, except the signature id, to the
reference PHP builder's, whose output SUNAT accepted (code 0). It validates
against the OASIS `UBL-SelfBilledInvoice-2.1.xsd`. No sunat-go beta submission
has been made yet.

## Reversion

Retentions (20), perceptions (40) and purchase settlements (04) are not voided
with a Comunicación de baja: SUNAT reverts them with a Resumen de reversiones
(RR), same `VoidedDocuments` schema, sent with `sendSummary` and polled with
`Client.Status`. `ubl.Voided` rejects these types and `ubl.Reversion` accepts
only them.

```go
others := sunat.New(signer, transport.New(transport.OtrosCPEBeta, creds))
sent, err := others.SendReversion(ctx, model.Reversion{
    Company: company, GenerationDate: issued, IssueDate: today, Correlative: 1,
    Lines: []model.VoidedLine{{Type: catalog.Retention, Series: "R001", Number: 12, Reason: "ERROR EN TASA"}},
})
```

Greenter and the reference application send every reversion to the otros CPE
endpoint. That a reversion of a purchase settlement (04) is accepted there is
not verified in beta yet.
