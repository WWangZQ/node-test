// Local batch worker, GPL-3.0. Reuses Mihomo adapters and Clash SpeedTest Pro detectors.
package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/http/httptrace"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/common/convert"
	"github.com/metacubex/mihomo/component/dialer"
	"github.com/metacubex/mihomo/constant"
	"github.com/zhsama/clash-speedtest/logger"
	"gopkg.in/yaml.v3"
)

type Input struct {
	RiskKeys            map[string]string                `json:"risk_keys"`
	RiskDisabled        map[string]string                `json:"risk_disabled"`
	Platforms           []string                         `json:"platforms"`
	IPRisk              bool                             `json:"ip_risk"`
	RiskCache           map[string]map[string]RiskSource `json:"risk_cache"`
	ProxyCheckRemaining int                              `json:"proxycheck_remaining"`
	Action              string                           `json:"action"`
	Content             string                           `json:"content"`
	Node                map[string]any                   `json:"node"`
	Mode                string                           `json:"mode"`
	Seconds             int                              `json:"seconds"`
	Megabytes           int                              `json:"megabytes"`
	Timeout             int                              `json:"timeout"`
	Interface           string                           `json:"interface"`
	Server              string                           `json:"server"`
	IPInfo              bool                             `json:"ip_info"`
}
type Evidence struct {
	URL  string `json:"url"`
	HTTP int    `json:"http_status"`
	Rule string `json:"rule"`
}
type Service struct {
	Label        string             `json:"label,omitempty"`
	RegionSource string             `json:"region_source,omitempty"`
	RegionKind   string             `json:"region_kind,omitempty"`
	Connectivity string             `json:"connectivity,omitempty"`
	Evidence     []Evidence         `json:"evidence,omitempty"`
	Parts        map[string]Service `json:"parts,omitempty"`

	Method      string `json:"method,omitempty"`
	URL         string `json:"url,omitempty"`
	ExitIP      string `json:"exit_ip,omitempty"`
	RegionCheck string `json:"region_check,omitempty"`
	Status      string `json:"status"`
	Region      string `json:"region"`
	Detail      string `json:"detail"`
}
type Row struct {
	IPProfiles         map[string]map[string]RiskSource `json:"ip_profiles,omitempty"`
	RiskStopped        map[string]string                `json:"risk_stopped,omitempty"`
	IPRisk             map[string]RiskSource            `json:"ip_risk,omitempty"`
	ClaudeRisk         map[string]RiskSource            `json:"claude_ip_risk,omitempty"`
	RiskUpdates        map[string]map[string]RiskSource `json:"risk_updates,omitempty"`
	ProxyCheckRequests int                              `json:"proxycheck_requests"`
	ProxyCheckLimited  bool                             `json:"proxycheck_limited"`
	Status             string                           `json:"status"`
	Error              string                           `json:"error"`
	TLS                *float64                         `json:"tls_ms"`
	HTTPS              *float64                         `json:"https_ms"`
	ProbeFailure       *float64                         `json:"probe_failure_pct"`
	Average            *float64                         `json:"average_MBps"`
	Peak               *float64                         `json:"peak_MBps"`
	Samples            []float64                        `json:"samples"`
	Bytes              int64                            `json:"bytes"`
	IP                 string                           `json:"ip"`
	Country            string                           `json:"country"`
	ASN                string                           `json:"asn"`
	Services           map[string]Service               `json:"services"`
}

