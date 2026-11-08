package archive

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeCloudflare serves zones (two pages), DNS records, Registrar and RDAP.
func fakeCloudflare(t *testing.T) {
	t.Helper()
	ok := func(w http.ResponseWriter, result any, page, total int) {
		json.NewEncoder(w).Encode(map[string]any{"success": true, "errors": []any{}, "result": result,
			"result_info": map[string]int{"page": page, "total_pages": total}})
	}
	zone := func(id, name string) map[string]any {
		return map[string]any{"id": id, "name": name, "status": "active", "name_servers": []string{"ada.ns.cloudflare.com"}, "account": map[string]string{"id": "acc1"}}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/client/v4/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer cf-token" {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]any{"success": false, "errors": []map[string]any{{"code": 9109, "message": "Invalid access token"}}})
			return
		}
		switch path := strings.TrimPrefix(r.URL.Path, "/client/v4"); path {
		case "/zones":
			if r.URL.Query().Get("page") == "1" {
				ok(w, []any{zone("z1", "heartie.example")}, 1, 2)
			} else {
				ok(w, []any{zone("z2", "thumbsnap.example"), zone("z3", "newsite.example")}, 2, 2)
			}
		case "/zones/z1/dns_records":
			ok(w, []any{
				map[string]any{"name": "heartie.example", "type": "A", "content": "203.0.113.99", "proxied": true},
				map[string]any{"name": "admin.heartie.example", "type": "CNAME", "content": "heartie.example"},
			}, 1, 1)
		case "/zones/z2/dns_records", "/zones/z3/dns_records":
			ok(w, []any{}, 1, 1)
		case "/accounts/acc1/registrar/domains":
			ok(w, []any{map[string]any{"id": "x1", "name": "heartie.example", "expires_at": "2027-06-01T10:00:00Z", "auto_renew": true}}, 1, 1)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/rdap/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/thumbsnap.example") {
			json.NewEncoder(w).Encode(map[string]any{"events": []map[string]string{{"eventAction": "registration", "eventDate": "2020-01-01T00:00:00Z"}, {"eventAction": "expiration", "eventDate": "2026-12-20T23:59:59Z"}}})
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	oldAPI, oldRDAP, oldCert := cloudflareAPI, rdapBase, certFetch
	cloudflareAPI, rdapBase = srv.URL+"/client/v4", srv.URL+"/rdap/domain/"
	certFetch = func(_ context.Context, host string) ObservedCert {
		if host == "admin.heartie.example" {
			return ObservedCert{Host: host, NotAfter: "2026-11-20", Issuer: "Let's Encrypt", Valid: true}
		}
		return ObservedCert{Host: host, NotAfter: "2027-01-30", Issuer: "Google Trust Services", Valid: true}
	}
	t.Cleanup(func() { cloudflareAPI, rdapBase, certFetch = oldAPI, oldRDAP, oldCert })
	t.Setenv("CF_TOKEN", "cf-token")
}

func TestScanAndDiff(t *testing.T) {
	fakeCloudflare(t)
	dir := copyExample(t)
	a := load(t, dir)
	res, err := a.Scan(context.Background(), ScanInput{Kinds: []string{"domain"}, TokenRef: "env:CF_TOKEN", Now: fixedNow, Collector: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Observations) != 2 || strings.Join(res.Unregistered, ",") != "newsite.example" || len(res.Notes) != 0 {
		t.Fatalf("result: %+v", res)
	}
	o, err := a.ReadObservation("domain:heartie.example")
	if err != nil || o == nil {
		t.Fatalf("observation: %v", err)
	}
	f := o.Domain
	if f.ExpiresAt != "2027-06-01" || f.ExpirySource != "cloudflare-registrar" || f.AutoRenew == nil || !*f.AutoRenew || f.Registrar != "provider:cloudflare" {
		t.Fatalf("heartie facts: %+v", f)
	}
	if len(f.Records) != 2 || f.Records[0].Name != "@" || f.Records[1].Name != "admin" {
		t.Fatalf("records: %+v", f.Records)
	}
	if len(f.Certificates) != 2 || f.Certificates[0].Host != "admin.heartie.example" {
		t.Fatalf("certificate hosts: %+v", f.Certificates)
	}
	th, _ := a.ReadObservation("domain:thumbsnap.example")
	if th.Domain.ExpiresAt != "2026-12-20" || th.Domain.ExpirySource != "rdap" || th.Domain.Registrar != "" {
		t.Fatalf("thumbsnap facts: %+v", th.Domain)
	}
	if _, err := os.Stat(filepath.Join(dir, ObservedDir, "providers", "cloudflare.json")); err != nil {
		t.Fatal(err)
	}

	drift, err := a.Diff(nil, "", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	find := func(ref, field string) *Drift {
		for i := range drift {
			if drift[i].Ref == ref && drift[i].Field == field {
				return &drift[i]
			}
		}
		return nil
	}
	if d := find("domain:heartie.example", "spec.renewal.expiresAt"); d == nil || d.Archive != "2027-05-01" || d.Observed != "2027-06-01" {
		t.Fatalf("expiry drift: %+v", drift)
	}
	if d := find("domain:heartie.example", "spec.records[0]"); d == nil || d.Observed != "203.0.113.99" {
		t.Fatalf("record drift: %+v", drift)
	}
	if find("domain:heartie.example", "spec.records[1]") != nil {
		t.Fatalf("matching CNAME reported: %+v", drift)
	}
	if d := find("domain:heartie.example", "certificate admin.heartie.example"); d == nil || d.Severity != "warn" {
		t.Fatalf("certificate drift: %+v", drift)
	}
	if find("domain:thumbsnap.example", "spec.registrar") != nil || find("domain:thumbsnap.example", "spec.renewal.expiresAt") != nil {
		t.Fatalf("thumbsnap should match: %+v", drift)
	}
	if d := find("domain:newsite.example", ""); d == nil || d.Severity != "info" {
		t.Fatalf("unregistered zone: %+v", drift)
	}
	rows := a.RenewalsWithObserved(fixedNow, 30)
	if len(rows) != 2 || rows[0].What != "certificate admin.heartie.example (observed)" {
		t.Fatalf("renewals with observations: %+v", rows)
	}
	// Observations never affect validation or INDEX.md.
	if b := load(t, dir); len(b.Errors) > 0 || len(b.Warnings) > 0 {
		t.Fatalf("observations changed validation: %v %v", b.Errors, b.Warnings)
	}
}

func TestScanWithoutTokenStillChecksExpiryAndCertificates(t *testing.T) {
	fakeCloudflare(t)
	a := load(t, copyExample(t))
	os.Remove(filepath.Join(a.Dir, ".gitignore"))
	res, err := a.Scan(context.Background(), ScanInput{Refs: []string{"thumbsnap.example"}, Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	if !gitignored(a.Dir) {
		t.Fatal("scan did not protect .devopspad/ from commits")
	}
	if len(res.Notes) != 2 || !strings.Contains(res.Notes[1], "Cloudflare skipped") {
		t.Fatalf("notes: %v", res.Notes)
	}
	f := res.Observations[0].Domain
	if f.ExpiresAt != "2026-12-20" || len(f.Certificates) != 1 || f.DNS != "" {
		t.Fatalf("facts: %+v", f)
	}
	if _, err := a.Scan(context.Background(), ScanInput{Refs: []string{"product:heartie"}}); err == nil {
		t.Fatal("product ref accepted")
	}
	if _, err := a.Scan(context.Background(), ScanInput{Kinds: []string{"domain"}, TokenRef: "env:WRONG"}); err != nil {
		t.Fatalf("missing token must degrade, got %v", err)
	}
	t.Setenv("WRONG", "bad-token")
	if _, err := a.Scan(context.Background(), ScanInput{Kinds: []string{"domain"}, TokenRef: "env:WRONG"}); err == nil || !strings.Contains(err.Error(), "Invalid access token") {
		t.Fatalf("rejected token: %v", err)
	}
}

func TestImportCloudflare(t *testing.T) {
	fakeCloudflare(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, RootFile), []byte("apiVersion: devopspad.io/v1alpha1\nkind: Archive\nmetadata:\n  name: t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	in := ImportInput{Cloudflare: true, TokenRef: "env:CF_TOKEN"}
	res, err := Import(dir, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 4 {
		t.Fatalf("files: %+v", res.Files)
	}
	a := load(t, dir)
	if len(a.Errors) > 0 {
		t.Fatalf("imported archive invalid: %v", a.Errors)
	}
	p := a.Get("provider:cloudflare").Object.(*Provider)
	if p.Spec.Credentials[0].Ref != "env:CF_TOKEN" {
		t.Fatalf("provider: %+v", p.Spec)
	}
	h := a.Get("domain:heartie.example").Object.(*Domain).Spec
	if h.Registrar != "provider:cloudflare" || h.Renewal.ExpiresAt != "2027-06-01" || !h.Renewal.AutoRenew {
		t.Fatalf("heartie: %+v", h)
	}
	th := a.Get("domain:thumbsnap.example").Object.(*Domain).Spec
	if th.Registrar != "" || th.Renewal.ExpiresAt != "2026-12-20" || th.DNS != "provider:cloudflare" {
		t.Fatalf("thumbsnap: %+v", th)
	}
	if a.Get("domain:newsite.example").Object.(*Domain).Spec.Renewal != nil {
		t.Fatal("newsite has no known expiry")
	}
	todos := strings.Join(res.Todos, "\n")
	for _, want := range []string{"2 domains: not in Cloudflare Registrar data", "1 domains: auto-renew unknown", "1 domains: expiry date not found", "link the 3 domains"} {
		if !strings.Contains(todos, want) {
			t.Fatalf("todos missing %q:\n%s", want, todos)
		}
	}
	// Once the provider exists its credential is used, and a repeat import changes nothing.
	again, err := Import(dir, ImportInput{Cloudflare: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range again.Files {
		if f.Action != "unchanged" {
			t.Fatalf("not idempotent: %+v", f)
		}
	}
}
