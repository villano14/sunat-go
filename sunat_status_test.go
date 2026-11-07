package sunat_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/villano14/sunat-go"
	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/transport"
)

type cpeStatusFake struct {
	batchBillOnly
	ctx    context.Context
	query  [3]string
	number int64
	calls  int
	status transport.CPEStatus
	err    error
}

func (f *cpeStatusFake) GetStatusCpe(ctx context.Context, ruc, typ, series string, number int64) (transport.CPEStatus, error) {
	f.ctx, f.query, f.number = ctx, [3]string{ruc, typ, series}, number
	f.calls++
	return f.status, f.err
}

func TestQueryStatus(t *testing.T) {
	for _, code := range []string{"0001", "0002", "0003", "0011", "0099", "0", "1", ""} {
		t.Run(code, func(t *testing.T) {
			f := &cpeStatusFake{status: transport.CPEStatus{Code: code, Message: "estado"}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			got, err := sunat.New(nil, f).QueryStatus(ctx, "20000000001", catalog.Invoice, "F022", 7782)
			if err != nil {
				t.Fatal(err)
			}
			if got.StatusCode != code || got.Message != "estado" || got.Accepted() != (code == "0001") || got.Rejected() != (code == "0002") || got.Voided() != (code == "0003") || got.NotFound() != (code == "0011") {
				t.Fatalf("result = %+v", got)
			}
			if f.ctx != ctx || f.query != [3]string{"20000000001", "01", "F022"} || f.number != 7782 || f.calls != 1 {
				t.Fatalf("forwarding = %+v", f)
			}
		})
	}
}

func TestQueryStatusErrorIdentityAndOptionalInterface(t *testing.T) {
	for _, want := range []error{context.Canceled, transport.ErrNoConsultEndpoint, &transport.Error{Code: "0125"}, &transport.HTTPError{StatusCode: 502}} {
		f := &cpeStatusFake{err: want}
		got, err := sunat.New(nil, f).QueryStatus(context.Background(), "20000000001", catalog.Invoice, "F001", 1)
		if !errors.Is(err, want) || !reflect.DeepEqual(got, sunat.DocumentStatusResult{}) || got.Accepted() {
			t.Fatalf("result=%+v err=%v", got, err)
		}
	}
	// Existing consumer implementations still satisfy the old interfaces but
	// need not implement the new status-only capability.
	var legacy transport.ConsultTransport = &batchFake{}
	_ = legacy
	for _, tr := range []transport.Transport{nil, batchBillOnly{}, &batchFake{}} {
		got, err := sunat.New(nil, tr).QueryStatus(context.Background(), "20000000001", catalog.Invoice, "F001", 1)
		if !errors.Is(err, sunat.ErrStatusConsultUnsupported) || got.Accepted() {
			t.Fatalf("result=%+v err=%v", got, err)
		}
	}
}

func TestStatusAndQueryCDRRemainDistinct(t *testing.T) {
	// A consult accepted code cannot accept a batch ticket or stand in for a CDR.
	fake := &batchFake{status: transport.Status{Code: "0001"}, consult: transport.Status{Code: "0001"}}
	c := sunat.New(nil, fake)
	batch, err := c.Status(context.Background(), "ticket")
	if err == nil || batch.Accepted() || batch.StatusCode != "0001" {
		t.Fatalf("batch=%+v err=%v", batch, err)
	}
	query, err := c.QueryCDR(context.Background(), "20000000001", catalog.Invoice, "F001", 1)
	if err != nil || query.Found() || query.Accepted() || query.StatusCode != "0001" {
		t.Fatalf("CDR=%+v err=%v", query, err)
	}
}