func emit(v any)              { b, _ := json.Marshal(v); fmt.Println("RESULT:" + string(b)) }
func round(n float64) float64 { return math.Round(n*100) / 100 }
func ptr(n float64) *float64  { v := round(n); return &v }
func main() {
	logger.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	var in Input
	if json.NewDecoder(io.LimitReader(os.Stdin, 8<<20)).Decode(&in) != nil {
		emit(map[string]any{"error": "输入格式无效"})
		return
	}
	if in.Action == "parse" {
		parse(in.Content)
		return
	}
	if in.Interface != "" {
		dialer.DefaultInterface.Store(in.Interface)
	}
	r := Row{Status: "failed", Samples: []float64{}, Services: map[string]Service{}}
	defer func() {
		if recover() != nil {
			r.Status = "failed"
			r.Error = "检测器异常，已跳过此节点"
		}
		emit(r)
	}()
	if in.Timeout < 1 {
		in.Timeout = 8
	}
	if in.Seconds < 1 {
		in.Seconds = 8
	}
	if in.Megabytes < 1 {
		in.Megabytes = 64
	}
	if in.Server == "" {
		in.Server = "https://speed.cloudflare.com"
	}
	// A copied subscription must not turn this worker into a chained/system proxy.
	delete(in.Node, "dialer-proxy")
	delete(in.Node, "interface-name")
	delete(in.Node, "routing-mark")
	in.Node["name"] = "local-test"
	p, err := adapter.ParseProxy(in.Node)
	if err != nil {
		r.Error = "节点配置无效或协议暂不支持"
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(500+in.Seconds+45*((len(selectedSpecs(in.Platforms))+5)/6))*time.Second)
	defer cancel()
	client := makeClient(ctx, p, time.Duration(in.Timeout)*time.Second)
	defer client.CloseIdleConnections()
	// Fresh connection per probe: target TLS handshake and whole HTTP response latency.
	var tlsSum, latSum float64
	ok, tlsOK := 0, 0
	for i := 0; i < 3; i++ {
		start := time.Now()
		var t0 time.Time
		var tlsDur float64
		trace := &httptrace.ClientTrace{TLSHandshakeStart: func() { t0 = time.Now() }, TLSHandshakeDone: func(_ tls.ConnectionState, e error) {
			if e == nil && !t0.IsZero() {
				tlsDur = float64(time.Since(t0).Microseconds()) / 1000
			}
		}}
		req, _ := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), "GET", strings.TrimRight(in.Server, "/")+"/__down?bytes=0", nil)
		resp, e := client.Do(req)
		if e == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
			if resp.StatusCode == 200 {
				latSum += float64(time.Since(start).Microseconds()) / 1000
				ok++
				if tlsDur > 0 {
					tlsSum += tlsDur
					tlsOK++
				}
			}
		}
	}
	r.ProbeFailure = ptr(float64(3-ok) * 100 / 3)
	if ok > 0 {
		r.HTTPS = ptr(latSum / float64(ok))
		if tlsOK > 0 {
			r.TLS = ptr(tlsSum / float64(tlsOK))
		}
		r.Status = "complete"
	} else {
		r.Error = "测速站连接失败"
	}
	if in.Mode != "speed" {
		runServices(ctx, p, in, &r)
		for _, s := range r.Services {
			if s.Status == "unlocked" || s.Status == "reachable" || s.Status == "locked" {
				if ok == 0 {
					r.Status = "partial"
				}
			}
		}
	}
	if in.IPRisk {
		runRisk(client, in, &r)
	}
	if in.IPInfo && r.IP == "" {
		resp, e := client.Get("https://ipwho.is/")
		if e == nil {
			var v struct {
				Success    bool   `json:"success"`
				IP         string `json:"ip"`
				Country    string `json:"country_code"`
				Connection struct {
					ASN int    `json:"asn"`
					ISP string `json:"isp"`
				} `json:"connection"`
			}
			if json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&v) == nil && v.Success {
				r.IP = v.IP
				r.Country = v.Country
				r.ASN = fmt.Sprintf("AS%d · %s", v.Connection.ASN, v.Connection.ISP)
			}
			resp.Body.Close()
		}
	}
	if in.Mode != "unlock" && ok > 0 {
		download(ctx, p, in, &r)
	}
	if in.Mode == "unlock" && (ok > 0 || r.Status == "partial") {
		r.Error = ""
		r.Status = "complete"
	}
	for _, s := range r.Services {
		if s.Status == "error" && r.Status == "complete" {
			r.Status = "partial"
		}
	}
}
func makeClient(ctx context.Context, p constant.Proxy, timeout time.Duration) *http.Client {
	tr := &http.Transport{DisableKeepAlives: true, ForceAttemptHTTP2: false, TLSHandshakeTimeout: timeout, ResponseHeaderTimeout: timeout, DialContext: func(c context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		n, _ := strconv.Atoi(port)
		return p.DialContext(c, &constant.Metadata{Host: host, DstPort: uint16(n)})
	}}
	return &http.Client{Transport: &boundTransport{ctx: ctx, tr: tr}, Timeout: timeout}
}

