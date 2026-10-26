package main

import (
	"net"
	"strings"
)

// Only typed booleans count as observations. Null and premium placeholders remain missing.
func flagsFrom(r *RiskSource, m map[string]any, names ...string) {
	for _, k := range names {
		riskFlag(r, m, k)
	}
}
func parseExtraRisk(source, ip string, doc map[string]any) RiskSource {
	r := riskBase(source, ip)
	d := doc
	if source == "Scamalytics" {
		d = object(doc["scamalytics"])
	}
	if source == "AbuseIPDB" {
		d = object(doc["data"])
	}
	if doc["error"] != nil || doc["errors"] != nil || d["status"] == "error" || (source == "IPQS" && doc["success"] != true) {
		// Provider errors may echo credentials. Never retain their raw text.
		message := strings.ToLower(str(doc["error"]) + str(doc["message"]) + str(d["error"]))
		r.Status = "error"
		r.Detail = "数据源返回业务错误"
		if strings.Contains(message, "limit") || strings.Contains(message, "credit") || strings.Contains(message, "quota") {
			r.Status = "limited"
			r.Detail = "数据源额度或频率受限"
		}
		if strings.Contains(message, "unauthor") || strings.Contains(message, "key") {
			r.Status = "auth"
			r.Detail = "密钥或账号验证失败"
		}
		return r
	}
	field := "ip"
	if source == "AbuseIPDB" {
		field = "ipAddress"
	}
	if source != "IPQS" { // IPQS specifies the target in the request path; response may omit ip.
		if net.ParseIP(str(d[field])) == nil || !net.ParseIP(str(d[field])).Equal(net.ParseIP(ip)) {
			r.Detail = "响应 IP 与查询目标不一致或缺失"
			return r
		}
	} else if got := str(d["ip"]); got != "" && got != ip {
		r.Detail = "响应 IP 与查询目标不一致"
		return r
	}
	r.Status = "ok"
	r.Abuse = map[string]any{}
	switch source {
	case "ipapi.is":
		flagsFrom(&r, d, "is_datacenter", "is_proxy", "is_vpn", "is_tor", "is_abuser", "is_mobile")
		r.NetworkType = str(object(d["company"])["type"])
		r.Country = str(object(d["location"])["country_code"])
		if v, ok := d["is_abuser"].(bool); ok {
			r.Abuse["listed"] = v
		}
		for _, k := range []string{"company", "asn"} {
			if v := object(d[k])["abuser_score"]; v != nil {
				r.Abuse[k+"_abuser_score"] = v
			}
		}
		r.Detail = "IP 分类与滥用标记；company/asn abuser_score 是组织或网段指标，单独保留"
		if len(r.Flags) == 0 {
			r.Status = "partial"
			r.Detail = "只返回基础信息；当前接口未提供风险字段"
		}
	case "Scamalytics":
		if d["status"] != "ok" {
			r.Status = "unknown"
			r.Detail = "缺少有效业务状态"
			return r
		}
		riskScore(&r, d["scamalytics_score"])
		r.Level = str(d["scamalytics_risk"])
		flagsFrom(&r, object(d["scamalytics_proxy"]), "is_datacenter", "is_vpn", "is_resproxy", "is_apple_icloud_private_relay")
		if v, ok := d["is_blacklisted_external"].(bool); ok {
			r.Abuse["listed"] = v
		}
		// Whitelist evidence fields; never persist account, credits or arbitrary API output.
		ext := object(doc["external_datasources"])
		r.Upstream = map[string]any{}
		for name, keys := range map[string][]string{"ip2proxy_lite": {"proxy_type", "usage_type", "ip_blacklisted", "ip_blacklist_type"}, "firehol": {"ip_blacklisted_30", "ip_blacklisted_1day", "is_proxy"}, "ipsum": {"ip_blacklisted", "num_blacklists"}, "spamhaus_drop": {"ip_blacklisted"}, "x4bnet": {"is_vpn", "is_datacenter", "is_tor", "is_blacklisted_spambot"}} {
			values := map[string]any{}
			for _, k := range keys {
				v := object(ext[name])[k]
				switch v.(type) {
				case bool, float64:
					values[k] = v
				case string:
					if !strings.Contains(str(v), "PREMIUM FIELD") {
						values[k] = v
					}
				}
			}
			if len(values) > 0 {
				r.Upstream[name] = values
			}
		}
		// Fold upstream positive detections into this provider's single vote.
		pt := str(object(ext["ip2proxy_lite"])["proxy_type"])
		if pt == "PUB" {
			r.Flags["is_proxy"] = true
		} else if pt == "0" {
			r.Flags["is_proxy"] = false
		}
		for _, k := range []string{"is_vpn", "is_datacenter", "is_tor"} {
			if object(ext["x4bnet"])[k] == true {
				r.Flags[k] = true
			}
		}
		r.NetworkType = str(object(ext["ip2proxy_lite"])["usage_type"])
		r.Country = str(object(ext["maxmind_geolite2"])["ip_country_code"])
		r.Detail = "保留 Scamalytics 原分与等级；其聚合上游仅供溯源，每个平台计一票"
	case "AbuseIPDB":
		riskScore(&r, d["abuseConfidenceScore"])
		r.Scale = "AbuseIPDB 滥用置信分 0–100；近 90 天窗口"
		r.NetworkType = str(d["usageType"])
		r.Country = str(d["countryCode"])
		flagsFrom(&r, d, "isTor")
		for _, k := range []string{"totalReports", "numDistinctUsers", "lastReportedAt"} {
			if d[k] != nil {
				r.Abuse[k] = d[k]
			}
		}
		if n, ok := d["totalReports"].(float64); ok {
			r.Abuse["listed"] = n > 0
		}
		r.Abuse["window_days"] = 90
		r.Detail = "近 90 天社区举报；0 次只表示此来源窗口内未记录。usageType 上游为 IPinfo"
	case "IPQS":
		riskScore(&r, d["fraud_score"])
		flagsFrom(&r, d, "proxy", "vpn", "tor", "active_vpn", "active_tor", "recent_abuse", "bot_status")
		r.NetworkType = str(d["connection_type"])
		r.Country = str(d["country_code"])
		if v, ok := d["recent_abuse"].(bool); ok {
			r.Abuse["listed"] = v
		}
		r.Detail = "IPQualityScore 原始风险分；strictness=0，未传入虚构浏览器或用户身份参数"
	}
	return r
}
