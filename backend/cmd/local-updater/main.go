// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const repo = "WWangZQ/node-test"
const bundleID = "io.github.WWangZQ.node-test"
const maxPackage = 512 << 20

var versionRE = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}
type Release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}
type Update struct {
	Version   string `json:"version"`
	Available bool   `json:"available"`
	Asset     Asset  `json:"asset"`
}
type Plan struct{ Target, Stage, Backup, Version, Installer string }

func newer(a, b string) bool {
	if !versionRE.MatchString(a) || !versionRE.MatchString(b) {
		return false
	}
	aa := strings.Split(strings.TrimPrefix(a, "v"), ".")
	bb := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := range aa {
		x, _ := strconv.Atoi(aa[i])
		y, _ := strconv.Atoi(bb[i])
		if x != y {
			return x > y
		}
	}
	return false
}
func assetName(version, platform, arch string) string {
	if platform == "darwin" {
		if arch == "amd64" {
			arch = "x86_64"
		}
		return "NodeTest-" + version + "-macos-" + arch + ".dmg"
	}
	if platform == "windows" && arch == "amd64" {
		return "NodeTest-" + version + "-windows-x64-Setup.exe"
	}
	return ""
}
func validateAsset(a Asset, version, platform, arch string) error {
	if a.Name != assetName(version, platform, arch) || a.Name == "" {
		return errors.New("更新文件与系统架构不匹配")
	}
	expected := "https://github.com/" + repo + "/releases/download/v" + version + "/" + a.Name
	if a.URL != expected || a.Size < 1 || a.Size > maxPackage || !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(a.Digest) {
		return errors.New("更新文件校验信息无效")
	}
	return nil
}
func selectUpdate(r Release, current, platform, arch string) (Update, error) {
	if r.Draft || r.Prerelease || !versionRE.MatchString(r.Tag) || !versionRE.MatchString(current) {
		return Update{}, errors.New("版本信息无效")
	}
	v := strings.TrimPrefix(r.Tag, "v")
	out := Update{Version: v, Available: newer(v, current)}
	if !out.Available {
		return out, nil
	}
	for _, a := range r.Assets {
		if a.Name == assetName(v, platform, arch) {
			if e := validateAsset(a, v, platform, arch); e != nil {
				return Update{}, e
			}
			out.Asset = a
			return out, nil
		}
	}
	return Update{}, errors.New("此版本的安装包正在准备，请稍后重试")
}
func client(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 {
			return errors.New("重定向过多")
		}
		h := r.URL.Hostname()
		if r.URL.Scheme != "https" || !(h == "github.com" || h == "api.github.com" || h == "release-assets.githubusercontent.com" || h == "objects.githubusercontent.com") {
			return errors.New("更新下载地址校验失败")
		}
		return nil
	}}
}
func check(current string) (Update, error) {
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "NodeTest-Updater")
	resp, e := client(25 * time.Second).Do(req)
	if e != nil {
		return Update{}, errors.New("连接更新服务失败，请稍后重试")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return Update{}, fmt.Errorf("更新服务返回 HTTP %d", resp.StatusCode)
	}
	var r Release
	if json.NewDecoder(io.LimitReader(resp.Body, 3<<20)).Decode(&r) != nil {
		return Update{}, errors.New("版本信息读取失败")
	}
	return selectUpdate(r, current, runtime.GOOS, runtime.GOARCH)
}
func saveDownload(reader io.Reader, file string, a Asset) error {
	f, e := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(file)
		}
	}()
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(f, h), io.LimitReader(reader, a.Size+1))
	if e != nil {
		return errors.New("下载中断，请重试")
	}
	if n != a.Size || "sha256:"+hex.EncodeToString(h.Sum(nil)) != a.Digest {
		return errors.New("安装包校验失败，请重新下载")
	}
	if e = f.Sync(); e != nil {
		return e
	}
	ok = true
	return nil
}
func download(a Asset, dir string) (string, error) {
	req, _ := http.NewRequest("GET", a.URL, nil)
	req.Header.Set("User-Agent", "NodeTest-Updater")
	resp, e := client(15 * time.Minute).Do(req)
	if e != nil {
		return "", errors.New("安装包下载失败，请重试")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("下载服务返回 HTTP %d", resp.StatusCode)
	}
	file := filepath.Join(dir, a.Name)
	return file, saveDownload(resp.Body, file, a)
}
func command(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Run()
}
func plist(app, key string) string {
	b, e := exec.Command("/usr/bin/plutil", "-extract", key, "raw", "-o", "-", filepath.Join(app, "Contents/Info.plist")).Output()
	if e != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
func validateApp(app, version string) error {
	if plist(app, "CFBundleIdentifier") != bundleID || plist(app, "CFBundleShortVersionString") != version {
		return errors.New("应用身份或版本校验失败")
	}
	if command("/usr/bin/codesign", "--verify", "--deep", "--strict", app) != nil {
		return errors.New("应用完整性校验失败")
	}
	return nil
}
func prepare(current, expected, target string) (string, error) {
	u, e := check(current)
	if e != nil {
		return "", e
	}
	if !u.Available || u.Version != expected {
		return "", errors.New("版本已变化，请重新检查更新")
	}
	dir, e := os.MkdirTemp("", "node-test-update-")
	if e != nil {
		return "", e
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(dir)
		}
	}()
	file, e := download(u.Asset, dir)
	if e != nil {
		return "", e
	}
	p := Plan{Version: u.Version, Installer: file}
	if runtime.GOOS == "darwin" {
		target, e = filepath.Abs(target)
		if e != nil || !strings.HasSuffix(target, ".app") || plist(target, "CFBundleIdentifier") != bundleID || plist(target, "CFBundleShortVersionString") != current {
			return "", errors.New("应用安装位置无效")
		}
		real, e := filepath.EvalSymlinks(target)
		if e != nil {
			return "", errors.New("应用安装位置读取失败")
		}
		target = real
		// Stage beside the installed bundle, so the replacement is an atomic rename.
		stageRoot, e := os.MkdirTemp(filepath.Dir(target), ".node-test-update-")
		if e != nil {
			return "", errors.New("应用目录需要写入权限，请将应用移到个人 Applications 文件夹后更新")
		}
		keep := false
		defer func() {
			if !keep {
				os.RemoveAll(stageRoot)
			}
		}()
		mount := filepath.Join(dir, "mount")
		os.Mkdir(mount, 0700)
		if command("/usr/bin/hdiutil", "attach", "-readonly", "-nobrowse", "-mountpoint", mount, file) != nil {
			return "", errors.New("安装包打开失败")
		}
		defer command("/usr/bin/hdiutil", "detach", mount)
		source := filepath.Join(mount, "节点测评.app")
		if e = validateApp(source, u.Version); e != nil {
			return "", e
		}
		p.Stage = filepath.Join(stageRoot, "节点测评.app")
		p.Target = target
		p.Backup = filepath.Join(stageRoot, "previous.app")
		if command("/usr/bin/ditto", source, p.Stage) != nil {
			return "", errors.New("准备更新失败")
		}
		if e = validateApp(p.Stage, u.Version); e != nil {
			return "", e
		}
		helper, e := os.Executable()
		if e != nil {
			return "", e
		}
		if command("/usr/bin/ditto", helper, filepath.Join(dir, "node-updater")) != nil {
			return "", errors.New("更新助手准备失败")
		}
		keep = true
	}
	b, _ := json.Marshal(p)
	plan := filepath.Join(dir, "plan.json")
	if e = os.WriteFile(plan, b, 0600); e != nil {
		if p.Stage != "" {
			os.RemoveAll(filepath.Dir(p.Stage))
		}
		return "", e
	}
	ok = true
	return plan, nil
}
func replaceBundle(p Plan, rename func(string, string) error) error {
	if e := rename(p.Target, p.Backup); e != nil {
		return e
	}
	if e := rename(p.Stage, p.Target); e != nil {
		if rollback := rename(p.Backup, p.Target); rollback != nil {
			return fmt.Errorf("更新失败，原版保存在 %s", p.Backup)
		}
		return e
	}
	return nil
}
func apply(plan string, pid int) error {
	if runtime.GOOS != "darwin" {
		return errors.New("此平台通过安装程序更新")
	}
	b, e := os.ReadFile(plan)
	if e != nil {
		return e
	}
	var p Plan
	if json.Unmarshal(b, &p) != nil {
		return errors.New("更新计划读取失败")
	}
	if pid < 1 {
		return errors.New("应用进程无效")
	}
	deadline := time.Now().Add(time.Minute)
	for exec.Command("/bin/kill", "-0", strconv.Itoa(pid)).Run() == nil {
		if time.Now().After(deadline) {
			return errors.New("应用尚未退出，请关闭后重试更新")
		}
		time.Sleep(250 * time.Millisecond)
	}
	if !strings.HasPrefix(filepath.Base(filepath.Dir(p.Stage)), ".node-test-update-") || filepath.Dir(filepath.Dir(p.Stage)) != filepath.Dir(p.Target) || filepath.Dir(p.Backup) != filepath.Dir(p.Stage) {
		return errors.New("更新路径校验失败")
	}
	if e = validateApp(p.Stage, p.Version); e != nil {
		return e
	}
	if e = replaceBundle(p, os.Rename); e != nil {
		return e
	}
	if command("/usr/bin/open", "-n", p.Target) != nil {
		os.Rename(p.Target, p.Stage)
		os.Rename(p.Backup, p.Target)
		command("/usr/bin/open", p.Target)
		return errors.New("新版启动失败，已恢复原版")
	}
	os.RemoveAll(filepath.Dir(p.Stage))
	os.RemoveAll(filepath.Dir(plan))
	return nil
}
func main() {
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	mode := os.Args[1]
	flags := flag.NewFlagSet(mode, flag.ExitOnError)
	current := flags.String("current", "", "current version")
	expected := flags.String("version", "", "expected update")
	target := flags.String("app", "", "installed bundle")
	plan := flags.String("plan", "", "prepared plan")
	pid := flags.Int("pid", 0, "parent pid")
	flags.Parse(os.Args[2:])
	var result any
	var e error
	switch mode {
	case "check":
		result, e = check(*current)
	case "prepare":
		var p string
		p, e = prepare(*current, *expected, *target)
		result = map[string]string{"plan": p}
	case "apply":
		e = apply(*plan, *pid)
	default:
		e = errors.New("更新操作无效")
	}
	if e != nil {
		if mode == "apply" && *plan != "" {
			os.WriteFile(filepath.Join(filepath.Dir(*plan), "update-error.txt"), []byte(e.Error()), 0600)
			var p Plan
			if b, err := os.ReadFile(*plan); err == nil && json.Unmarshal(b, &p) == nil && plist(p.Target, "CFBundleIdentifier") == bundleID {
				command("/usr/bin/open", p.Target)
			}
			command("/usr/bin/osascript", "-e", `display alert "节点测评更新失败" message "原应用已保留。请重新打开应用后重试。"`)
		}
		json.NewEncoder(os.Stdout).Encode(map[string]string{"error": e.Error()})
		os.Exit(1)
	}
	json.NewEncoder(os.Stdout).Encode(result)
}
