package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNewRiskSources(t *testing.T) {
	for _, x := range []struct {
		name, body string
		want       string
	}{
		{"ipapi.is", `{"ip":"203.0.113.1","is_datacenter":true,"is_abuser":false,"company":{"type":"hosting","abuser_score":"0.02 (Low)"}}`, "ok"},
		{"ipapi.is", `{"ip":"203.0.113.1","is_datacenter":null,"is_proxy":null}`, "partial"},
		{"ipapi.is", `{"ip":"203.0.113.2","is_datacenter":true}`, "unknown"},
		{"Scamalytics", `{"scamalytics":{"status":"ok","ip":"203.0.113.1","scamalytics_score":0,"scamalytics_risk":"low","scamalytics_proxy":{"is_datacenter":true,"is_vpn":null,"is_resproxy":"PREMIUM FIELD - upgrade to view"}},"external_datasources":{"x4bnet":{"is_tor":true}}}`, "ok"},
		{"Scamalytics", `{"scamalytics":{"status":"error","error":"Excess limit: credits exhausted secret"}}`, "limited"},
		{"AbuseIPDB", `{"data":{"ipAddress":"203.0.113.1","abuseConfidenceScore":0,"totalReports":0,"numDistinctUsers":0,"usageType":"Data Center/Web Hosting/Transit","lastReportedAt":null}}`, "ok"},
		{"IPQS", `{"success":true,"fraud_score":92,"proxy":true,"recent_abuse":true}`, "ok"},
		{"IPQS", `{"success":false,"message":"invalid key secret"}`, "auth"},
	} {
		t.Run(x.name+x.want, func(t *testing.T) {
			r := parseRisk(x.name, "203.0.113.1", decode(x.body))
			if r.Status != x.want {
				t.Fatalf("%+v", r)
			}
			b, _ := json.Marshal(r)
			if strings.Contains(string(b), "secret") || strings.Contains(string(b), "PREMIUM FIELD") {
				t.Fatal("raw error/placeholder leaked")
			}
			if x.name == "Scamalytics" && r.Status == "ok" {
				if _, ok := r.Flags["is_vpn"]; ok {
					t.Fatal("null became false")
				}
				if r.Score == nil || *r.Score != 0 || r.Flags["is_datacenter"] != true {
					t.Fatal(r)
				}
			}
		})
	}
}
func TestRiskKeysAndTargetRouting(t *testing.T) {
	calls := map[string]int{}
	c := routedClient(func(r *http.Request) (int, string, error) {
		calls[r.URL.Host]++
		if r.URL.Host == "api.abuseipdb.com" {
			if r.Header.Get("Key") != "fixture-secret" || r.URL.Query().Get("ipAddress") != "203.0.113.1" {
				t.Fatal("wrong query/header")
			}
			return 200, `{"data":{"ipAddress":"203.0.113.1","abuseConfidenceScore":0,"totalReports":0}}`, nil
		}
		if r.URL.Host == "ipqualityscore.com" {
			return 200, `{"success":true,"fraud_score":0,"proxy":true}`, nil
		}
		return 429, "", nil
	})
	in := Input{ProxyCheckRemaining: 100, RiskKeys: map[string]string{"AbuseIPDB": "fixture-secret", "IPQS": "fixture-secret"}}
	r := Row{}
	v := queryRiskIP(c, in, &r, "203.0.113.1")
	queryRiskIP(c, in, &r, "203.0.113.1")
	if calls["api.abuseipdb.com"] != 1 || calls["ipqualityscore.com"] != 1 || v["ipapi.is"].Status != "not_configured" {
		t.Fatal(calls, v)
	}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "fixture-secret") {
		t.Fatal("credential leaked into report")
	}
	if v["AbuseIPDB"].IP != "203.0.113.1" {
		t.Fatal(v)
	}
}
func TestGoogleIndependentAndExplicitEvidence(t *testing.T) {
	s := classifyGoogle(response(200, `<div id="search">results</div><div id="fbar">Hong Kong</div>`))
	if s.Status != "unlocked" || s.Region != "HK" || s.ExitIP != "" {
		t.Fatal(s)
	}
	g := classifyAI("Gemini", response(200, `<script>{"45631641":true};["",2,1,200,"TWN"]</script>`))
	if g.Region == s.Region || g.Region != "TW" {
		t.Fatal(g)
	}
	challenge := classifyGoogle(response(429, `Our systems have detected unusual traffic. IP address: 203.0.113.1`))
	if challenge.Status != "challenge" || challenge.ExitIP != "203.0.113.1" {
		t.Fatal(challenge)
	}
	empty := classifyGoogle(response(200, `<title>Google Search</title>`))
	if empty.Status == "unlocked" {
		t.Fatal("generic HTTP 200 became unlocked")
	}
}
func TestDefaultServices(t *testing.T) {
	want := map[string]bool{}
	for _, n := range []string{"OpenAI", "Gemini", "Claude", "Grok", "Perplexity", "Copilot", "Meta AI", "Disney+", "Netflix", "YouTube", "Spotify", "Apple TV+", "TikTok", "Steam", "Wikipedia", "Google"} {
		want[n] = true
	}
	got := selectedSpecs(nil)
	if len(got) != len(want) {
		t.Fatal(len(got))
	}
	for _, s := range got {
		if !want[s.ID] {
			t.Fatal(s)
		}
	}
}

func TestLivePublicRisk(t *testing.T) {
	if os.Getenv("NODE_TEST_LIVE") != "1" {
		t.Skip("opt-in public API checks")
	}
	c := &http.Client{Timeout: 10 * time.Second}
	for _, x := range []struct{ n, ip, u string }{{"IPPure", "", "https://my.ippure.com/v1/info"}, {"ProxyCheck", "1.1.1.1", "https://proxycheck.io/v3/1.1.1.1"}, {"NCGY", "1.1.1.1", "https://ip.nc.gy/json?ip=1.1.1.1"}} {
		r := requestRisk(c, x.n, x.ip, x.u)
		t.Logf("%s status=%s score_present=%v flag_fields=%d", x.n, r.Status, r.Score != nil, len(r.Flags))
		if r.Status == "ok" && r.IP == "" {
			t.Fatal("success without target")
		}
	}
}
