package ubl

import (
	"fmt"
	"slices"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
)

type despatchView struct {
	model.Despatch
	SignatureID string
	IsSender    bool // Guía de remisión remitente (09)
	IsCarrier   bool // Guía de remisión transportista (31)
	WeightUnit  string
	GrossWeight string
	ItemsWeight string // Empty when the items weight is zero
	Indicators  []string
}

// despatchWeight formats a weight with exactly three decimals.
func despatchWeight(d decimal.Decimal) string { return d.StringFixed(3) }

// Despatch renders a Guía de Remisión Electrónica, remitente (09) or
// transportista (31), as unsigned UBL 2.1 DespatchAdvice XML.
//
// For type 31 the remitente (Despatch.Shipper) is required and the transfer
// reason and transport mode are not rendered. The weight unit defaults to
// "KGM".
func Despatch(d model.Despatch) ([]byte, error) {
	v := despatchView{Despatch: d, SignatureID: SignatureID, WeightUnit: d.Shipment.WeightUnit}
	switch d.Type {
	case catalog.DespatchSender:
		v.IsSender = true
	case catalog.DespatchCarrier:
		v.IsCarrier = true
		if d.Shipper == nil {
			return nil, fmt.Errorf("ubl: despatch %s: shipper (remitente) is required for type %s", d.ID(), d.Type)
		}
	default:
		return nil, fmt.Errorf("ubl: unsupported despatch type %q", string(d.Type))
	}
	if v.WeightUnit == "" {
		v.WeightUnit = "KGM"
	}
	v.GrossWeight = despatchWeight(d.Shipment.GrossWeight)
	if !d.Shipment.ItemsWeight.IsZero() {
		v.ItemsWeight = despatchWeight(d.Shipment.ItemsWeight)
	}
	for _, i := range d.Shipment.Indicators {
		v.Indicators = append(v.Indicators, string(i))
	}
	transshipment := string(catalog.IndicatorScheduledTransshipment)
	if d.Shipment.Transshipment && !slices.Contains(v.Indicators, transshipment) {
		v.Indicators = append(v.Indicators, transshipment)
	}
	return render("despatch.xml.tmpl", v)
}
