//go:build darwin

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestMacAppliesSignedBundleAndRelaunches(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "launched.txt")
	swift := filepath.Join(root, "main.swift")
	binary := filepath.Join(root, "Fixture")
	os.WriteFile(swift, []byte("import Foundation\ntry! \"launched\".write(toFile:"+strconv.Quote(marker)+", atomically:true, encoding:.utf8)\n"), 0600)
	if b, e := exec.Command("/usr/bin/swiftc", swift, "-o", binary).CombinedOutput(); e != nil {
		t.Fatalf("fixture build: %s %v", b, e)
	}
	bundle := func(dir, version string) {
		t.Helper()
		os.MkdirAll(filepath.Join(dir, "Contents/MacOS"), 0700)
		b, _ := os.ReadFile(binary)
		os.WriteFile(filepath.Join(dir, "Contents/MacOS/Fixture"), b, 0700)
		info := fmt.Sprintf(`<?xml version="1.0"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>%s</string><key>CFBundleShortVersionString</key><string>%s</string><key>CFBundleExecutable</key><string>Fixture</string><key>CFBundlePackageType</key><string>APPL</string><key>LSBackgroundOnly</key><true/></dict></plist>`, bundleID, version)
		os.WriteFile(filepath.Join(dir, "Contents/Info.plist"), []byte(info), 0600)
		if command("/usr/bin/codesign", "--force", "--deep", "--sign", "-", dir) != nil {
			t.Fatal("fixture signing")
		}
	}
	stageRoot := filepath.Join(root, ".node-test-update-fixture")
	p := Plan{Target: filepath.Join(root, "NodeTest.app"), Stage: filepath.Join(stageRoot, "NodeTest.app"), Backup: filepath.Join(stageRoot, "previous.app"), Version: "1.4.0"}
	bundle(p.Target, "1.3.2")
	bundle(p.Stage, p.Version)
	planRoot := filepath.Join(root, "download")
	os.Mkdir(planRoot, 0700)
	file := filepath.Join(planRoot, "plan.json")
	b, _ := json.Marshal(p)
	os.WriteFile(file, b, 0600)
	parent := exec.Command("/bin/sleep", "0.3")
	if e := parent.Start(); e != nil {
		t.Fatal(e)
	}
	go parent.Wait()
	if e := apply(file, parent.Process.Pid); e != nil {
		t.Fatal(e)
	}
	if plist(p.Target, "CFBundleShortVersionString") != "1.4.0" {
		t.Fatal("version did not update")
	}
	for end := time.Now().Add(15 * time.Second); ; {
		if b, e := os.ReadFile(marker); e == nil && string(b) == "launched" {
			break
		}
		if time.Now().After(end) {
			t.Fatal("updated app did not launch")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, e := os.Stat(stageRoot); !os.IsNotExist(e) {
		t.Fatal("staging retained")
	}
}
