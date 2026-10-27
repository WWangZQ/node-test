package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKnownIPLookupUsesIndependentTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ip":"203.0.113.1","is_datacenter":false,"company":{"type":"isp"}}`))
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	c := makeLookupClient(ctx)
	defer c.CloseIdleConnections()
	v := requestRisk(c, "ipapi.is", "203.0.113.1", server.URL)
	if v.Status != "ok" || v.IP != "203.0.113.1" {
		t.Fatal(v)
	}
	cancel()
	if requestRisk(c, "ipapi.is", "203.0.113.1", server.URL).Status == "ok" {
		t.Fatal("cancelled lookup continued")
	}
}
func TestPrivateMessageURLRejectedBeforeRequest(t *testing.T) {
	calls := 0
	c := routedClient(func(r *http.Request) (int, string, error) { calls++; return 429, "", nil })
	in := Input{RiskKeys: map[string]string{"Scamalytics": "https://scamalytics.com/pvt/test#secret", "scam_user": "name"}}
	r := Row{}
	out := queryRiskIP(c, in, &r, "203.0.113.1")
	if out["Scamalytics"].Status != "config" || r.RiskStopped["Scamalytics"] != "config" {
		t.Fatal(out)
	}
	if calls != 1 {
		t.Fatalf("only NCGY should be queried, got %d", calls)
	}
}
func TestScamalytics404StopsAccountRetries(t *testing.T) {
	c := routedClient(func(r *http.Request) (int, string, error) { return 404, "", nil })
	r := requestRisk(c, "Scamalytics", "203.0.113.1", "https://api11.scamalytics.com/v3/fixture")
	if r.Status != "config" {
		t.Fatal(r)
	}
}
func TestDownloadRejectsSuccessHTMLAndJSON(t *testing.T) {
	for _, ct := range []string{"text/html", "application/json", "text/plain"} {
		r := &http.Response{Header: http.Header{"Content-Type": []string{ct}}}
		if binaryResponse(r, []byte("I just served you 10mb")) {
			t.Fatal(ct)
		}
	}
	r := &http.Response{Header: make(http.Header)}
	if binaryResponse(r, []byte("<!doctype html>challenge")) {
		t.Fatal("HTML sniffer")
	}
	if !binaryResponse(r, make([]byte, 512)) {
		t.Fatal("binary fixture rejected")
	}
}
func TestSpeedSourceSelection(t *testing.T) {
	ts := speedTargets(Input{Server: "https://speed.cloudflare.com", Megabytes: 32}, false)
	if len(ts) != 3 || ts[1].name != "OVH" || ts[2].name != "Vultr SG" {
		t.Fatal(ts)
	}
	if len(speedTargets(Input{Server: "http://127.0.0.1:1234"}, false)) != 1 {
		t.Fatal("custom fixture must stay local")
	}
}
