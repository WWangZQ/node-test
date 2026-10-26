package main

import (
	"encoding/base64"
	"encoding/json"
	"github.com/zhsama/clash-speedtest/unlock"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

func probeNetflix(c *http.Client) Service {
	s := Service{Status: "unknown", Label: "未判定", Detail: "两部测试片未返回可确认的片库标记"}
	unavailable := 0
	for _, id := range []string{"81280792", "70143836"} {
		r, e := fetch(c, "GET", "https://www.netflix.com/title/"+id, "")
		if e != nil {
			return networkFailure(e)
		}
		s.Evidence = append(s.Evidence, Evidence{URL: r.URL, HTTP: r.Code, Rule: "两部测试片的片库元数据 / HTTP 状态"})
		reg := matchCountry(r.Body, `"requestCountry"\s*:\s*"([A-Z]{2})"`, `"requestCountry"\s*:\s*\{\s*"id"\s*:\s*"([A-Z]{2})"`)
		if reg == "" {
			u, _ := url.Parse(r.URL)
			if u != nil {
				m := regexp.MustCompile(`^/([a-z]{2})(?:-[a-z]{2})?/`).FindStringSubmatch(u.Path)
				if len(m) > 1 {
					reg = country(m[1])
				}
			}
		}
		if reg != "" {
			s.Region = reg
			s.RegionKind = "service"
			s.RegionSource = "Netflix 片库响应 / 地区路径"
		}
		if r.Code == 404 || strings.Contains(r.Body, "page-404") {
			unavailable++
			continue
		}
		if g := gate(r); g != nil {
			g.Region = s.Region
			g.RegionKind = s.RegionKind
			g.RegionSource = s.RegionSource
			g.Evidence = s.Evidence
			return *g
		}
		if strings.Contains(r.Body, "og:video") || strings.Contains(r.Body, "watch-video") || strings.Contains(r.Body, "player-title-link") {
			s.Status = "unlocked"
			s.Label = "解锁"
			s.Detail = "测试片返回视频元数据；按原工具的片库探测规则判定，未登录播放"
			return s
		}
	}
	if unavailable == 2 {
		s.Status = "partial"
		s.Label = "片库受限"
		s.Detail = "两部地区测试片均不可用；可能仅开放自制内容或片源已下架"
	}
	return s
}
func requestWithHeaders(c *http.Client, method, target, body string, headers map[string]string) (responseData, error) {
	req, e := http.NewRequest(method, target, strings.NewReader(body))
	if e != nil {
		return responseData{}, e
	}
	req.Header.Set("User-Agent", unlock.UA_Browser)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	r, e := c.Do(req)
	if e != nil {
		return responseData{}, e
	}
	defer r.Body.Close()
	b, e := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	return responseData{Code: r.StatusCode, URL: r.Request.URL.String(), Body: string(b), Header: r.Header}, e
}
func probeDisney(c *http.Client) Service {
	// Public browser application identifier used by the anonymous device flow, not an account credential.
	auth := "Bearer ZGlzbmV5JmJyb3dzZXImMS4wLjA.Cu56AgSfBTDag5NiRA81oLHkDZfu5L3CKadnefEAY84"
	s := Service{Status: "unknown", Label: "未判定", Detail: "Disney 区域接口未完成"}
	call := func(path, body, contentType string) (map[string]any, bool) {
		r, e := requestWithHeaders(c, "POST", "https://disney.api.edge.bamgrid.com"+path, body, map[string]string{"Authorization": auth, "Content-Type": contentType})
		if e != nil {
			s = networkFailure(e)
			return nil, false
		}
		s.Evidence = append(s.Evidence, Evidence{URL: r.URL, HTTP: r.Code, Rule: "Disney 匿名设备地区接口"})
		if g := gate(r); g != nil {
			g.Evidence = s.Evidence
			s = *g
			return nil, false
		}
		var d map[string]any
		if json.Unmarshal([]byte(r.Body), &d) != nil {
			s.Detail = "Disney 区域接口响应无法解析"
			return nil, false
		}
		return d, true
	}
	d, ok := call("/devices", `{"deviceFamily":"browser","applicationRuntime":"chrome","deviceProfile":"windows","attributes":{}}`, "application/json")
	if !ok {
		return s
	}
	assertion := str(d["assertion"])
	if assertion == "" {
		s.Detail = "Disney 未返回匿名设备 assertion；检测接口可能已变动"
		return s
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:token-exchange"}, "latitude": {"0"}, "longitude": {"0"}, "platform": {"browser"}, "subject_token": {assertion}, "subject_token_type": {"urn:bamtech:params:oauth:token-type:device"}}
	d, ok = call("/token", form.Encode(), "application/x-www-form-urlencoded")
	if !ok {
		return s
	}
	refresh := str(d["refresh_token"])
	if refresh == "" {
		s.Detail = "Disney 未返回匿名设备 refresh_token"
		return s
	}
	payload, _ := json.Marshal(map[string]any{"query": "mutation refreshToken($input: RefreshTokenInput!) {refreshToken(refreshToken: $input) {activeSession {sessionId}}}", "variables": map[string]any{"input": map[string]any{"refreshToken": refresh}}})
	d, ok = call("/graph/v1/device/graphql", string(payload), "application/json")
	if !ok {
		return s
	}
	session := object(object(object(d["extensions"])["sdk"])["session"])
	s.Region = country(str(object(session["location"])["countryCode"]))
	s.RegionKind = "service"
	s.RegionSource = "Disney session.location.countryCode"
	allowed, valid := session["inSupportedLocation"].(bool)
	if !valid {
		s.Detail = "Disney 响应缺少 inSupportedLocation"
		return s
	}
	if !allowed {
		s.Status = "locked"
		s.Label = "不可用·地区限制"
		s.Detail = "Disney 区域接口返回 inSupportedLocation=false"
	} else {
		s.Status = "unlocked"
		s.Label = "解锁"
		s.Detail = "Disney 区域接口返回 inSupportedLocation=true"
	}
	s.Connectivity = "connected"
	return s
}

func probeMediaPage(spec ServiceSpec, c *http.Client) Service {
	target := spec.URL
	if spec.ID == "TikTok" {
		target = "https://www.tiktok.com/explore"
	}
	if spec.ID == "YouTube" {
		target = "https://www.youtube.com/premium?hl=en"
	}
	r, e := fetch(c, "GET", target, "")
	if e != nil {
		return networkFailure(e)
	}
	return annotate(classifyMediaPage(spec.ID, r), r, "服务专用页面地区与拒绝标记")
}
func classifyMediaPage(name string, r responseData) Service {
	b := normalizeBody(r.Body)
	region := ""
	switch name {
	case "YouTube":
		region = matchCountry(b, `"(?:GL|countryCode|country_code|INNERTUBE_CONTEXT_GL)"\s*:\s*"([A-Za-z]{2})"`)
	case "Prime Video":
		region = matchCountry(b, `"currentTerritory"\s*:\s*"([A-Z]{2})"`)
	case "TikTok":
		region = matchCountry(b, `"region"\s*:\s*"([A-Z]{2})"`)
	case "Spotify":
		region = spotifyMarket(b)
	case "HBO Max":
		region = matchCountry(b, `"(?:userCountry|geoCountry|countryCode)"\s*:\s*"([A-Z]{2})"`)
	}
	s := Service{Status: "unknown", Label: "未判定", Region: region, RegionKind: "service", RegionSource: name + " 业务地区字段", Detail: "未识别该服务所需的可用标记"}
	if region == "" {
		s.RegionKind = ""
		s.RegionSource = ""
	}
	if g := gate(r); g != nil {
		g.Region = region
		g.RegionSource = s.RegionSource
		g.RegionKind = s.RegionKind
		return *g
	}
	switch name {
	case "YouTube":
		if strings.Contains(b, "www.google.cn") {
			s.Status = "locked"
			s.Label = "不可用·送中"
			s.Region = "CN"
			s.RegionSource = "YouTube 返回 google.cn 地区信号"
			s.RegionKind = "service"
			s.Detail = "YouTube Premium 页面返回中国区信号"
			return s
		}
		if strings.Contains(strings.ToLower(b), "ad-free") || strings.Contains(b, `"browseId":"SPunlimited"`) || region != "" && strings.Contains(strings.ToLower(b), "youtube premium") {
			s.Status = "unlocked"
			s.Label = "解锁"
			s.Detail = "YouTube Premium 返回产品入口与地区，未出现地区拒绝"
		}
	case "Prime Video":
		m := regexp.MustCompile(`"isServiceRestricted"\s*:\s*(true|false)`).FindStringSubmatch(b)
		if len(m) > 1 && m[1] == "true" {
			s.Status = "locked"
			s.Label = "不可用·地区限制"
			s.Detail = "Prime Video 返回 isServiceRestricted=true"
		} else if region != "" {
			s.Status = "unlocked"
			s.Label = "解锁"
			s.Detail = "Prime Video 返回 currentTerritory，未返回服务受限标志"
		}
	case "TikTok":
		u, _ := url.Parse(r.URL)
		if u != nil && (u.Path == "/about" || strings.HasPrefix(u.Path, "/notfound")) {
			s.Status = "locked"
			s.Label = "不可用·跳转限制"
			s.Detail = "TikTok explore 被重定向至 about/notfound"
		} else if region != "" {
			s.Status = "unlocked"
			s.Label = "可用"
			s.Detail = "TikTok explore 正常返回业务地区字段"
		}
	case "Spotify":
		if region != "" {
			s.Status = "unlocked"
			s.Label = "播放区可用"
			s.Detail = "Spotify 返回 appServerConfig.market；未测试注册操作"
		}
	case "HBO Max":
		if region != "" && (strings.Contains(strings.ToLower(visibleBody(b)), "subscribe") || strings.Contains(strings.ToLower(visibleBody(b)), "sign up")) {
			s.Status = "unlocked"
			s.Label = "区域入口可用"
			s.Detail = "HBO Max 返回地区与订阅入口；未做账号播放检测"
		}
	}
	return s
}

func spotifyMarket(b string) string {
	m := regexp.MustCompile(`(?s)<script[^>]+id="appServerConfig"[^>]*>([^<]+)</script>`).FindStringSubmatch(b)
	if len(m) < 2 {
		return ""
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding} {
		raw, e := enc.DecodeString(strings.TrimSpace(m[1]))
		if e == nil {
			var d map[string]any
			if json.Unmarshal(raw, &d) == nil {
				return country(str(d["market"]))
			}
		}
	}
	return ""
}
