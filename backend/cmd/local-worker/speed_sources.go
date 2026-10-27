package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	C "github.com/metacubex/mihomo/constant"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"
)

type speedTarget struct {
	name, url  string
	rangeProbe bool
}
type SpeedAttempt struct {
	Source string `json:"source"`
	Phase  string `json:"phase"`
	HTTP   int    `json:"http_status,omitempty"`
	Error  string `json:"error,omitempty"`
}

func speedTargets(in Input, latency bool) []speedTarget {
	n := in.Megabytes * 1000000
	if latency {
		n = 0
	}
	primary := speedTarget{"Cloudflare", fmt.Sprintf("%s/__down?bytes=%d", strings.TrimRight(in.Server, "/"), n), false}
	if strings.TrimRight(in.Server, "/") != "https://speed.cloudflare.com" {
		primary.name = "Custom"
		return []speedTarget{primary}
	}
	return []speedTarget{primary, {"OVH", "https://proof.ovh.net/files/100Mb.dat", latency}, {"Vultr SG", "https://sgp-ping.vultr.com/vultr.com.100MB.bin", latency}}
}
func speedError(e error) string {
	if e == nil {
		return ""
	}
	var ne net.Error
	if errors.As(e, &ne) && ne.Timeout() {
		return "请求超时"
	}
	if errors.Is(e, io.ErrClosedPipe) || errors.Is(e, io.EOF) {
		return "连接被关闭"
	}
	if strings.Contains(strings.ToLower(e.Error()), "tls") {
		return "TLS 握手失败"
	}
	return "连接失败"
}
func binaryResponse(resp *http.Response, b []byte) bool {
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	p := strings.ToLower(strings.TrimSpace(string(b)))
	return !strings.Contains(ct, "text/") && !strings.Contains(ct, "json") && !strings.Contains(ct, "xml") && !strings.HasPrefix(p, "<!doctype") && !strings.HasPrefix(p, "<html")
}
func measureLatency(ctx context.Context, p C.Proxy, in Input, r *Row) int {
	c := makeClient(ctx, p, time.Duration(in.Timeout)*time.Second)
	defer c.CloseIdleConnections()
	for _, target := range speedTargets(in, true) {
		var total, tlsTotal float64
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
			req, _ := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), "GET", target.url, nil)
			if target.rangeProbe {
				req.Header.Set("Range", "bytes=0-0")
			}
			resp, e := c.Do(req)
			a := SpeedAttempt{Source: target.name, Phase: "latency", Error: speedError(e)}
			if tlsDur > 0 {
				r.TLS = ptr(tlsDur)
				r.LatencySource = target.name
			}
			if e == nil {
				a.HTTP = resp.StatusCode
				b, re := io.ReadAll(io.LimitReader(resp.Body, 1024))
				resp.Body.Close()
				if (resp.StatusCode == 200 || resp.StatusCode == 206) && re == nil && (!target.rangeProbe || binaryResponse(resp, b)) {
					total += float64(time.Since(start).Microseconds()) / 1000
					ok++
					if tlsDur > 0 {
						tlsTotal += tlsDur
						tlsOK++
					}
				} else if re != nil {
					a.Error = speedError(re)
				} else {
					a.Error = fmt.Sprintf("HTTP %d 或响应内容异常", resp.StatusCode)
				}
			}
			r.SpeedAttempts = append(r.SpeedAttempts, a)
			if ok == 0 {
				break
			}
		}
		if ok > 0 {
			r.ProbeFailure = ptr(float64(3-ok) * 100 / 3)
			r.HTTPS = ptr(total / float64(ok))
			r.TLS = nil
			if tlsOK > 0 {
				r.TLS = ptr(tlsTotal / float64(tlsOK))
			}
			r.LatencySource = target.name
			r.Status = "complete"
			return ok
		}
	}
	r.ProbeFailure = ptr(100)
	r.Error = "各延迟目标均失败"
	return 0
}
func download(ctx context.Context, p C.Proxy, in Input, r *Row) {
	for _, target := range speedTargets(in, false) {
		c := makeClient(ctx, p, time.Duration(in.Seconds)*time.Second)
		start := time.Now()
		resp, e := c.Get(target.url)
		a := SpeedAttempt{Source: target.name, Phase: "download", Error: speedError(e)}
		if e != nil {
			c.CloseIdleConnections()
			r.SpeedAttempts = append(r.SpeedAttempts, a)
			continue
		}
		a.HTTP = resp.StatusCode
		if resp.StatusCode != 200 && resp.StatusCode != 206 {
			a.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
			resp.Body.Close()
			c.CloseIdleConnections()
			r.SpeedAttempts = append(r.SpeedAttempts, a)
			continue
		}
		prefix := make([]byte, 512)
		n, pe := io.ReadFull(resp.Body, prefix)
		prefix = prefix[:n]
		if !binaryResponse(resp, prefix) || n < 512 {
			a.Error = "响应为网页、错误内容或数据过短"
			resp.Body.Close()
			c.CloseIdleConnections()
			r.SpeedAttempts = append(r.SpeedAttempts, a)
			continue
		}
		r.Bytes = int64(n)
		r.Samples = []float64{}
		last := start
		bucket := int64(n)
		buf := make([]byte, 65536)
		var readErr error
		if pe != nil {
			readErr = pe
		}
		for readErr == nil && r.Bytes < int64(in.Megabytes)*1000000 {
			remaining := int64(in.Megabytes)*1000000 - r.Bytes
			want := len(buf)
			if remaining < int64(want) {
				want = int(remaining)
			}
			n, e := resp.Body.Read(buf[:want])
			r.Bytes += int64(n)
			bucket += int64(n)
			if span := time.Since(last).Seconds(); span >= 1 {
				r.Samples = append(r.Samples, round(float64(bucket)/span/1e6))
				last = time.Now()
				bucket = 0
			}
			if e != nil {
				readErr = e
			}
		}
		resp.Body.Close()
		c.CloseIdleConnections()
		duration := time.Since(start).Seconds()
		if tail := time.Since(last).Seconds(); bucket > 0 && (tail >= .5 || len(r.Samples) == 0) {
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
		r.DownloadSource = target.name
		r.Error = ""
		if r.HTTPS == nil {
			r.Status = "partial"
			r.Error = "下载成功；延迟目标失败"
		} else {
			r.Status = "complete"
		}
		if readErr != nil && readErr != io.EOF && duration < float64(in.Seconds)*.9 {
			r.Status = "partial"
			a.Error = speedError(readErr)
			r.Error = "下载中断：" + a.Error
		}
		r.SpeedAttempts = append(r.SpeedAttempts, a)
		return
	}
	r.Error = "各下载源均失败"
	if r.HTTPS != nil || r.Status == "partial" {
		r.Status = "partial"
	} else {
		r.Status = "failed"
	}
}
