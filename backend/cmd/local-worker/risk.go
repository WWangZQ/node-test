package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type RiskSource struct {
	Level       string          `json:"level,omitempty"`
	Abuse       map[string]any  `json:"abuse,omitempty"`
	Upstream    map[string]any  `json:"upstream,omitempty"`
	Source      string          `json:"source"`
	Status      string          `json:"status"`
	IP          string          `json:"ip,omitempty"`
	Score       *float64        `json:"score,omitempty"`
	Scale       string          `json:"scale,omitempty"`
	NetworkType string          `json:"network_type,omitempty"`
	Flags       map[string]bool `json:"flags,omitempty"`
	Country     string          `json:"country,omitempty"`
	ASN         string          `json:"asn,omitempty"`
	Detail      string          `json:"detail"`
	CheckedAt   string          `json:"checked_at"`
	Cached      bool            `json:"cached,omitempty"`
}

func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func str(v any) string            { s, _ := v.(string); return s }
func riskBase(source, ip string) RiskSource {
	return RiskSource{Source: source, IP: ip, Status: "unknown", CheckedAt: time.Now().UTC().Format(time.RFC3339), Flags: map[string]bool{}}
}
func riskFlag(r *RiskSource, m map[string]any, key string) {
	if b, ok := m[key].(bool); ok {
		r.Flags[key] = b
	}
}
func riskScore(r *RiskSource, v any) {
	if n, ok := v.(float64); ok && n >= 0 && n <= 100 {
		r.Score = &n
		r.Scale = "0–100，越高风险越高（该来源口径）"
	}
}
func parseRisk(source, ip string, doc map[string]any) RiskSource {
	r := riskBase(source, ip)
	if source == "ipapi.is" || source == "Scamalytics" || source == "AbuseIPDB" || source == "IPQS" {
		return parseExtraRisk(source, ip, doc)
	}
	switch source {
	case "IPPure":
		r.IP = str(doc["ip"])
		if net.ParseIP(r.IP) == nil {
			r.Detail = "响应缺少有效出口 IP"
			return r
		}
		r.Status = "ok"
		riskScore(&r, doc["fraudScore"])
		riskFlag(&r, doc, "isResidential")
		riskFlag(&r, doc, "isBroadcast")
		if b, ok := r.Flags["isResidential"]; ok {
			if b {
				r.NetworkType = "住宅/家宽"
			} else {
				r.NetworkType = "非住宅"
			}
		}
		r.Country = str(doc["countryCode"])
		if n, ok := doc["asn"].(float64); ok {
			r.ASN = fmt.Sprintf("AS%.0f · %s", n, str(doc["asOrganization"]))
		}
		r.Detail = "公开自查接口；住宅与欺诈分由 IPPure 判断，分数不等于欺诈事实或 Claude 官方评级"
	case "ProxyCheck":
		if str(doc["status"]) != "ok" && str(doc["status"]) != "warning" {
			r.Detail = "接口未返回有效结果，可能需要密钥或已达额度"
			if strings.Contains(strings.ToLower(str(doc["message"])), "limit") {
				r.Status = "limited"
			}
			return r
		}
		d := object(doc[ip])
		detections := object(d["detections"])
		if detections == nil {
			r.Detail = "缺少 detections 字段"
			return r
		}
		r.Status = "ok"
		riskScore(&r, detections["risk"])
		for _, k := range []string{"proxy", "vpn", "compromised", "scraper", "tor", "hosting", "anonymous"} {
			riskFlag(&r, detections, k)
		}
		network := object(d["network"])
		r.NetworkType = str(network["type"])
		r.ASN = str(network["asn"]) + " · " + str(network["provider"])
		r.Country = str(object(d["location"])["country_code"])
		r.Detail = "ProxyCheck v3 风险分类；机房等网络类型本身可带基础分，不能据此认定存在欺诈行为"
	case "NCGY":
		if got := str(doc["ip"]); got != ip {
			r.Detail = "查询响应的 IP 与目标不一致"
			return r
		}
		flags := object(doc["proxy"])
		if flags == nil {
			r.Detail = "缺少 proxy 分类字段"
			return r
		}
		r.Status = "ok"
		for _, k := range []string{"is_proxy", "is_vpn", "is_tor", "is_hosting", "is_cdn", "is_school", "is_anonymous"} {
			riskFlag(&r, flags, k)
		}
		r.Country = str(object(doc["country"])["iso_code"])
		a := object(doc["asn"])
		if n, ok := a["autonomous_system_number"].(float64); ok {
			r.ASN = fmt.Sprintf("AS%.0f · %s", n, str(a["autonomous_system_organization"]))
		}
		r.Detail = "ip.nc.gy 的 OpenProxyDB 分类；与其他数据库的差异分别保留"
	}
	return r
}
func requestRisk(c *http.Client, source, ip, endpoint string, headers ...map[string]string) RiskSource {
	r := riskBase(source, ip)
	req, e := http.NewRequest("GET", endpoint, nil)
	if e != nil {
		r.Status = "error"
		r.Detail = "请求配置错误"
		return r
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LocalNodeTest/1.3")
	for _, h := range headers {
		for k, v := range h {
			req.Header.Set(k, v)
		}
	}
	// Credentials stay on the documented host even if it redirects.
	client := *c
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	resp, e := client.Do(req)
	if e != nil {
		r.Status = "error"
		r.Detail = "数据源连接失败"
		return r
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 {
		r.Status = "limited"
		r.Detail = "数据源限流，本轮停止该来源"
		return r
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		r.Status = "auth"
		r.Detail = "数据源拒绝访问或密钥无效，本轮停止该来源"
		return r
	}
	if resp.StatusCode != 200 {
		r.Status = "error"
		r.Detail = fmt.Sprintf("数据源 HTTP %d", resp.StatusCode)
		return r
	}
	var doc map[string]any
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc) != nil {
		r.Detail = "数据源响应格式无法识别"
		return r
	}
	return parseRisk(source, ip, doc)
}
func runRisk(c *http.Client, in Input, r *Row) {
	r.IPRisk = map[string]RiskSource{}
	r.RiskUpdates = map[string]map[string]RiskSource{}
	r.IPProfiles = map[string]map[string]RiskSource{}
	r.RiskStopped = map[string]string{}
	pure := requestRisk(c, "IPPure", "", "https://my.ippure.com/v1/info")
	r.IPRisk["IPPure"] = pure
	if pure.Status == "ok" {
		r.IP = pure.IP
		r.Country = pure.Country
		r.ASN = pure.ASN
	}
	if r.IP == "" { // The public location endpoint is only a fallback for identifying the tested exit.
		v, e := fetch(c, "GET", "https://ipwho.is/", "")
		if e == nil && v.Code == 200 {
			var d map[string]any
			_ = json.Unmarshal([]byte(v.Body), &d)
			if d["success"] == true && net.ParseIP(str(d["ip"])) != nil {
				r.IP = str(d["ip"])
				r.Country = str(d["country_code"])
				a := object(d["connection"])
				if n, ok := a["asn"].(float64); ok {
					r.ASN = fmt.Sprintf("AS%.0f · %s", n, str(a["isp"]))
				}
			}
		}
	}
	for name, v := range queryRiskIP(c, in, r, r.IP) {
		r.IPRisk[name] = v
	}
	r.IPProfiles[r.IP] = r.IPRisk
	for _, name := range []string{"OpenAI", "Google", "Gemini", "Claude", "Grok", "Perplexity", "Copilot", "Meta AI"} {
		ip := r.Services[name].ExitIP
		if net.ParseIP(ip) == nil {
			continue
		}
		if _, ok := r.IPProfiles[ip]; !ok {
			r.IPProfiles[ip] = queryRiskIP(c, in, r, ip)
		}
		if name == "Claude" {
			r.ClaudeRisk = r.IPProfiles[ip]
		}
	}
}

var riskProviders = []string{"ProxyCheck", "NCGY", "ipapi.is", "Scamalytics", "AbuseIPDB", "IPQS"}

func queryRiskIP(c *http.Client, in Input, r *Row, ip string) map[string]RiskSource {
	out := map[string]RiskSource{}
	if net.ParseIP(ip) == nil {
		return out
	}
	if existing, ok := r.RiskUpdates[ip]; ok {
		return existing
	}
	if r.RiskUpdates == nil {
		r.RiskUpdates = map[string]map[string]RiskSource{}
	}
	if r.RiskStopped == nil {
		r.RiskStopped = map[string]string{}
	}
	for _, name := range riskProviders {
		if cached, ok := in.RiskCache[ip][name]; ok && cached.Source == name && cached.IP == ip && cached.Status == "ok" && cacheFresh(cached) {
			cached.Cached = true
			out[name] = cached
			continue
		}
		v := riskBase(name, ip)
		if status := in.RiskDisabled[name]; status != "" {
			v.Status = status
			v.Detail = "该来源本轮已停止查询"
			out[name] = v
			continue
		}
		if status := r.RiskStopped[name]; status != "" {
			v.Status = status
			v.Detail = "该来源本轮已停止查询"
			out[name] = v
			continue
		}
		key := in.RiskKeys[name]
		if name != "ProxyCheck" && name != "NCGY" && (key == "" || name == "Scamalytics" && in.RiskKeys["scam_user"] == "") {
			v.Status = "not_configured"
			v.Detail = "请在 IP 数据源设置中填写 API 密钥"
			out[name] = v
			continue
		}
		if name == "ProxyCheck" && (in.ProxyCheckRemaining <= r.ProxyCheckRequests || r.ProxyCheckLimited) {
			v.Status = "skipped"
			v.Detail = "本机每日查询预算已用完或该来源已限流"
			out[name] = v
			continue
		}
		endpoint := ""
		headers := map[string]string{}
		switch name {
		case "ProxyCheck":
			endpoint = "https://proxycheck.io/v3/" + url.PathEscape(ip)
			if key != "" {
				endpoint += "?key=" + url.QueryEscape(key)
			}
			r.ProxyCheckRequests++
		case "NCGY":
			time.Sleep(2100 * time.Millisecond)
			endpoint = "https://ip.nc.gy/json?ip=" + url.QueryEscape(ip)
		case "ipapi.is":
			endpoint = "https://api.ipapi.is/?q=" + url.QueryEscape(ip) + "&key=" + url.QueryEscape(key)
		case "Scamalytics":
			host := "api11.scamalytics.com"
			if in.RiskKeys["scam_region"] == "EU" {
				host = "api12.scamalytics.com"
			}
			endpoint = "https://" + host + "/v3/" + url.PathEscape(in.RiskKeys["scam_user"]) + "?key=" + url.QueryEscape(key) + "&ip=" + url.QueryEscape(ip)
		case "AbuseIPDB":
			endpoint = "https://api.abuseipdb.com/api/v2/check?maxAgeInDays=90&ipAddress=" + url.QueryEscape(ip)
			headers["Key"] = key
		case "IPQS":
			endpoint = "https://ipqualityscore.com/api/json/ip/" + url.PathEscape(key) + "/" + url.PathEscape(ip) + "?strictness=0"
		}
		v = requestRisk(c, name, ip, endpoint, headers)
		out[name] = v
		if v.Status == "limited" || v.Status == "auth" {
			r.RiskStopped[name] = v.Status
			if name == "ProxyCheck" {
				r.ProxyCheckLimited = true
			}
		}
	}
	r.RiskUpdates[ip] = out
	return out
}

func cacheFresh(r RiskSource) bool {
	t, e := time.Parse(time.RFC3339, r.CheckedAt)
	return e == nil && time.Since(t) >= 0 && time.Since(t) < 10*time.Minute
}
