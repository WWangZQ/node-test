package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/metacubex/mihomo/constant"
	"github.com/zhsama/clash-speedtest/unlock"
)

//go:embed catalog.json
var catalogData []byte

type ServiceSpec struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	URL      string `json:"url"`
	Method   string `json:"method"`
	Default  bool   `json:"default"`
}

func catalog() []ServiceSpec {
	var out []ServiceSpec
	_ = json.Unmarshal(catalogData, &out)
	return out
}
func selectedSpecs(names []string) []ServiceSpec {
	out := []ServiceSpec{}
	for _, s := range catalog() {
		if names == nil && s.Default {
			out = append(out, s)
			continue
		}
		for _, n := range names {
			if s.ID == n {
				out = append(out, s)
				break
			}
		}
	}
	return out
}

type responseData struct {
	Code   int
	URL    string
	Body   string
	Header http.Header
}

func fetch(c *http.Client, method, url, body string) (responseData, error) {
	req, e := http.NewRequest(method, url, strings.NewReader(body))
	if e != nil {
		return responseData{}, e
	}
	req.Header.Set("User-Agent", unlock.UA_Browser)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, e := c.Do(req)
	if e != nil {
		return responseData{}, e
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return responseData{Code: resp.StatusCode, URL: resp.Request.URL.String(), Body: string(b), Header: resp.Header}, e
}
func gate(r responseData) *Service {
	if reason := regionDenial(r); reason != "" {
		return &Service{Status: "locked", Label: "不可用·地区限制", Detail: reason, Connectivity: "connected"}
	}
	b := strings.ToLower(r.Body)
	if r.Header.Get("Cf-Mitigated") == "challenge" || strings.Contains(b, "<title>just a moment") || strings.Contains(b, "<title>attention required!") || (r.Code == 403 && strings.Contains(b, "cf-chl-")) {
		return &Service{Status: "challenge", Label: "可访问·CF验证", Detail: "已连接到服务的 Cloudflare 验证页；需完成验证后才能使用，地区是否允许另行判断", Connectivity: "connected"}
	}
	if r.Code == 429 {
		return &Service{Status: "limited", Label: "暂不可用·限流", Detail: "服务返回 HTTP 429", Connectivity: "connected"}
	}
	if r.Code == 401 {
		return &Service{Status: "auth", Label: "需登录验证", Detail: "服务要求登录；当前接口未提供区域结论", Connectivity: "connected"}
	}
	if r.Code == 403 || r.Code == 451 {
		return &Service{Status: "denied", Label: "不可用·拒绝访问", Detail: fmt.Sprintf("服务返回 HTTP %d，且未识别为 Cloudflare 验证页", r.Code), Connectivity: "connected"}
	}
	if r.Code < 200 || r.Code >= 300 {
		return &Service{Status: "error", Label: "失败·接口异常", Detail: fmt.Sprintf("接口返回 HTTP %d", r.Code), Connectivity: "connected"}
	}
	return nil
}

func probeService(spec ServiceSpec, c *http.Client) (s Service) {
	s = Service{Status: "error", Method: spec.Method, URL: spec.URL, Detail: "检测失败"}
	defer func() {
		if recover() != nil {
			s = Service{Status: "error", Detail: "检测器异常"}
		}
		s.Method = spec.Method
		s.URL = spec.URL
	}()
	if spec.ID == "Google" {
		return probeGoogle(spec, c)
	}
	if spec.ID == "Apple TV+" || spec.ID == "Steam" {
		return probeWebProduct(spec, c)
	}
	if isAI(spec.ID) {
		return probeAI(spec, c)
	}
	if spec.ID == "Netflix" {
		return probeNetflix(c)
	}
	if spec.ID == "Disney+" {
		return probeDisney(c)
	}
	switch spec.ID {
	case "YouTube", "Spotify", "Prime Video", "TikTok", "HBO Max":
		return probeMediaPage(spec, c)
	}
	method, body := "GET", ""
	if spec.Method == "dazn" {
		method = "POST"
		body = `{"LandingPageKey":"generic","Languages":"en","Platform":"web","PlatformAttributes":{},"Version":"2"}`
	}
	r, e := fetch(c, method, spec.URL, body)
	if e != nil {
		s.Detail = "网络连接或读取失败"
		return s
	}
	if g := gate(r); g != nil {
		return *g
	}
	var doc map[string]any
	_ = json.Unmarshal([]byte(r.Body), &doc)
	switch spec.Method {
	case "bili":
		code, ok := doc["code"].(float64)
		if !ok {
			return Service{Status: "unknown", Detail: "响应缺少 code，未判断为解锁"}
		}
		if code == 0 {
			result, _ := doc["result"].(map[string]any)
			dash := object(result["dash"])
			video, _ := dash["video"].([]any)
			audio, _ := dash["audio"].([]any)
			durl, _ := result["durl"].([]any)
			if !hasMediaURL(video) && !hasMediaURL(audio) && !hasMediaURL(durl) {
				return Service{Status: "unknown", Detail: "接口成功但未返回播放资源"}
			}
			return Service{Status: "unlocked", Detail: "地区限定节目返回播放资源（单节目探测）"}
		}
		if code == -10403 {
			return Service{Status: "locked", Detail: "地区限定接口返回 -10403"}
		}
		return Service{Status: "unknown", Detail: fmt.Sprintf("Bilibili 返回业务码 %.0f", code)}
	case "abema":
		region, _ := doc["isoCountryCode"].(string)
		if region == "" {
			region, _ = doc["country"].(string)
		}
		if len(region) != 2 {
			return Service{Status: "unknown", Detail: "响应缺少地区字段"}
		}
		if region == "JP" {
			return Service{Status: "unlocked", Region: region, Detail: "Abema 地区接口返回日本"}
		}
		return Service{Status: "partial", Region: region, Detail: "海外区域，仅确认海外内容范围"}
	case "dazn":
		reg, _ := doc["Region"].(map[string]any)
		if reg == nil {
			reg, _ = doc["region"].(map[string]any)
		}
		allowed, ok := reg["isAllowed"].(bool)
		if !ok {
			return Service{Status: "unknown", Detail: "DAZN 响应缺少 isAllowed 字段"}
		}
		region, _ := reg["GeolocatedCountry"].(string)
		if region == "" {
			region, _ = reg["countryCode"].(string)
		}
		if allowed {
			return Service{Status: "unlocked", Region: strings.ToUpper(region), Detail: "DAZN 区域接口允许访问"}
		}
		return Service{Status: "locked", Region: strings.ToUpper(region), Detail: "DAZN 区域接口拒绝访问"}
	case "bbc":
		if strings.Contains(r.Body, `"geolocation"`) {
			return Service{Status: "locked", Detail: "BBC 媒体接口返回 geolocation 限制"}
		}
		if strings.Contains(r.Body, `"connection"`) && hasMediaURL(doc) {
			return Service{Status: "unlocked", Region: "GB", Detail: "BBC 返回媒体连接信息"}
		}
		return Service{Status: "unknown", Detail: "BBC 未返回可确认的媒体连接"}
	case "mytv":
		region, ok := doc["region"].(float64)
		if !ok {
			return Service{Status: "unknown", Detail: "MyTVSuper 响应缺少地区字段"}
		}
		if region == 1 {
			return Service{Status: "unlocked", Region: "HK", Detail: "MyTVSuper 地区接口返回香港"}
		}
		return Service{Status: "locked", Detail: "MyTVSuper 返回其他地区"}
	case "radiko":
		re := regexp.MustCompile(`class=["'](JP[0-9]{1,2})["']`)
		m := re.FindStringSubmatch(r.Body)
		if len(m) > 1 {
			return Service{Status: "unlocked", Region: m[1], Detail: "Radiko 地区接口返回日本地区"}
		}
		return Service{Status: "unknown", Detail: "未识别 Radiko 地区信息"}
	case "wiki":
		q, _ := doc["query"].(map[string]any)
		u, _ := q["userinfo"].(map[string]any)
		if u == nil {
			return Service{Status: "unknown", Detail: "未返回用户封禁信息"}
		}
		if _, ok := u["blockid"]; ok {
			return Service{Status: "locked", Label: "编辑受限", Detail: "Wikipedia 返回当前地址的封禁记录"}
		}
		return Service{Status: "partial", Label: "未见本地封禁", Detail: "未返回本地封禁记录；页面保护与全局封禁仍可能影响编辑"}
	default:
		if len(strings.TrimSpace(r.Body)) < 100 {
			return Service{Status: "unknown", Detail: "响应为空或过短"}
		}
		return Service{Status: "reachable", Detail: "公开页面可达；未登录账号，未验证播放、注册或模型服务"}
	}
}
func claudeRegionReference(region string) string {
	// Conservative reference only. Unlisted/unknown countries never receive a guessed pass.
	yes := "|US|GB|JP|SG|TW|KR|CA|AU|NZ|DE|FR|NL|SE|CH|AT|BE|DK|ES|IT|PT|FI|NO|IE|PL|CZ|IN|ID|MY|TH|PH|VN|AE|BR|MX|ZA|AR|CL|CO|PE|IL|TR|UA|RO|GR|HU|SK|SI|HR|IS|LU|EE|LV|LT|PK|BD|LK|NP|KH|LA|MN|KE|NG|GH|EG|MA|QA|SA|BH|KW|OM|JO|LB|"
	if len(region) == 2 && strings.Contains(yes, "|"+region+"|") {
		return "官方支持列表已列入（2026-10-25 核对）"
	}
	if strings.Contains("|CN|HK|MO|RU|IR|KP|", "|"+region+"|") && len(region) == 2 {
		return "官方支持列表未列入（2026-10-25 核对）"
	}
	return "地区待核实；以官方支持列表为准"
}
func runServices(ctx context.Context, p constant.Proxy, in Input, r *Row) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, 6)
	for _, spec := range selectedSpecs(in.Platforms) {
		wg.Add(1)
		go func(spec ServiceSpec) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				mu.Lock()
				r.Services[spec.ID] = Service{Status: "error", Detail: "节点检测超时"}
				mu.Unlock()
				return
			}
			defer func() { <-sem }()
			sub, cancel := context.WithTimeout(ctx, 45*time.Second)
			defer cancel()
			c := makeClient(sub, p, time.Duration(in.Timeout)*time.Second)
			defer c.CloseIdleConnections()
			s := probeService(spec, c)
			mu.Lock()
			r.Services[spec.ID] = s
			mu.Unlock()
		}(spec)
	}
	wg.Wait()
}

// A present JSON key alone is not evidence of a playable resource.
func hasMediaURL(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, w := range x {
			if k == "url" || k == "href" || k == "baseUrl" || k == "base_url" {
				if u, ok := w.(string); ok && (strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://")) {
					return true
				}
			}
			if hasMediaURL(w) {
				return true
			}
		}
	case []any:
		for _, w := range x {
			if hasMediaURL(w) {
				return true
			}
		}
	}
	return false
}
