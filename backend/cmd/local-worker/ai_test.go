package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func response(code int, body string) responseData {
	return responseData{Code: code, Body: body, URL: "https://service.example/", Header: http.Header{}}
}
func TestAIServiceVerdictsAndRegions(t *testing.T) {
	for _, v := range []struct {
		name, body, want, reg string
		code                  int
	}{
		{"Gemini", `<script>var a={\"45631641\":true};var b=["",2,1,200,"USA"];</script>`, "unlocked", "US", 200},
		{"Gemini", `<script>[45631641,null,true];["",2,1,200,"TWN"]</script>`, "unlocked", "TW", 200},
		{"Gemini", `<script>{"45631641":false};["",2,1,200,"CHN"]</script>`, "unknown", "CN", 200},
		{"Gemini", `<p>Gemini isn't available in your country</p><script>["",2,1,200,"HKG"]</script>`, "locked", "HK", 200},
		{"Gemini", `<script>["",2,1,200,"SGP"]</script><p>该地区无法使用</p>`, "locked", "SG", 200},
		{"Grok", `<title>Grok</title><script>self.x={\"countryCode\":\"SG\"}</script><p>Ask anything</p>`, "unlocked", "SG", 200},
		{"Grok", `<title>Grok</title><script>var translations={"error":"not available in your country"};var initialData={"countryCode":"US"};</script>`, "unlocked", "US", 200},
		{"Perplexity", `<title>Perplexity</title><script src="https://www.perplexity.ai/assets/app.js"></script>`, "unlocked", "", 200},
		{"Claude", `<title>Claude</title><script>{\"ipCountry\":\"TW\"}</script><p>Continue with Google</p>`, "unlocked", "TW", 200},
		{"Claude", `<title>Claude</title><p>Claude is not available in your region</p><script>{"ipCountry":"HK"}</script>`, "locked", "HK", 200},
		{"Grok", `<title>Just a moment...</title>`, "challenge", "", 403},
		{"Grok", `<h1>Access denied</h1>`, "denied", "", 403},
	} {
		t.Run(v.name+v.want+v.reg, func(t *testing.T) {
			s := classifyAI(v.name, response(v.code, v.body))
			if s.Status != v.want || s.Region != v.reg {
				t.Fatalf("got %+v want %s %s", s, v.want, v.reg)
			}
		})
	}
}
func TestOpenAIAppSpecific403(t *testing.T) {
	for _, v := range []struct {
		body, want string
		code       int
		app        bool
	}{
		{`{"cookie_consent_required":false}`, "unlocked", 200, false},
		{`{"cf_details":"Request is not allowed. Please try again later.","type":"dc"}`, "unlocked", 403, true},
		{`{"error":{"code":"unsupported_country_region_territory"}}`, "locked", 200, false},
		{`<title>Just a moment...</title>`, "challenge", 403, true},
		{`<p>Sorry, you have been blocked</p>`, "locked", 403, true},
		{`{}`, "unknown", 200, false},
	} {
		s := classifyOpenAI(response(v.code, v.body), v.app)
		if s.Status != v.want {
			t.Fatalf("%+v", s)
		}
	}
}
func routedClient(fn func(*http.Request) (int, string, error)) *http.Client {
	return &http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
		code, body, e := fn(r)
		if e != nil {
			return nil, e
		}
		return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
}
func TestOpenAICombinedAndTrace(t *testing.T) {
	c := routedClient(func(r *http.Request) (int, string, error) {
		switch r.URL.Host {
		case "api.openai.com":
			return 403, `{"error":{"code":"unsupported_country"}}`, nil
		case "ios.chat.openai.com":
			return 403, `{"cf_details":"Request is not allowed. Please try again later."}`, nil
		default:
			return 200, "h=chatgpt.com\nip=203.0.113.1\nloc=HK\n", nil
		}
	})
	s := probeOpenAI(c)
	if s.Status != "partial" || s.Label != "仅APP" || s.Region != "HK" || s.RegionKind != "egress" || len(s.Parts) != 2 {
		t.Fatal(s)
	}
}
func TestChallengeKeepsTraceRegion(t *testing.T) {
	c := routedClient(func(r *http.Request) (int, string, error) {
		if r.URL.Path == "/cdn-cgi/trace" {
			return 200, "h=grok.com\nip=203.0.113.1\nloc=TW\n", nil
		}
		return 403, "<title>Just a moment...</title>", nil
	})
	s := probeAI(ServiceSpec{ID: "Grok", URL: "https://grok.com/"}, c)
	if s.Status != "challenge" || s.Region != "TW" || s.Connectivity != "connected" {
		t.Fatal(s)
	}
}
func TestNetworkFailureDoesNotBecomeAvailableFromTrace(t *testing.T) {
	c := routedClient(func(r *http.Request) (int, string, error) {
		if r.URL.Path == "/cdn-cgi/trace" {
			return 200, "h=grok.com\nip=203.0.113.1\nloc=TW\n", nil
		}
		return 0, "", errors.New("dial failed")
	})
	s := probeAI(ServiceSpec{ID: "Grok", URL: "https://grok.com/"}, c)
	if s.Status != "error" || s.Region != "TW" || s.Connectivity != "failed" {
		t.Fatal(s)
	}
}
func TestNetflixMetadataAndDisneySupported(t *testing.T) {
	n := probeNetflix(fakeClient(200, `<meta property="og:video" content="https://www.netflix.com/watch/81280792"><script>{"requestCountry":{"id":"JP"}}</script>`))
	if n.Status != "unlocked" || n.Region != "JP" {
		t.Fatal(n)
	}
	c := routedClient(func(r *http.Request) (int, string, error) {
		switch r.URL.Path {
		case "/devices":
			return 200, `{"assertion":"fixture"}`, nil
		case "/token":
			return 200, `{"refresh_token":"fixture"}`, nil
		default:
			return 200, `{"extensions":{"sdk":{"session":{"location":{"countryCode":"TW"},"inSupportedLocation":true}}}}`, nil
		}
	})
	d := probeDisney(c)
	if d.Status != "unlocked" || d.Region != "TW" {
		t.Fatal(d)
	}
	encoded, _ := json.Marshal(d)
	if strings.Contains(string(encoded), "fixture") {
		t.Fatal("anonymous tokens leaked")
	}
}
func TestLiveAIProbe(t *testing.T) {
	if os.Getenv("NODE_TEST_LIVE") != "1" {
		t.Skip("opt-in public endpoints only")
	}
	var wg sync.WaitGroup
	for _, spec := range catalog() {
		if !isAI(spec.ID) || spec.ID == "Meta AI" {
			continue
		}
		wg.Add(1)
		go func(spec ServiceSpec) {
			defer wg.Done()
			c := &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}, Timeout: 8 * time.Second}
			s := probeAI(spec, c)
			t.Logf("%s: status=%s label=%s region=%s source=%s details=%s", spec.ID, s.Status, s.Label, s.Region, s.RegionSource, s.Detail)
		}(spec)
	}
	wg.Wait()
}
func TestMediaSpecificRegionsAndFailures(t *testing.T) {
	for _, v := range []struct{ name, body, want, reg string }{
		{"Prime Video", `{"isServiceRestricted":false,"currentTerritory":"TW"}`, "unlocked", "TW"},
		{"Prime Video", `{"isServiceRestricted":true,"currentTerritory":"CN"}`, "locked", "CN"},
		{"TikTok", `{"region":"SG"}`, "unlocked", "SG"},
		{"TikTok", `{"region":"en-US"}`, "unknown", ""},
		{"YouTube", `<p>Premium is not available in your country</p><script>{"GL":"CN"}</script>`, "locked", "CN"},
		{"YouTube", `<p>YouTube Premium ad-free</p><script>{"GL":"TW"}</script>`, "unlocked", "TW"},
	} {
		s := classifyMediaPage(v.name, response(200, v.body))
		if s.Status != v.want || s.Region != v.reg {
			t.Fatalf("%s %+v", v.name, s)
		}
	}
}
