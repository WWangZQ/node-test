package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type fakeTransport func(*http.Request) (*http.Response, error)

func (f fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fakeClient(code int, body string) *http.Client {
	return &http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
}
func TestProbeEvidence(t *testing.T) {
	for _, v := range []struct {
		name, method, body, want string
		code                     int
	}{
		{"Grok", "web", "<title>Just a moment...</title>", "challenge", 200},
		{"Perplexity", "web", strings.Repeat("ok", 100), "denied", 403},
		{"Grok", "web", strings.Repeat("page", 100), "unknown", 200},
		{"Grok", "web", "not available in your country", "locked", 403},
		{"Grok", "web", `{}`, "limited", 429},
		{"Bilibili", "bili", `{}`, "unknown", 200},
		{"Bilibili", "bili", `{"code":0,"result":{"dash":null}}`, "unknown", 200},
		{"Bilibili", "bili", `{"code":0,"result":{"durl":[{"url":"https://media.example"}]}}`, "unlocked", 200},
		{"Bilibili", "bili", `{"code":0,"result":{"durl":[{}]}}`, "unknown", 200},
		{"BBC", "bbc", `{"connection":null,"href":null}`, "unknown", 200},
		{"DAZN", "dazn", `{"Region":{}}`, "unknown", 200},
		{"DAZN", "dazn", `{"Region":{"isAllowed":false}}`, "locked", 200},
		{"Abema", "abema", `{"isoCountryCode":"US"}`, "partial", 200},
		{"MyTVSuper", "mytv", `{}`, "unknown", 200},
		{"Disney+", "legacy", "<html>subscribe bundle</html>", "unknown", 200},
		{"Netflix", "legacy", `{"requestCountry":"JP"}`, "unknown", 200},
		{"OpenAI", "legacy", "<title>Just a moment...</title>", "challenge", 200},
	} {
		t.Run(v.name+v.want, func(t *testing.T) {
			s := probeService(ServiceSpec{ID: v.name, Method: v.method, URL: "https://example.com"}, fakeClient(v.code, v.body))
			if s.Status != v.want {
				t.Fatalf("%+v", s)
			}
			if v.name == "Disney+" && s.Region != "" {
				t.Fatal("fabricated Disney region", s)
			}
		})
	}
}
func decode(s string) map[string]any {
	var m map[string]any
	_ = json.Unmarshal([]byte(s), &m)
	return m
}
func TestRiskMissingAndConflictingSources(t *testing.T) {
	pure := parseRisk("IPPure", "", decode(`{"ip":"203.0.113.1","isResidential":true,"fraudScore":0}`))
	if pure.Score == nil || *pure.Score != 0 || pure.NetworkType != "住宅/家宽" {
		t.Fatal(pure)
	}
	missing := parseRisk("IPPure", "", decode(`{"ip":"203.0.113.1","fraudScore":null}`))
	if missing.Score != nil || missing.NetworkType != "" {
		t.Fatal(missing)
	}
	pc := parseRisk("ProxyCheck", "203.0.113.1", decode(`{"status":"ok","203.0.113.1":{"detections":{"risk":33,"hosting":true,"proxy":false}}}`))
	if pc.Score == nil || *pc.Score != 33 || !pc.Flags["hosting"] || pc.Flags["proxy"] {
		t.Fatal(pc)
	}
	nc := parseRisk("NCGY", "203.0.113.1", decode(`{"ip":"203.0.113.1","proxy":{"is_proxy":true,"is_hosting":false}}`))
	if !nc.Flags["is_proxy"] || nc.Flags["is_hosting"] {
		t.Fatal(nc)
	}
	mismatch := parseRisk("NCGY", "203.0.113.1", decode(`{"ip":"203.0.113.2","proxy":{"is_proxy":true}}`))
	if mismatch.Status == "ok" {
		t.Fatal(mismatch)
	}
}
func TestQuotaAndCache(t *testing.T) {
	count := 0
	c := &http.Client{Transport: fakeTransport(func(req *http.Request) (*http.Response, error) {
		count++
		return &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("limited")), Request: req}, nil
	})}
	r := Row{RiskUpdates: map[string]map[string]RiskSource{}}
	in := Input{ProxyCheckRemaining: 1}
	queryRiskIP(c, in, &r, "203.0.113.1")
	queryRiskIP(c, in, &r, "203.0.113.2")
	if r.ProxyCheckRequests != 1 || !r.ProxyCheckLimited || count != 2 {
		t.Fatalf("requests %d count %d", r.ProxyCheckRequests, count)
	}
	cached := riskBase("ProxyCheck", "203.0.113.3")
	cached.Status = "ok"
	in.RiskCache = map[string]map[string]RiskSource{"203.0.113.3": {"ProxyCheck": cached, "NCGY": cached}}
	v := queryRiskIP(c, in, &r, "203.0.113.3")
	if !v["ProxyCheck"].Cached || count != 2 {
		t.Fatal(v, count)
	}
}
func TestClaudeDoesNotClaimAccountSafety(t *testing.T) {
	s := probeService(ServiceSpec{ID: "Claude", Method: "claude", URL: "https://claude.ai/login"}, fakeClient(403, "Access denied"))
	if s.Status != "denied" || s.ExitIP != "" {
		t.Fatal(s)
	}
	if !strings.Contains(claudeRegionReference("HK"), "未列入") || !strings.Contains(claudeRegionReference("SG"), "已列入") {
		t.Fatal("country reference")
	}
}
