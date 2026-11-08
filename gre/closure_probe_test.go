package gre_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/villano14/sunat-go/gre"
)

// S1-S3: reserved characters must survive OAuth form encoding; parsed errors
// must redact credentials in codes, descriptions and nested gateway details.
func TestClosureOAuthEncodingAndRedaction(t *testing.T) {
	for _, stage := range []string{"token", "api"} {
		t.Run(stage, func(t *testing.T) {
			creds := gre.Credentials{ClientID: "closure-client", ClientSecret: "S+&= ñ", RUC: "20000000001", User: "U+&=日", Password: "P+&= 日"}
			var tokenCalls, apiCalls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/oauth2/token/") {
					tokenCalls.Add(1)
					if r.Method != "POST" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
						t.Error("unexpected OAuth request metadata")
					}
					if err := r.ParseForm(); err != nil {
						t.Error(err)
					}
					want := url.Values{"grant_type": {"password"}, "scope": {gre.Scope}, "client_id": {creds.ClientID}, "client_secret": {creds.ClientSecret}, "username": {creds.RUC + creds.User}, "password": {creds.Password}}
					if !reflect.DeepEqual(r.PostForm, want) {
						t.Error("OAuth form did not round-trip")
					}
					if stage == "token" {
						w.WriteHeader(401)
						_ = json.NewEncoder(w).Encode(map[string]string{"error": creds.ClientSecret, "error_description": creds.Password})
						return
					}
					fmt.Fprint(w, `{"access_token":"closure-token","expires_in":3600}`)
					return
				}
				apiCalls.Add(1)
				if r.Header.Get("Authorization") != "Bearer closure-token" {
					t.Error("missing bearer token")
				}
				w.WriteHeader(422)
				_ = json.NewEncoder(w).Encode(map[string]any{"cod": creds.ClientSecret, "msg": creds.Password, "errors": []map[string]string{{"cod": creds.Password, "msg": creds.ClientSecret}}})
			}))
			defer srv.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := gre.New(creds, gre.WithEndpoints(srv.URL, srv.URL)).Status(ctx, "ticket")
			var got *gre.Error
			if !errors.As(err, &got) {
				t.Fatalf("error = %v; want *gre.Error", err)
			}
			wantStatus, wantMessage, wantAPI := 401, "[redacted]", int32(0)
			if stage == "api" {
				wantStatus, wantMessage, wantAPI = 422, "[redacted]: [redacted] [redacted]", 1
			}
			if got.HTTPStatus != wantStatus || got.Code != "[redacted]" || got.Message != wantMessage || got.Error() != "gre: [redacted]: "+wantMessage {
				t.Fatalf("unexpected sanitized error: %+v", got)
			}
			if tokenCalls.Load() != 1 || apiCalls.Load() != wantAPI {
				t.Fatalf("calls token=%d api=%d", tokenCalls.Load(), apiCalls.Load())
			}
		})
	}
}

// S1-S3: all requests have already used the revoked token before renewal is
// released. Late 401s must reuse the replacement, not invalidate it again.
func TestClosureConcurrent401SharesRenewal(t *testing.T) {
	const workers = 20
	var tokens, api, stale atomic.Int32
	var revoked atomic.Bool
	arrivals, release := make(chan struct{}, workers), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/oauth2/token/") {
			n := tokens.Add(1)
			fmt.Fprintf(w, `{"access_token":"token-%d","expires_in":3600}`, n)
			return
		}
		api.Add(1)
		auth := r.Header.Get("Authorization")
		if revoked.Load() && auth == "Bearer token-1" {
			stale.Add(1)
			arrivals <- struct{}{}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			w.WriteHeader(401)
			fmt.Fprint(w, `{"cod":"401","msg":"Unauthorized"}`)
			return
		}
		if auth != "Bearer token-1" && auth != "Bearer token-2" {
			t.Error("unexpected bearer token")
		}
		fmt.Fprint(w, `{"codRespuesta":"98"}`)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c := gre.New(gre.Credentials{ClientID: "id", ClientSecret: "secret"}, gre.WithEndpoints(srv.URL, srv.URL))
	if _, err := c.Status(ctx, "prime"); err != nil {
		t.Fatal(err)
	}
	revoked.Store(true)
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st, err := c.Status(ctx, "ticket")
			if err == nil && !st.InProcess() {
				err = fmt.Errorf("status = %+v", st)
			}
			errs <- err
		}()
	}
	for i := 0; i < workers; i++ {
		select {
		case <-arrivals:
		case <-ctx.Done():
			t.Fatal("not all revoked-token requests arrived")
		}
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if tokens.Load() != 2 || api.Load() != 1+2*workers || stale.Load() != workers {
		t.Fatalf("tokens=%d api=%d stale=%d; want 2,%d,%d", tokens.Load(), api.Load(), stale.Load(), 1+2*workers, workers)
	}
}