type boundTransport struct {
	ctx context.Context
	tr  *http.Transport
}

func (b *boundTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c, cancel := context.WithCancel(r.Context())
	stop := context.AfterFunc(b.ctx, cancel)
	resp, e := b.tr.RoundTrip(r.Clone(c))
	if e != nil {
		stop()
		cancel()
		return nil, e
	}
	resp.Body = &boundedBody{ReadCloser: resp.Body, remaining: 256 << 20, done: func() { stop(); cancel() }}
	return resp, nil
}
func (b *boundTransport) CloseIdleConnections() { b.tr.CloseIdleConnections() }

type boundedBody struct {
	io.ReadCloser
	remaining int64
	done      func()
}

func (b *boundedBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, e := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	return n, e
}
func (b *boundedBody) Close() error { b.done(); return b.ReadCloser.Close() }
func download(ctx context.Context, p constant.Proxy, in Input, r *Row) {
	c := makeClient(ctx, p, time.Duration(in.Seconds)*time.Second)
	defer c.CloseIdleConnections()
	start := time.Now()
	resp, e := c.Get(fmt.Sprintf("%s/__down?bytes=%d", strings.TrimRight(in.Server, "/"), in.Megabytes*1000000))
	if e != nil {
		r.Status = "partial"
		r.Error = "下载测速失败"
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		r.Status = "partial"
		r.Error = "测速站返回非成功状态"
		return
	}
	buf := make([]byte, 65536)
	last := start
	var bucket int64
	var readErr error
	for {
		n, e := resp.Body.Read(buf)
		r.Bytes += int64(n)
		bucket += int64(n)
		now := time.Now()
		span := now.Sub(last).Seconds()
		if span >= 1 {
			r.Samples = append(r.Samples, round(float64(bucket)/span/1e6))
			last = now
			bucket = 0
		}
		if e != nil {
			readErr = e
			break
		}
		if r.Bytes >= int64(in.Megabytes)*1000000 {
			break
		}
	}
	duration := time.Since(start).Seconds()
	if r.Bytes == 0 {
		r.Status = "partial"
		r.Error = "未下载到测速数据"
		return
	}
	if readErr != nil && readErr != io.EOF && duration < float64(in.Seconds)*0.9 {
		r.Status = "partial"
		r.Error = "下载中断，速度仅供参考"
	}
	if tail := time.Since(last).Seconds(); bucket > 0 && (tail >= 0.5 || len(r.Samples) == 0) {
		r.Samples = append(r.Samples, round(float64(bucket)/tail/1e6))
	}
	r.Average = ptr(float64(r.Bytes) / duration / 1e6)
	peak := 0.0
	for _, v := range r.Samples {
		if v > peak {
			peak = v
		}
	}
	r.Peak = ptr(peak)
}
func parse(content string) {
	b := []byte(strings.TrimSpace(content))
	var doc struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	_ = yaml.Unmarshal(b, &doc)
	if len(doc.Proxies) == 0 {
		if decoded, e := base64.StdEncoding.DecodeString(string(b)); e == nil {
			_ = yaml.Unmarshal(decoded, &doc)
		}
	}
	if len(doc.Proxies) == 0 {
		nodes, e := convert.ConvertsV2Ray(b)
		if e == nil {
			doc.Proxies = nodes
		}
	}
	if len(doc.Proxies) == 0 {
		emit(map[string]any{"error": "未找到节点。请使用 Clash/Mihomo 订阅，或常见节点链接订阅；仅含 proxy-providers 的配置请导出实际节点。"})
		return
	}
	if len(doc.Proxies) > 500 {
		emit(map[string]any{"error": "每次最多支持 500 个节点"})
		return
	}
	for i, n := range doc.Proxies {
		if n["name"] == nil {
			n["name"] = fmt.Sprintf("节点 %d", i+1)
		}
	}
	emit(map[string]any{"nodes": doc.Proxies})
}
