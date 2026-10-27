package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureAsset() Asset {
	b := []byte("verified installer")
	h := sha256.Sum256(b)
	return Asset{Name: "NodeTest-1.4.0-windows-x64-Setup.exe", URL: "https://github.com/WWangZQ/node-test/releases/download/v1.4.0/NodeTest-1.4.0-windows-x64-Setup.exe", Size: int64(len(b)), Digest: "sha256:" + hex.EncodeToString(h[:])}
}
func TestVersionsAndAssetSelection(t *testing.T) {
	for _, x := range []struct {
		a, b string
		want bool
	}{{"1.4.0", "1.3.2", true}, {"1.10.0", "1.9.9", true}, {"1.4.0", "1.4.0", false}, {"1.3.2", "1.4.0", false}, {"1.5.0-beta", "1.4.0", false}, {"bad", "1.4.0", false}} {
		if newer(x.a, x.b) != x.want {
			t.Fatal(x)
		}
	}
	a := fixtureAsset()
	r := Release{Tag: "v1.4.0", Assets: []Asset{a}}
	u, e := selectUpdate(r, "1.3.2", "windows", "amd64")
	if e != nil || !u.Available {
		t.Fatal(u, e)
	}
	if _, e = selectUpdate(r, "1.3.2", "darwin", "arm64"); e == nil {
		t.Fatal("wrong architecture accepted")
	}
	r.Prerelease = true
	if _, e = selectUpdate(r, "1.3.2", "windows", "amd64"); e == nil {
		t.Fatal("prerelease accepted")
	}
}
func TestUntrustedOrIncompleteAssetRejected(t *testing.T) {
	a := fixtureAsset()
	for _, mutate := range []func(*Asset){func(a *Asset) { a.URL = strings.Replace(a.URL, "WWangZQ/node-test", "attacker/node-test", 1) }, func(a *Asset) { a.Digest = "" }, func(a *Asset) { a.Size = maxPackage + 1 }, func(a *Asset) { a.Name = "../installer.exe" }} {
		b := a
		mutate(&b)
		if validateAsset(b, "1.4.0", "windows", "amd64") == nil {
			t.Fatal(b)
		}
	}
}
func TestVerifiedDownloadAndCorruptionCleanup(t *testing.T) {
	for _, data := range []string{"verified installer", "corrupted installer", "verified installer extra"} {
		file := filepath.Join(t.TempDir(), "update.exe")
		e := saveDownload(bytes.NewBufferString(data), file, fixtureAsset())
		if data == "verified installer" {
			if e != nil {
				t.Fatal(e)
			}
			got, _ := os.ReadFile(file)
			if string(got) != data {
				t.Fatal("wrong bytes")
			}
		} else {
			if e == nil {
				t.Fatal("corrupt payload accepted")
			}
			if _, e = os.Stat(file); !os.IsNotExist(e) {
				t.Fatal("failed download retained")
			}
		}
	}
}
func TestReplaceAndRollbackPreserveUserData(t *testing.T) {
	for _, fail := range []bool{false, true} {
		root := t.TempDir()
		p := Plan{Target: filepath.Join(root, "app"), Stage: filepath.Join(root, "stage"), Backup: filepath.Join(root, "backup")}
		os.Mkdir(p.Target, 0700)
		os.Mkdir(p.Stage, 0700)
		os.WriteFile(filepath.Join(p.Target, "version"), []byte("old"), 0600)
		os.WriteFile(filepath.Join(p.Stage, "version"), []byte("new"), 0600)
		data := filepath.Join(root, "reports.json")
		os.WriteFile(data, []byte("keep"), 0600)
		rename := func(from, to string) error {
			if fail && from == p.Stage {
				return errors.New("simulated locked destination")
			}
			return os.Rename(from, to)
		}
		e := replaceBundle(p, rename)
		if fail != (e != nil) {
			t.Fatal(e)
		}
		got, _ := os.ReadFile(filepath.Join(p.Target, "version"))
		want := "new"
		if fail {
			want = "old"
		}
		if string(got) != want {
			t.Fatal(string(got))
		}
		b, _ := os.ReadFile(data)
		if string(b) != "keep" {
			t.Fatal("user data changed")
		}
	}
}
