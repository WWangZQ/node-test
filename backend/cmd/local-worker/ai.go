package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/text/language"
)

func normalizeBody(b string) string {
	for i := 0; i < 3; i++ {
		b = strings.ReplaceAll(b, `\"`, `"`)
	}
	return b
}
func country(v string) string {
	r, e := language.ParseRegion(strings.ToUpper(strings.TrimSpace(v)))
	if e == nil && r.IsCountry() {
		return r.String()
	}
	return ""
}
func matchCountry(body string, patterns ...string) string {
	b := normalizeBody(body)
	for _, p := range patterns {
		m := regexp.MustCompile(p).FindStringSubmatch(b)
		if len(m) > 1 {
			if v := country(m[1]); v != "" {
				return v
			}
		}
	}
	return ""
}
func visibleBody(body string) string {
	z := html.NewTokenizer(strings.NewReader(body))
	skip := 0
	var b strings.Builder
	for {
		switch z.Next() {
		case html.ErrorToken:
			return strings.ToLower(b.String())
		case html.StartTagToken:
			t, _ := z.TagName()
			if string(t) == "script" || string(t) == "style" {
				skip++
			}
		case html.EndTagToken:
			t, _ := z.TagName()
			if (string(t) == "script" || string(t) == "style") && skip > 0 {
				skip--
			}
		case html.TextToken:
			if skip == 0 {
				b.Write(z.Text())
				b.WriteByte(' ')
			}
		}
	}
}
func regionDenial(r responseData) string {
	u, e := url.Parse(r.URL)
	if e == nil {
		path := strings.ToLower(u.Path)
		for _, p := range []string{"unavailable-in-region", "unsupported-country", "unsupported-region", "/unavailable", "/not-available", "/geo-blocked"} {
			if strings.Contains(path, p) {
				return "最终页面路径明确表示地区不可用：" + u.Path
			}
		}
	}
	b := strings.ToLower(normalizeBody(r.Body))
	var doc any
	if json.Unmarshal([]byte(r.Body), &doc) == nil {
		for _, m := range []string{"unsupported_country", "unsupported_region", "unsupported_location", "forbidden-location", "country_not_supported", "location_not_supported"} {
			if strings.Contains(b, m) {
				return "服务业务响应明确拒绝当前地区：" + m
			}
		}
	}
	text := visibleBody(r.Body)
	for _, m := range []string{"not available in your country", "not available in your region", "isn't available in your country", "isn’t available in your country", "not supported in your country", "not supported in your region", "not supported in this country", "not supported in this region", "region is not supported", "country is not supported", "unavailable in your region", "unavailable in your country", "your location is not supported", "该地区无法使用", "您所在的国家/地区不受支持", "您所在的地区不受支持", "不支持您所在的地区", "你所在的地区暂不支持", "当前地区暂不支持", "目前不支持您所在的国家", "所在地區尚未開放", "所在地区尚未开放"} {
		if strings.Contains(text, m) {
			return "页面明确显示地区限制：" + m
		}
	}
	return ""
}
func networkFailure(e error) Service {
	label := "失败·连接"
	if n, ok := e.(net.Error); ok && n.Timeout() {
		label = "失败·超时"
	}
	return Service{Status: "error", Label: label, Connectivity: "failed", Detail: "未取得服务有效响应；" + label}
}
func annotate(s Service, r responseData, rule string) Service {
	s.Evidence = append(s.Evidence, Evidence{URL: r.URL, HTTP: r.Code, Rule: rule})
	if s.Connectivity == "" {
		s.Connectivity = "connected"
	}
	return s
}
func fetchTrace(c *http.Client, base string, s *Service) {
	r, e := fetch(c, "GET", strings.TrimRight(base, "/")+"/cdn-cgi/trace", "")
	if e != nil {
		return
	}
	s.Evidence = append(s.Evidence, Evidence{URL: r.URL, HTTP: r.Code, Rule: "读取服务域名的出口地区，不替代产品地区判定"})
	if r.Code != 200 {
		return
	}
	host := ""
	loc := ""
	ip := ""
	for _, line := range strings.Split(r.Body, "\n") {
		kv := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "h":
			host = kv[1]
		case "loc":
			loc = country(kv[1])
		case "ip":
			if net.ParseIP(kv[1]) != nil {
				ip = kv[1]
			}
		}
	}
	u, _ := url.Parse(base)
	if u == nil || host != u.Hostname() {
		return
	}
	s.ExitIP = ip
	if s.Region == "" && loc != "" {
		s.Region = loc
		s.RegionSource = u.Hostname() + " /cdn-cgi/trace loc"
		s.RegionKind = "egress"
	}
}
func isAI(name string) bool {
	switch name {
	case "OpenAI", "Gemini", "Claude", "Grok", "Perplexity", "Copilot", "Meta AI":
		return true
	}
	return false
}
func probeAI(spec ServiceSpec, c *http.Client) Service {
	if spec.ID == "OpenAI" {
		return probeOpenAI(c)
	}
	if spec.ID == "Copilot" {
		return probeCopilot(c)
	}
	r, e := fetch(c, "GET", spec.URL, "")
	var s Service
	if e != nil {
		s = networkFailure(e)
	} else {
		s = classifyAI(spec.ID, r)
		s = annotate(s, r, s.Detail)
	}
	if spec.ID == "Gemini" {
		return s
	}
	base := map[string]string{"Claude": "https://claude.ai", "Grok": "https://grok.com", "Perplexity": "https://www.perplexity.ai", "Meta AI": "https://www.meta.ai"}[spec.ID]
	if base != "" {
		fetchTrace(c, base, &s)
	}

	return s
}
func classifyAI(name string, r responseData) Service {
	region := matchCountry(r.Body, `"ipCountry"\s*:\s*"([A-Za-z]{2})"`, `"countryCode"\s*:\s*"([A-Za-z]{2})"`)
	if name == "Gemini" {
		region = matchCountry(r.Body, `,\s*2\s*,\s*1\s*,\s*200\s*,\s*"([A-Z]{3})"`, `"(?:countryCode|ipCountry)"\s*:\s*"([A-Z]{2,3})"`)
	}
	s := Service{Status: "unknown", Label: "未判定", Region: region, Connectivity: "connected", Detail: "收到响应，但未识别到该服务的可用或地区拒绝依据"}
	if region != "" {
		s.RegionKind = "service"
		s.RegionSource = name + " 响应中的地区字段"
	}
	if g := gate(r); g != nil {
		g.Region = s.Region
		g.RegionKind = s.RegionKind
		g.RegionSource = s.RegionSource
		return *g
	}
	b := normalizeBody(r.Body)
	low := strings.ToLower(b)
	if name == "Gemini" {
		allow := regexp.MustCompile(`(?:"45631641"\s*:\s*|45631641\s*,\s*null\s*,\s*)(true|false)`).FindStringSubmatch(b)
		if len(allow) > 1 {
			if allow[1] == "true" {
				s.Status = "unlocked"
				s.Label = "可用"
				s.Detail = "Gemini 当前响应的可用性字段 45631641=true"
			} else {
				s.Status = "unknown"
				s.Label = "待核实"
				s.Detail = "Gemini 返回 45631641=false；该内部字段无法单独证明地区封禁，未取得明确拒绝文案"
			}
			return s
		}

		return s
	}
	markers := map[string][]string{"Claude": {"claude.ai", "continue with google", "continue with email", "log in to claude", "sign in to claude", "ipCountry"}, "Grok": {"grok.com", "ask anything", "sign in to grok", "countryCode"}, "Perplexity": {"perplexity.ai", "ask anything", "where knowledge begins", "__next", "/assets/"}, "Meta AI": {"meta.ai", "ask meta ai"}}
	title := regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`).FindStringSubmatch(b)
	recognized := false
	for _, v := range markers[name] {
		if strings.Contains(low, strings.ToLower(v)) {
			recognized = true
		}
	}
	titleOK := len(title) > 1 && strings.Contains(strings.ToLower(title[1]), strings.ToLower(strings.ReplaceAll(name, " AI", "")))
	if r.Code == 200 && recognized && (titleOK || name == "Claude" && region != "") {
		s.Status = "unlocked"
		s.Label = "可用"
		s.Detail = name + " 正常应用入口返回成功，识别到服务页面标记，未出现地区拒绝；按网络访问探测判定"
	}
	return s
}
func classifyOpenAI(r responseData, app bool) Service {
	b := strings.ToLower(r.Body)
	if reason := regionDenial(r); reason != "" {
		return annotate(Service{Status: "locked", Label: "地区受限", Detail: reason}, r, reason)
	}
	if app {
		// This is the known iOS edge business response used by upstream probes, not a generic 403 success rule.
		var d map[string]any
		_ = json.Unmarshal([]byte(r.Body), &d)
		msg := strings.ToLower(str(d["cf_details"]))
		if msg == "request is not allowed. please try again later." {
			return annotate(Service{Status: "unlocked", Label: "APP可用", Detail: "iOS 接口返回原检测工具识别的非地区拦截业务响应"}, r, "cf_details 精确匹配已知允许地区探测特征")
		}
		if strings.Contains(b, "disallowed isp") || strings.Contains(b, "blocked_why_headline") || strings.Contains(b, "sorry, you have been blocked") {
			return annotate(Service{Status: "locked", Label: "APP受限", Detail: "iOS 接口明确返回 ISP 或 IP 拦截"}, r, "识别 ISP/IP 拦截特征")
		}
	}
	if g := gate(r); g != nil {
		return annotate(*g, r, g.Detail)
	}
	if !app {
		var d map[string]any
		if json.Unmarshal([]byte(r.Body), &d) == nil {
			_, consent := d["cookie_consent_required"].(bool)
			reg := country(str(d["country"]))
			if consent || reg != "" {
				s := Service{Status: "unlocked", Label: "网页可用", Region: reg, Detail: "OpenAI 合规接口返回有效业务字段，未返回地区限制"}
				if reg != "" {
					s.RegionKind = "service"
					s.RegionSource = "OpenAI 合规接口 country"
				}
				return annotate(s, r, "有效 cookie_consent_required / country 字段")
			}
		}
	}
	return annotate(Service{Status: "unknown", Label: "未判定", Detail: "响应缺少当前规则所需业务字段"}, r, "字段未识别")
}
func probeOpenAI(c *http.Client) Service {
	parts := map[string]Service{}
	for _, v := range []struct {
		k, u string
		app  bool
	}{{"web", "https://api.openai.com/compliance/cookie_requirements", false}, {"app", "https://ios.chat.openai.com/", true}} {
		r, e := fetch(c, "GET", v.u, "")
		if e != nil {
			parts[v.k] = networkFailure(e)
		} else {
			parts[v.k] = classifyOpenAI(r, v.app)
		}
	}
	a, b := parts["web"], parts["app"]
	s := a
	s.Parts = parts
	s.Evidence = append(append([]Evidence{}, a.Evidence...), b.Evidence...)
	switch {
	case a.Status == "unlocked" && b.Status == "unlocked":
		s.Status = "unlocked"
		s.Label = "解锁"
	case a.Status == "unlocked":
		s.Status = "partial"
		s.Label = "网页可用·APP待定"
		if b.Status == "locked" || b.Status == "denied" {
			s.Label = "仅网页"
		}
	case b.Status == "unlocked":
		s.Status = "partial"
		s.Label = "APP可用·网页待定"
		if a.Status == "locked" || a.Status == "denied" {
			s.Label = "仅APP"
		}
	case a.Status == "locked" && b.Status == "locked":
		s.Status = "locked"
		s.Label = "不可用·地区/IP"
	}
	s.Detail = "网页：" + a.Label + "，" + a.Detail + "；APP：" + b.Label + "，" + b.Detail
	fetchTrace(c, "https://chatgpt.com", &s)
	return s
}
func probeCopilot(c *http.Client) Service {
	r, e := fetch(c, "GET", "https://copilot.microsoft.com/c/api/user", "")
	if e != nil {
		return networkFailure(e)
	}
	var d map[string]any
	_ = json.Unmarshal([]byte(r.Body), &d)
	reg := country(str(d["regionCode"]))
	s := Service{Status: "unknown", Label: "未判定", Region: reg, RegionKind: "service", RegionSource: "Copilot 用户接口 regionCode", Detail: "Copilot 用户接口缺少识别字段"}
	if g := gate(r); g != nil {
		s = *g
		s.Region = reg
	} else if str(d["id"]) != "" && reg != "" {
		s.Status = "unlocked"
		s.Label = "可用"
		s.Detail = "Copilot 匿名用户接口返回用户状态与地区"
	}
	return annotate(s, r, s.Detail)
}
