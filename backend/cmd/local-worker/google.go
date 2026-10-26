package main

import (
	"golang.org/x/net/html"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Google Search is probed on its own domain. Gemini/Bard location endpoints cannot establish its exit.
func probeGoogle(spec ServiceSpec, c *http.Client) Service {
	r, e := fetch(c, "GET", spec.URL, "")
	if e != nil {
		return networkFailure(e)
	}
	s := classifyGoogle(r)
	if s.Region == "" {
		if home, err := fetch(c, "GET", "https://www.google.com/?hl=en", ""); err == nil {
			loc := classifyGoogle(home)
			if loc.Region != "" {
				s.Region = loc.Region
				s.RegionKind = "service"
				s.RegionSource = "Google 首页页脚地区"
				s.Evidence = append(s.Evidence, Evidence{URL: home.URL, HTTP: home.Code, Rule: "Google 自身首页页脚地区"})
			}
		}
	}
	return annotate(s, r, "Google 搜索响应；与 Gemini 分开请求，不传 gl、定位或登录 Cookie")
}
func classifyGoogle(r responseData) Service {
	s := Service{Status: "unknown", Label: "待核实", Detail: "Google 未返回可识别的搜索结果或地区字段"}
	u, _ := url.Parse(r.URL)
	low := strings.ToLower(r.Body)
	if (u != nil && strings.Contains(u.Path, "/sorry")) || strings.Contains(low, "our systems have detected unusual traffic") {
		s.Status = "challenge"
		s.Label = "人机验证"
		s.Detail = "Google 返回异常流量验证；搜索尚未通过"
		// Only the explicitly labelled client address on Google's challenge counts as its observed exit.
		m := regexp.MustCompile(`(?i)IP address:\s*(?:<[^>]*>\s*)*([0-9a-f:.]+)`).FindStringSubmatch(r.Body)
		if len(m) > 1 && net.ParseIP(m[1]) != nil {
			s.ExitIP = m[1]
		}
		return s
	}
	if g := gate(r); g != nil {
		return *g
	}
	if strings.Contains(low, "please enable javascript") || strings.Contains(low, "having trouble accessing google search") {
		s.Status = "challenge"
		s.Label = "浏览器验证"
		s.Detail = "Google 返回 JavaScript 浏览器检查页，未取得搜索结果"
	}
	root, e := html.Parse(strings.NewReader(r.Body))
	if e != nil {
		return s
	}
	footer := ""
	results := false
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if a.Key == "id" {
					if a.Val == "search" || a.Val == "rso" {
						results = true
					}
					if a.Val == "fbar" || a.Val == "footcnt" {
						var text func(*html.Node)
						text = func(x *html.Node) {
							if x.Type == html.TextNode {
								footer += " " + x.Data
							}
							for ch := x.FirstChild; ch != nil; ch = ch.NextSibling {
								text(ch)
							}
						}
						text(n)
					}
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)
	if results {
		s.Status = "unlocked"
		s.Label = "搜索可用"
		s.Detail = "Google 返回搜索结果容器"
	}
	// The location footer reports a product region, not proof of a client IP or physical server location.
	longest := 0
	for _, reg := range language.Supported.Regions() {
		if !reg.IsCountry() {
			continue
		}
		name := display.English.Regions().Name(reg)
		if len(name) > longest && regexp.MustCompile(`(?i)\b`+regexp.QuoteMeta(name)+`\b`).MatchString(footer) {
			s.Region = reg.String()
			longest = len(name)
		}
	}
	for name, code := range map[string]string{"Hong Kong": "HK", "Macau": "MO", "Macao": "MO", "Taiwan": "TW", "South Korea": "KR", "United States": "US", "China": "CN"} {
		if len(name) > longest && regexp.MustCompile(`(?i)\b`+regexp.QuoteMeta(name)+`\b`).MatchString(footer) {
			s.Region = code
			longest = len(name)
		}
	}
	if s.Region != "" {
		s.RegionKind = "service"
		s.RegionSource = "Google 搜索页底部地区"
		s.Detail += "；地区来自 Google 搜索页底部"
	}
	return s
}
func probeWebProduct(spec ServiceSpec, c *http.Client) Service {
	r, e := fetch(c, "GET", spec.URL, "")
	if e != nil {
		return networkFailure(e)
	}
	s := Service{Status: "unknown", Label: "待核实", Detail: "未识别产品业务页面"}
	if g := gate(r); g != nil {
		return annotate(*g, r, g.Detail)
	}
	body := strings.ToLower(r.Body)
	if spec.ID == "Steam" {
		s.Region = matchCountry(r.Body, `"COUNTRY"\s*:\s*"([A-Z]{2})"`, `g_strCountryCode\s*=\s*"([A-Z]{2})"`)
		if strings.Contains(body, "store.steampowered.com") && (strings.Contains(body, "store_nav") || strings.Contains(body, "g_steamid")) {
			s.Status = "unlocked"
			s.Label = "商店可用"
			s.Detail = "Steam 商店入口加载；购买地区仍受账号和支付方式影响"
		}
	} else {
		s.Region = matchCountry(r.Body, `"countryCode"\s*:\s*"([A-Za-z]{2})"`)
		if strings.Contains(body, "tv.apple.com") && (strings.Contains(body, "apple tv") || strings.Contains(body, "apple-tv")) {
			s.Status = "unlocked"
			s.Label = "网页可用"
			s.Detail = "Apple TV 网页入口加载；未验证订阅付费内容播放"
		}
	}
	if s.Region != "" {
		s.RegionKind = "service"
		s.RegionSource = spec.ID + " 页面地区字段"
	}
	return annotate(s, r, s.Detail)
}
