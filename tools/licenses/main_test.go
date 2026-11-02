/*
Copyright 2026 Joseph Anthony Abbott III

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func newModule(path, dir string, targets ...platform) *module {
	m := &module{path: path, dir: dir, pkgDirs: map[string]struct{}{dir: {}}, platforms: map[platform]struct{}{}}
	for _, p := range targets {
		m.platforms[p] = struct{}{}
	}
	return m
}

// testDockerfile mirrors the stages of the repository's Dockerfile on the
// Alpine release whose musl COPYRIGHT is embedded.
var testDockerfile = "FROM golang:1.26-alpine" + muslAlpineRelease + "@sha256:0123 AS static-builder\n" +
	"RUN CGO_ENABLED=1 go build \\\n    -tags " + linuxReleaseTags + " \\\n    -o /out/capsus .\n" +
	"FROM scratch AS static\n"

func TestNormalizeText(t *testing.T) {
	bom := string(rune(0xFEFF))
	tests := []struct {
		name, in, want string
	}{
		{"crlf and trailing spaces", "\r\n\r\nLine one  \r\nLine two\t\r\n\r\n", "Line one\nLine two\n"},
		{"lone carriage returns", "a\rb\r", "a\nb\n"},
		{"byte order mark", bom + "MIT License\n", "MIT License\n"},
		{"inner blank lines kept", "a\n\n\nb", "a\n\n\nb\n"},
		{"leading indentation kept", "   * item\n", "   * item\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeText([]byte(tt.in))
			if err != nil {
				t.Fatalf("normalizeText: %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
	for _, bad := range []string{"", " \r\n\t\n", "\xff\xfe"} {
		if _, err := normalizeText([]byte(bad)); err == nil {
			t.Errorf("normalizeText(%q) succeeded, want an error", bad)
		}
	}
}

func TestReadLicenseFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "LICENSE.txt"), "MIT text\r\n")
	writeFile(t, filepath.Join(dir, "COPYING"), "copying text\n")
	writeFile(t, filepath.Join(dir, "PATENTS"), "patents text\n")
	writeFile(t, filepath.Join(dir, "notice.go"), "package x\n")
	writeFile(t, filepath.Join(dir, "README.md"), "readme\n")
	writeFile(t, filepath.Join(dir, "THIRD_PARTY_LICENSES"), "a tool's own list\n")
	writeFile(t, filepath.Join(dir, "LICENSES", "inner"), "not read\n")

	texts, err := readLicenseFiles(dir, "sub/")
	if err != nil {
		t.Fatalf("readLicenseFiles: %v", err)
	}
	var names []string
	for _, lt := range texts {
		names = append(names, lt.name)
	}
	if got, want := strings.Join(names, ","), "sub/COPYING,sub/LICENSE.txt,sub/PATENTS"; got != want {
		t.Fatalf("names = %s, want %s", got, want)
	}
	if texts[1].text != "MIT text\n" {
		t.Fatalf("LICENSE.txt text = %q, want normalised text", texts[1].text)
	}

	writeFile(t, filepath.Join(dir, "LICENSE"), "\xff\n")
	if _, err := readLicenseFiles(dir, ""); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("readLicenseFiles with invalid UTF-8: err = %v, want a UTF-8 error", err)
	}
	if _, err := readLicenseFiles(filepath.Join(dir, "missing"), ""); err == nil {
		t.Fatal("readLicenseFiles of a missing directory succeeded")
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "NOTICE.txt"), "notice\n")
	if got, err := readFile(dir, "NOTICE.txt"); err != nil || got != "notice\n" {
		t.Fatalf("readFile = %q, %v", got, err)
	}
	_, err := readFile(dir, "missing")
	if err == nil || !strings.Contains(err.Error(), filepath.Join(dir, "missing")) {
		t.Fatalf("readFile of a missing file: err = %v, want the full path", err)
	}
	if _, err := readFile(dir, "../NOTICE.txt"); err == nil {
		t.Fatal("readFile accepted a name outside dir")
	}
}

func TestModuleTexts(t *testing.T) {
	t.Run("root and package license files", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "LICENSE"), "root license\n")
		writeFile(t, filepath.Join(dir, "internal", "vendored", "LICENSE"), "vendored license\n")
		writeFile(t, filepath.Join(dir, "plain", "plain.go"), "package plain\n")
		m := newModule("example.com/mod", dir)
		m.pkgDirs[filepath.Join(dir, "internal", "vendored")] = struct{}{}
		m.pkgDirs[filepath.Join(dir, "plain")] = struct{}{}
		texts, err := moduleTexts(m)
		if err != nil {
			t.Fatalf("moduleTexts: %v", err)
		}
		if len(texts) != 2 || texts[0].name != "LICENSE" || texts[1].name != "internal/vendored/LICENSE" {
			t.Fatalf("texts = %+v, want the root and the package license", texts)
		}
	})

	t.Run("bundled tool with license and notice", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "LICENSE"), "Apache License\n")
		writeFile(t, filepath.Join(dir, "NOTICE"), "Tool\r\n\r\nCopyright 2026 Someone\r\n")
		writeFile(t, filepath.Join(dir, "pkg", "main.go"), "package pkg\n")
		m := newModule("example.com/tool", dir)
		m.pkgDirs[filepath.Join(dir, "pkg")] = struct{}{}
		texts, err := moduleTexts(m)
		if err != nil {
			t.Fatalf("moduleTexts: %v", err)
		}
		if len(texts) != 2 || texts[0].name != "LICENSE" || texts[1].name != "NOTICE" || texts[1].text != "Tool\n\nCopyright 2026 Someone\n" {
			t.Fatalf("texts = %+v, want the LICENSE and the normalised NOTICE", texts)
		}
	})

	t.Run("no license file", func(t *testing.T) {
		m := newModule("example.com/unlicensed", t.TempDir())
		if _, err := moduleTexts(m); err == nil || !strings.Contains(err.Error(), "has no license file") {
			t.Fatalf("moduleTexts: err = %v, want a missing-license error", err)
		}
	})

	const localereader = "github.com/mattn/go-localereader"
	rl := readmeLicenses()[localereader]
	t.Run("reviewed README statement", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "README.md"), "# go-localereader\r\n\r\n"+strings.ReplaceAll(rl.excerpt, "\n", "\r\n")+"\r\n")
		texts, err := moduleTexts(newModule(localereader, dir))
		if err != nil {
			t.Fatalf("moduleTexts: %v", err)
		}
		if len(texts) != 2 || !strings.Contains(texts[0].text, "    MIT\n") || !strings.HasPrefix(texts[1].text, "Permission is hereby granted") {
			t.Fatalf("texts = %+v, want the README statement and the MIT terms", texts)
		}
	})

	t.Run("changed README statement", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "README.md"), "# go-localereader\n\n## License\n\nGPL\n")
		if _, err := moduleTexts(newModule(localereader, dir)); err == nil || !strings.Contains(err.Error(), "no longer contains") {
			t.Fatalf("moduleTexts: err = %v, want a changed-statement error", err)
		}
	})

	t.Run("missing README", func(t *testing.T) {
		if _, err := moduleTexts(newModule(localereader, t.TempDir())); err == nil {
			t.Fatal("moduleTexts without README succeeded")
		}
	})
}

func TestSQLiteNotice(t *testing.T) {
	header := "#ifndef USE_LIBSQLITE3\r\n/*\r\n** 2001-09-15\r\n**\r\n** " + sqliteDisclaimer + "  In place of\r\n** a legal notice, here is a blessing:\r\n**\r\n**    May you do good and not evil.\r\n**\r\n*************\r\n** API docs\r\n"
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, sqliteHeader), header)
	got, err := sqliteNotice(dir)
	if err != nil {
		t.Fatalf("sqliteNotice: %v", err)
	}
	want := "2001-09-15\n\n" + sqliteDisclaimer + "  In place of\na legal notice, here is a blessing:\n\n   May you do good and not evil.\n"
	if got != want {
		t.Fatalf("sqliteNotice = %q, want %q", got, want)
	}

	writeFile(t, filepath.Join(dir, sqliteHeader), "/*\n** Copyright someone\n*****\n")
	if _, err := sqliteNotice(dir); err == nil || !strings.Contains(err.Error(), "review the SQLite license") {
		t.Fatalf("sqliteNotice without the dedication: err = %v", err)
	}
	writeFile(t, filepath.Join(dir, sqliteHeader), "int x;\n")
	if _, err := sqliteNotice(dir); err == nil || !strings.Contains(err.Error(), "no header comment") {
		t.Fatalf("sqliteNotice without a comment: err = %v", err)
	}
	if _, err := sqliteNotice(t.TempDir()); err == nil {
		t.Fatal("sqliteNotice without the header succeeded")
	}
}

func TestCheckDockerfile(t *testing.T) {
	tests := []struct {
		name, content, wantErr string
	}{
		{"same release and tags", testDockerfile, ""},
		{"platform flag", "FROM --platform=$BUILDPLATFORM golang:1.26-alpine" + muslAlpineRelease + " AS b\nRUN go build -tags=" + linuxReleaseTags + " .\n", ""},
		{"two builds with the release tags", testDockerfile + "RUN go build -tags " + linuxReleaseTags + " ./other\n", ""},
		{"other builder release", strings.Replace(testDockerfile, "alpine"+muslAlpineRelease, "alpine0.1", 1), "uses Alpine 0.1"},
		{"other runtime release", testDockerfile + "FROM alpine:0.1\n", "uses Alpine 0.1"},
		{"no alpine", "FROM debian:12\nRUN go build -tags " + linuxReleaseTags + " .\n", "no Alpine image"},
		{"other tags", strings.Replace(testDockerfile, linuxReleaseTags, linuxReleaseTags+",extra", 1), "builds with -tags " + linuxReleaseTags + ",extra"},
		{"no tags", strings.Replace(testDockerfile, "-tags "+linuxReleaseTags, "", 1), "builds without -tags"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, dockerfile), tt.content)
			err := checkDockerfile(root)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("checkDockerfile: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("checkDockerfile: err = %v, want %q", err, tt.wantErr)
			}
		})
	}
	if err := checkDockerfile(t.TempDir()); err == nil {
		t.Fatal("checkDockerfile without a Dockerfile succeeded")
	}
}

func TestIncludedIn(t *testing.T) {
	all := releasePlatforms()
	set := func(ps ...platform) map[platform]struct{} {
		m := map[platform]struct{}{}
		for _, p := range ps {
			m[p] = struct{}{}
		}
		return m
	}
	tests := []struct {
		name string
		set  map[platform]struct{}
		want string
	}{
		{"all", set(all...), "all binaries"},
		{"windows", set(platform{"windows", "amd64"}, platform{"windows", "arm64"}), "Windows binaries"},
		{"mixed", set(platform{"linux", "arm64"}, platform{"darwin", "amd64"}, platform{"darwin", "arm64"}), "linux/arm64, macOS binaries"},
	}
	for _, tt := range tests {
		if got := includedIn(tt.set, all); got != tt.want {
			t.Errorf("%s: includedIn = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestAddPackages(t *testing.T) {
	linux := platform{"linux", "amd64"}
	stream := `{"Dir":"/goroot/src/fmt","Standard":true}
{"Dir":"/repo","Module":{"Path":"github.com/jabbott-iii/Capsus","Main":true,"Dir":"/repo"}}
{"Dir":"/repo/Munus/pkg","Module":{"Path":"github.com/jabbott-iii/Munus","Dir":"/repo/Munus","Replace":{"Path":"./Munus","Dir":"/repo/Munus"}}}
{"Dir":"/mod/a@v1/pkg","Module":{"Path":"example.com/a","Dir":"/mod/a@v1"}}
{"Dir":"/mod/a@v1/other","Module":{"Path":"example.com/a","Dir":"/mod/a@v1"}}
{"Dir":"/local/b/pkg","Module":{"Path":"example.com/b","Dir":"/mod/b@v1","Replace":{"Path":"../b","Dir":"/local/b"}}}
`
	byPath := map[string]*module{}
	if err := addPackages(byPath, strings.NewReader(stream), linux); err != nil {
		t.Fatalf("addPackages: %v", err)
	}
	if len(byPath) != 3 {
		t.Fatalf("modules = %v, want the bundled tool, example.com/a and example.com/b", byPath)
	}
	if tool := byPath["github.com/jabbott-iii/Munus"]; tool == nil || tool.dir != "/repo/Munus" {
		t.Fatalf("bundled tool = %+v, want its folder", tool)
	}
	if a := byPath["example.com/a"]; a.dir != "/mod/a@v1" || len(a.pkgDirs) != 2 {
		t.Fatalf("example.com/a = %+v", a)
	}
	if b := byPath["example.com/b"]; b.dir != "/local/b" {
		t.Fatalf("replaced module dir = %q, want the replacement's", b.dir)
	}
	if err := addPackages(byPath, strings.NewReader(stream), platform{"windows", "arm64"}); err != nil {
		t.Fatalf("addPackages: %v", err)
	}
	if n := len(byPath["example.com/a"].platforms); n != 2 {
		t.Fatalf("platforms = %d, want 2", n)
	}

	if err := addPackages(map[string]*module{}, strings.NewReader(`{"Dir":"/x","Module":{"Path":"example.com/c"}}`), linux); err == nil || !strings.Contains(err.Error(), "go mod download") {
		t.Fatalf("module without dir: err = %v", err)
	}
	if err := addPackages(map[string]*module{}, strings.NewReader(`{"Dir":`), linux); err == nil {
		t.Fatal("truncated go list output was accepted")
	}
}

func TestNoticeProblems(t *testing.T) {
	notice := "Capsus\r\n\r\nDirect dependencies:\r\n  - example.com/a Someone MIT License\r\n  - example.com/gone Someone MIT License\r\n\r\nAlso:\r\n  - SQLite, embedded in example.com/a, public domain\r\n  - MinGW-w64 runtime (Windows binaries only), the mingw-w64 project\r\n"
	mods := []*module{newModule("example.com/a", "/a"), newModule("example.com/b", "/b"), newModule("example.com/a/v2", "/a2"), newModule("github.com/jabbott-iii/Munus", "/m")}
	got := strings.Join(noticeProblems(notice, mods), "\n")
	want := "does not list example.com/b\ndoes not list example.com/a/v2\ndoes not list github.com/jabbott-iii/Munus\nlists example.com/gone, which no release binary includes"
	if got != want {
		t.Fatalf("noticeProblems =\n%s\nwant\n%s", got, want)
	}
	if p := noticeProblems("  - example.com/a\n  - github.com/jabbott-iii/Munus Someone Apache 2.0 License\n", []*module{mods[0], mods[3]}); len(p) != 0 {
		t.Fatalf("noticeProblems for a complete NOTICE.txt = %v", p)
	}
}

func TestFirstDifference(t *testing.T) {
	if _, differs := firstDifference("a\nb\n", "a\nb\n"); differs {
		t.Fatal("equal texts differ")
	}
	if line, _ := firstDifference("a\nb\n", "a\nc\n"); line != 2 {
		t.Fatalf("line = %d, want 2", line)
	}
	if line, _ := firstDifference("a\n", "a\nb\n"); line != 2 {
		t.Fatalf("line = %d, want 2 for a longer text", line)
	}
}

func TestOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the go command")
	}
	dir := t.TempDir()
	out, err := output(exec.Command("go", "env", "GOOS"), dir, "GOOS=plan9")
	if err != nil || strings.TrimSpace(string(out)) != "plan9" {
		t.Fatalf("output = %q, %v; want the extra environment to apply", out, err)
	}
	_, err = output(exec.Command("go", "capsus-no-such-command"), dir)
	if err == nil || !strings.Contains(err.Error(), "go capsus-no-such-command:") || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("output of a failing command: err = %v, want the command and its standard error", err)
	}
}

// fakeRepository builds a repository root, GOROOT and three modules (go-sqlite3,
// a bundled tool and a Windows-only module) and returns them.
func fakeRepository(t *testing.T) (root, goroot string, mods []*module) {
	t.Helper()
	root = t.TempDir()
	writeFile(t, filepath.Join(root, dockerfile), testDockerfile)
	writeFile(t, filepath.Join(root, noticeFile), "  - "+sqliteModule+" Someone MIT License\r\n  - example.com/tool Someone Apache 2.0 License\r\n  - example.com/win Someone MIT License\r\n")
	goroot = t.TempDir()
	writeFile(t, filepath.Join(goroot, "LICENSE"), "Go license\n")
	writeFile(t, filepath.Join(goroot, "PATENTS"), "Go patents\n")
	sqliteDir := t.TempDir()
	writeFile(t, filepath.Join(sqliteDir, "LICENSE"), "sqlite3 driver license\n")
	writeFile(t, filepath.Join(sqliteDir, sqliteHeader), "/*\n** "+sqliteDisclaimer+"\n*****\n")
	toolDir := t.TempDir()
	writeFile(t, filepath.Join(toolDir, "LICENSE"), "tool license\n")
	writeFile(t, filepath.Join(toolDir, "NOTICE"), "tool notice\n")
	winDir := t.TempDir()
	writeFile(t, filepath.Join(winDir, "LICENSE"), "windows module license\n")
	all := releasePlatforms()
	mods = []*module{
		newModule(sqliteModule, sqliteDir, all...),
		newModule("example.com/tool", toolDir, all...),
		newModule("example.com/win", winDir, platform{"windows", "amd64"}, platform{"windows", "arm64"}),
	}
	return root, goroot, mods
}

func TestGenerate(t *testing.T) {
	root, goroot, mods := fakeRepository(t)
	got, err := generate(root, goroot, mods)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	again, err := generate(root, goroot, mods)
	if err != nil || again != got {
		t.Fatalf("generate is not deterministic (err = %v)", err)
	}
	mingwTitle := "MinGW-w64 runtime (license text of mingw-w64 " + mingwVersion + ")"
	order := []string{
		"Third-party software licenses for Capsus\n",
		"Contents:\n  - Go standard library and runtime\n  - SQLite (compiled into " + sqliteModule + ")\n  - musl libc " + muslVersion + "\n  - " + mingwTitle + "\n  - " + sqliteModule + "\n  - example.com/tool\n  - example.com/win\n",
		"Go standard library and runtime\nIncluded in: all binaries\n",
		"--- LICENSE ---\n\nGo license\n",
		"--- PATENTS ---\n\nGo patents\n",
		"--- " + sqliteHeader + ", header comment ---\n\n" + sqliteDisclaimer + "\n",
		"musl libc " + muslVersion + "\nIncluded in: Linux binaries (statically linked; built on Alpine " + muslAlpineRelease + ")\n",
		"--- COPYRIGHT ---\n\nmusl as a whole is licensed under the following standard MIT license:",
		mingwTitle + "\nIncluded in: Windows binaries (parts linked statically)\n",
		"--- COPYING.MinGW-w64-runtime.txt ---\n\nMinGW-w64 runtime licensing\n",
		sqliteModule + "\nIncluded in: all binaries\n",
		"sqlite3 driver license\n",
		"example.com/tool\nIncluded in: all binaries\n",
		"--- LICENSE ---\n\ntool license\n\n--- NOTICE ---\n\ntool notice\n",
		"example.com/win\nIncluded in: Windows binaries\n",
		"windows module license\n",
	}
	pos := 0
	for _, part := range order {
		i := strings.Index(got[pos:], part)
		if i < 0 {
			t.Fatalf("output lacks %q after offset %d:\n%s", part, pos, got)
		}
		pos += i + len(part)
	}
	if strings.Contains(got, "\r") || !strings.HasSuffix(got, "\n") || strings.HasSuffix(got, "\n\n") {
		t.Fatal("output must use LF line endings and end with a single newline")
	}

	writeFile(t, filepath.Join(root, dockerfile), "FROM alpine:0.1\n")
	if _, err := generate(root, goroot, mods); err == nil {
		t.Fatal("generate accepted a Dockerfile on another Alpine release")
	}
	writeFile(t, filepath.Join(root, dockerfile), testDockerfile)
	if _, err := generate(root, t.TempDir(), mods); err == nil || !strings.Contains(err.Error(), "no license file found in GOROOT") {
		t.Fatalf("generate with an empty GOROOT: err = %v", err)
	}
	unlicensed := append(mods, newModule("example.com/none", t.TempDir()))
	if _, err := generate(root, goroot, unlicensed); err == nil {
		t.Fatal("generate accepted a module without a license")
	}
}

func TestFinish(t *testing.T) {
	root, goroot, mods := fakeRepository(t)
	content, err := generate(root, goroot, mods)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	out := filepath.Join(root, outputFile)

	var stdout, stderr bytes.Buffer
	if code := finish(root, true, content, mods, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), outputFile) {
		t.Fatalf("check without %s: code %d, stderr %q", outputFile, code, stderr.String())
	}

	stderr.Reset()
	if code := finish(root, false, content, mods, &stdout, &stderr); code != 0 {
		t.Fatalf("write: code %d, stderr %q", code, stderr.String())
	}
	if data, err := os.ReadFile(out); err != nil || string(data) != content {
		t.Fatalf("written file differs (err = %v)", err)
	}
	if code := finish(root, true, content, mods, &stdout, &stderr); code != 0 {
		t.Fatalf("check of the written file: code %d, stderr %q", code, stderr.String())
	}

	// A Windows checkout converts the file to CRLF; the check still passes.
	writeFile(t, out, strings.ReplaceAll(content, "\n", "\r\n"))
	if code := finish(root, true, content, mods, &stdout, &stderr); code != 0 {
		t.Fatalf("check of a CRLF checkout: code %d, stderr %q", code, stderr.String())
	}

	writeFile(t, out, strings.Replace(content, "windows module license", "edited", 1))
	stderr.Reset()
	if code := finish(root, true, content, mods, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "out of date") || !strings.Contains(stderr.String(), "make licenses") {
		t.Fatalf("check of an edited file: code %d, stderr %q", code, stderr.String())
	}

	writeFile(t, out, content)
	writeFile(t, filepath.Join(root, noticeFile), "  - "+sqliteModule+"\n  - example.com/tool\n")
	stderr.Reset()
	if code := finish(root, true, content, mods, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), noticeFile+": does not list example.com/win") {
		t.Fatalf("check with an incomplete NOTICE.txt: code %d, stderr %q", code, stderr.String())
	}
	stderr.Reset()
	if code := finish(root, false, content, mods, &stdout, &stderr); code != 1 {
		t.Fatalf("write with an incomplete NOTICE.txt: code %d, want 1", code)
	}
	if err := os.Remove(filepath.Join(root, noticeFile)); err != nil {
		t.Fatal(err)
	}
	if code := finish(root, true, content, mods, &stdout, &stderr); code != 1 {
		t.Fatalf("check without NOTICE.txt: code %d, want 1", code)
	}
}

// offlineGo makes the go command fail instead of downloading a module that is
// not in the module cache: tests use no network. `go mod download`, or CI's
// `go mod tidy -diff` step, fills the cache first.
func offlineGo(t *testing.T) {
	t.Helper()
	t.Setenv("GOPROXY", "off")
}

// TestListModulesForRepository runs `go list` on this repository.
func TestListModulesForRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the go command")
	}
	offlineGo(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mods, err := listModules(context.Background(), root, []platform{{"linux", "amd64"}})
	if err != nil {
		t.Fatalf("listModules: %v", err)
	}
	byPath := map[string]*module{}
	for _, m := range mods {
		byPath[m.path] = m
	}
	if _, ok := byPath["github.com/jabbott-iii/Capsus"]; ok {
		t.Fatal("the main module was listed")
	}
	sqlite := byPath[sqliteModule]
	if sqlite == nil {
		t.Fatalf("%s not listed in %d modules", sqliteModule, len(mods))
	}
	if _, err := os.Stat(filepath.Join(sqlite.dir, sqliteHeader)); err != nil {
		t.Fatalf("go-sqlite3 directory: %v", err)
	}
	// The bundled tools are replaced by their folders (AGENTS.md D-3).
	for _, tool := range []string{"Munus", "Cryptare", "Salus"} {
		m := byPath["github.com/jabbott-iii/"+tool]
		if m == nil {
			t.Fatalf("bundled tool %s not listed", tool)
		}
		got, err := os.Stat(m.dir)
		if err != nil {
			t.Fatalf("%s directory: %v", tool, err)
		}
		want, err := os.Stat(filepath.Join(root, tool))
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(got, want) {
			t.Fatalf("%s directory = %s, want the %s folder", tool, m.dir, tool)
		}
	}
	if _, err := goRoot(context.Background(), root); err != nil {
		t.Fatalf("goRoot: %v", err)
	}
}

// TestRepositoryLicensesUpToDate is the CI check of the committed
// THIRD_PARTY_LICENSES and NOTICE.txt (integration test: runs the go command
// on the module cache, offline).
func TestRepositoryLicensesUpToDate(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the go command")
	}
	offlineGo(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	// The go command reads the sources, go.mod and go.sum in a child process,
	// which go test's result cache does not see. Listing every directory of
	// the repository records their entries' sizes and modification times, so
	// a changed import or dependency invalidates a cached pass.
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repository: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if code := run(root, true, &stdout, &stderr); code != 0 {
		t.Fatalf("%s or %s is out of date, or a module is missing from the module cache (run `go mod download`) (exit %d):\n%s",
			outputFile, noticeFile, code, stderr.String())
	}
}
