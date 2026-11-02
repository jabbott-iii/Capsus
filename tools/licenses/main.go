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

// Command licenses writes THIRD_PARTY_LICENSES, the full license texts of the
// third-party software compiled into the Capsus release binaries, and checks
// that the committed file and NOTICE.txt are up to date. It is adapted from
// Munus' tools/licenses (.claude/rules/build-and-ci.md P-040).
//
// Run it from the repository root (`make licenses` runs the first form):
//
//	go run ./tools/licenses          # rewrite THIRD_PARTY_LICENSES
//	go run ./tools/licenses -check   # fail if THIRD_PARTY_LICENSES or NOTICE.txt is out of date
//
// The module set is the union of the packages `go list -deps` reports for
// every release platform of cd.yml with cgo enabled, Linux with the
// Dockerfile's build tags, ignoring go.work files and GOFLAGS. The bundled
// tools are modules replaced by their folders (AGENTS.md D-3), so their
// LICENSE and NOTICE files are reproduced like any other module's. Module
// versions are not written (a binary records them; see `go version -m`), so
// the file only changes when a module is added or removed or a license text
// changes.
package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	outputFile = "THIRD_PARTY_LICENSES"
	noticeFile = "NOTICE.txt"
	dockerfile = "Dockerfile"

	// The Linux release binaries are built in the Dockerfile's Alpine builder
	// and link musl statically (P-030). An Alpine release keeps one upstream
	// musl version, so when the Dockerfile moves to another Alpine release,
	// replace musl-COPYRIGHT with the COPYRIGHT file of that release's musl
	// version and update both constants; the Dockerfile check below fails
	// until then.
	muslVersion       = "1.2.6"
	muslAlpineRelease = "3.24"

	// The Windows release binaries are linked by MinGW-w64 toolchains (the
	// runner's GCC for windows/amd64, the pinned llvm-mingw for windows/arm64),
	// which link parts of the MinGW-w64 runtime in statically.
	// mingw-w64-runtime.txt is COPYING.MinGW-w64-runtime.txt from this
	// mingw-w64 release: the text of earlier releases plus one more notice, so
	// it also covers an older toolchain.
	mingwVersion = "14.0.0"

	// linuxReleaseTags are the build tags of the Linux release binaries
	// (Dockerfile, static-builder stage); cd.yml builds the macOS and Windows
	// binaries without tags. The Dockerfile check below fails when they differ.
	linuxReleaseTags = "sqlite_omit_load_extension,osusergo,netgo"

	// listFields names the `go list -json` fields that listedPackage decodes.
	listFields = "-json=Dir,Standard,Module"

	sqliteModule     = "github.com/mattn/go-sqlite3"
	sqliteHeader     = "sqlite3-binding.h"
	sqliteDisclaimer = "The author disclaims copyright to this source code."

	commandTimeout = 5 * time.Minute
	ruleWidth      = 80
)

// muslCopyright is musl's COPYRIGHT file, verbatim from the musl release named
// by muslVersion.
//
//go:embed musl-COPYRIGHT
var muslCopyright string

// mingwRuntimeLicense is the MinGW-w64 runtime license file of the release
// named by mingwVersion, without its trailing spaces.
//
//go:embed mingw-w64-runtime.txt
var mingwRuntimeLicense string

// mitTerms is the standard MIT license text without a copyright line, written
// for modules whose README names the MIT license but that ship no license file.
const mitTerms = `Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.`

// licenseFileName matches the license and notice files collected from a
// module's root directory and from the directories of its compiled packages.
var licenseFileName = regexp.MustCompile(`(?i)^(licen[cs]e|copying|notice|patents|unlicense|copyright)([._-].*)?$`)

// noticeModuleLine matches a list entry in NOTICE.txt that names a module path.
var noticeModuleLine = regexp.MustCompile(`(?m)^[ \t]*-[ \t]+([a-z0-9.-]+\.[a-z]{2,}/[^\s]+)`)

// dockerFrom matches the image reference of a Dockerfile FROM line.
var dockerFrom = regexp.MustCompile(`(?im)^[ \t]*FROM[ \t]+(?:--platform=[^\s]+[ \t]+)?([^\s]+)`)

// alpineRelease matches the Alpine release in an image reference such as
// golang:1.26-alpine3.24 or alpine:3.24.
var alpineRelease = regexp.MustCompile(`alpine:?(\d+\.\d+)`)

// dockerTags matches the build tags of a go command in the Dockerfile.
var dockerTags = regexp.MustCompile(`-tags[= ]([^\s"']+)`)

type platform struct{ goos, goarch string }

// releasePlatforms returns the release targets built by cd.yml.
func releasePlatforms() []platform {
	return []platform{
		{"linux", "amd64"}, {"linux", "arm64"},
		{"darwin", "amd64"}, {"darwin", "arm64"},
		{"windows", "amd64"}, {"windows", "arm64"},
	}
}

// readmeLicense describes a module that ships no license file but states its
// license in a file of its root directory. The statement is checked verbatim,
// so a changed README fails generation until it is reviewed again.
type readmeLicense struct {
	file    string
	excerpt string
	terms   string
}

func readmeLicenses() map[string]readmeLicense {
	return map[string]readmeLicense{
		"github.com/mattn/go-localereader": {
			file:    "README.md",
			excerpt: "## License\n\nMIT\n\n## Author\n\nYasuhiro Matsumoto (a.k.a. mattn)",
			terms:   mitTerms,
		},
	}
}

// module is a third-party module compiled into at least one release binary.
type module struct {
	path      string
	dir       string
	pkgDirs   map[string]struct{}
	platforms map[platform]struct{}
}

// licenseText is one file (or statement) reproduced in a section.
type licenseText struct {
	name string
	text string
}

// section is one component of THIRD_PARTY_LICENSES.
type section struct {
	title    string
	included string
	texts    []licenseText
}

func main() {
	check := flag.Bool("check", false, "check that "+outputFile+" and "+noticeFile+" are up to date instead of writing "+outputFile)
	root := flag.String("C", ".", "repository root")
	flag.Parse()
	if flag.NArg() != 0 {
		_, _ = fmt.Fprintln(os.Stderr, "licenses: unexpected arguments:", strings.Join(flag.Args(), " "))
		os.Exit(2)
	}
	os.Exit(run(*root, *check, os.Stdout, os.Stderr))
}

func run(root string, check bool, stdout, stderr io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	mods, err := listModules(ctx, root, releasePlatforms())
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "licenses:", err)
		return 1
	}
	goroot, err := goRoot(ctx, root)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "licenses:", err)
		return 1
	}
	content, err := generate(root, goroot, mods)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "licenses:", err)
		return 1
	}
	return finish(root, check, content, mods, stdout, stderr)
}

// finish writes or checks THIRD_PARTY_LICENSES and checks NOTICE.txt.
func finish(root string, check bool, content string, mods []*module, stdout, stderr io.Writer) int {
	status := 0
	if check {
		current, err := readFile(root, outputFile)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "licenses: %v\n", err)
			return 1
		}
		// A Windows checkout may have CRLF line endings; compare the text.
		if line, differs := firstDifference(strings.ReplaceAll(current, "\r\n", "\n"), content); differs {
			_, _ = fmt.Fprintf(stderr, "licenses: %s is out of date (first difference at line %d); run `make licenses`\n", outputFile, line)
			status = 1
		}
	} else {
		// WriteFile keeps the mode of an existing file (0644 in a checkout); a
		// new file gets 0600 here, and git records either as 100644.
		if err := os.WriteFile(filepath.Join(root, outputFile), []byte(content), 0o600); err != nil {
			_, _ = fmt.Fprintf(stderr, "licenses: write %s: %v\n", outputFile, err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "wrote %s (%d modules)\n", outputFile, len(mods))
	}

	notice, err := readFile(root, noticeFile)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "licenses: %v\n", err)
		return 1
	}
	for _, problem := range noticeProblems(notice, mods) {
		_, _ = fmt.Fprintf(stderr, "licenses: %s: %s\n", noticeFile, problem)
		status = 1
	}
	return status
}

// listModules returns the modules of the packages compiled into the main
// package for each platform, sorted by module path. The main module is left
// out; the bundled tools are included.
func listModules(ctx context.Context, root string, targets []platform) ([]*module, error) {
	byPath := map[string]*module{}
	for _, target := range targets {
		out, err := goList(ctx, root, target)
		if err != nil {
			return nil, fmt.Errorf("list packages for %s/%s: %w", target.goos, target.goarch, err)
		}
		if err := addPackages(byPath, bytes.NewReader(out), target); err != nil {
			return nil, fmt.Errorf("list packages for %s/%s: %w", target.goos, target.goarch, err)
		}
	}
	mods := make([]*module, 0, len(byPath))
	for _, m := range byPath {
		mods = append(mods, m)
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].path < mods[j].path })
	return mods, nil
}

// goList runs `go list -deps` on the main package for a release platform, with
// the build tags of its release binary. The go command's arguments are
// constants; only the environment names the platform.
func goList(ctx context.Context, root string, target platform) ([]byte, error) {
	var cmd *exec.Cmd
	if target.goos == "linux" {
		cmd = exec.CommandContext(ctx, "go", "list", "-deps", listFields, "-tags="+linuxReleaseTags, ".")
	} else {
		cmd = exec.CommandContext(ctx, "go", "list", "-deps", listFields, ".")
	}
	// The module set must not depend on the caller's workspace or flags.
	return output(cmd, root, "GOOS="+target.goos, "GOARCH="+target.goarch, "CGO_ENABLED=1", "GOWORK=off", "GOFLAGS=-mod=readonly")
}

// goRoot returns the GOROOT of the go command, whose license files cover the
// standard library and runtime.
func goRoot(ctx context.Context, root string) (string, error) {
	out, err := output(exec.CommandContext(ctx, "go", "env", "GOROOT"), root)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(out))
	if value == "" {
		return "", errors.New("go env GOROOT is empty")
	}
	return value, nil
}

// output runs cmd in dir with extra environment variables and returns its
// standard output; an error includes the command's standard error.
func output(cmd *exec.Cmd, dir string, env ...string) ([]byte, error) {
	cmd.Dir = dir
	// Environ sets PWD to dir, so the go command reports the paths of dir.
	cmd.Env = append(cmd.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		name := strings.Join(cmd.Args, " ")
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("%s: %w: %s", name, err, msg)
		}
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

// listedPackage holds the `go list -json` fields used here.
type listedPackage struct {
	Dir      string
	Standard bool
	Module   *listedModule
}

type listedModule struct {
	Path    string
	Main    bool
	Dir     string
	Replace *listedModule
}

// addPackages records the modules of the packages in a `go list -json` stream.
func addPackages(byPath map[string]*module, r io.Reader, target platform) error {
	dec := json.NewDecoder(r)
	for {
		var pkg listedPackage
		if err := dec.Decode(&pkg); errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return fmt.Errorf("decode go list output: %w", err)
		}
		if pkg.Standard || pkg.Module == nil || pkg.Module.Main {
			continue
		}
		dir := pkg.Module.Dir
		if pkg.Module.Replace != nil && pkg.Module.Replace.Dir != "" {
			dir = pkg.Module.Replace.Dir
		}
		if dir == "" {
			return fmt.Errorf("module %s has no directory; run `go mod download`", pkg.Module.Path)
		}
		m := byPath[pkg.Module.Path]
		if m == nil {
			m = &module{path: pkg.Module.Path, dir: dir, pkgDirs: map[string]struct{}{}, platforms: map[platform]struct{}{}}
			byPath[pkg.Module.Path] = m
		}
		m.platforms[target] = struct{}{}
		if pkg.Dir != "" {
			m.pkgDirs[pkg.Dir] = struct{}{}
		}
	}
}

// generate renders THIRD_PARTY_LICENSES for the given modules.
func generate(root, goroot string, mods []*module) (string, error) {
	if err := checkDockerfile(root); err != nil {
		return "", err
	}
	all := releasePlatforms()

	goTexts, err := readLicenseFiles(goroot, "")
	if err != nil {
		return "", fmt.Errorf("license files of the Go distribution: %w", err)
	}
	if len(goTexts) == 0 {
		return "", fmt.Errorf("no license file found in GOROOT %s", goroot)
	}
	sections := []section{{
		title:    "Go standard library and runtime",
		included: "all binaries",
		texts:    goTexts,
	}}

	var sqlite *module
	for _, m := range mods {
		if m.path == sqliteModule {
			sqlite = m
		}
	}
	if sqlite != nil {
		notice, err := sqliteNotice(sqlite.dir)
		if err != nil {
			return "", err
		}
		sections = append(sections, section{
			title:    "SQLite (compiled into " + sqliteModule + ")",
			included: includedIn(sqlite.platforms, all),
			texts:    []licenseText{{name: sqliteHeader + ", header comment", text: notice}},
		})
	}

	musl, err := normalizeText([]byte(muslCopyright))
	if err != nil {
		return "", fmt.Errorf("musl-COPYRIGHT: %w", err)
	}
	sections = append(sections, section{
		title:    "musl libc " + muslVersion,
		included: "Linux binaries (statically linked; built on Alpine " + muslAlpineRelease + ")",
		texts:    []licenseText{{name: "COPYRIGHT", text: musl}},
	})

	mingw, err := normalizeText([]byte(mingwRuntimeLicense))
	if err != nil {
		return "", fmt.Errorf("mingw-w64-runtime.txt: %w", err)
	}
	sections = append(sections, section{
		title:    "MinGW-w64 runtime (license text of mingw-w64 " + mingwVersion + ")",
		included: "Windows binaries (parts linked statically)",
		texts:    []licenseText{{name: "COPYING.MinGW-w64-runtime.txt", text: mingw}},
	})

	for _, m := range mods {
		texts, err := moduleTexts(m)
		if err != nil {
			return "", err
		}
		sections = append(sections, section{title: m.path, included: includedIn(m.platforms, all), texts: texts})
	}
	return render(sections), nil
}

// moduleTexts returns the license texts of a module: the license files in its
// root directory and in the directories of its compiled packages, or the
// reviewed README statement for a module that ships no license file.
func moduleTexts(m *module) ([]licenseText, error) {
	texts, err := readLicenseFiles(m.dir, "")
	if err != nil {
		return nil, fmt.Errorf("module %s: %w", m.path, err)
	}
	if len(texts) == 0 {
		rl, ok := readmeLicenses()[m.path]
		if !ok {
			return nil, fmt.Errorf("module %s has no license file; review its license and add it to readmeLicenses", m.path)
		}
		statement, err := readmeStatement(m.dir, rl)
		if err != nil {
			return nil, fmt.Errorf("module %s: %w", m.path, err)
		}
		texts = []licenseText{
			{name: "no license file; " + rl.file + " states", text: statement},
			{name: "license terms reproduced for reference", text: rl.terms + "\n"},
		}
	}

	// Package directories below the module root, sorted by their slash-separated
	// path so that the output is the same on every OS.
	rels := map[string]string{}
	for dir := range m.pkgDirs {
		rel, err := filepath.Rel(m.dir, dir)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			continue
		}
		rels[filepath.ToSlash(rel)] = dir
	}
	names := make([]string, 0, len(rels))
	for rel := range rels {
		names = append(names, rel)
	}
	sort.Strings(names)
	for _, rel := range names {
		sub, err := readLicenseFiles(rels[rel], rel+"/")
		if err != nil {
			return nil, fmt.Errorf("module %s: %w", m.path, err)
		}
		texts = append(texts, sub...)
	}
	return texts, nil
}

// readLicenseFiles reads the license files directly in dir, sorted by name.
// prefix is prepended to the names shown in the output.
func readLicenseFiles(dir, prefix string) ([]licenseText, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var texts []licenseText
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() || !licenseFileName.MatchString(name) || strings.HasSuffix(strings.ToLower(name), ".go") {
			continue
		}
		data, err := readFile(dir, name)
		if err != nil {
			return nil, err
		}
		text, err := normalizeText([]byte(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Join(dir, name), err)
		}
		texts = append(texts, licenseText{name: prefix + name, text: text})
	}
	return texts, nil
}

// readmeStatement returns the reviewed license statement of rl, indented,
// after checking that the module's file still contains it verbatim.
func readmeStatement(dir string, rl readmeLicense) (string, error) {
	data, err := readFile(dir, rl.file)
	if err != nil {
		return "", err
	}
	text, err := normalizeText([]byte(data))
	if err != nil {
		return "", fmt.Errorf("%s: %w", rl.file, err)
	}
	if !strings.Contains(text, rl.excerpt) {
		return "", fmt.Errorf("%s no longer contains the reviewed license statement; review the module's license", rl.file)
	}
	lines := strings.Split(rl.excerpt, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = "    " + line
		}
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// sqliteNotice returns the public-domain dedication at the top of the SQLite
// header that go-sqlite3 compiles in, without the comment markers.
func sqliteNotice(moduleDir string) (string, error) {
	data, err := readFile(moduleDir, sqliteHeader)
	if err != nil {
		return "", fmt.Errorf("SQLite notice: %w", err)
	}
	lines := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	start, end := -1, -1
	for i, line := range lines {
		if start < 0 && strings.HasPrefix(line, "/*") {
			start = i + 1
			continue
		}
		if start >= 0 && strings.HasPrefix(line, "*****") {
			end = i
			break
		}
	}
	if start < 0 || end < 0 {
		return "", fmt.Errorf("SQLite notice: no header comment in %s", sqliteHeader)
	}
	var b strings.Builder
	for _, line := range lines[start:end] {
		line = strings.TrimPrefix(line, "**")
		line = strings.TrimPrefix(line, " ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	text, err := normalizeText([]byte(b.String()))
	if err != nil {
		return "", fmt.Errorf("SQLite notice: %w", err)
	}
	if !strings.Contains(text, sqliteDisclaimer) {
		return "", fmt.Errorf("SQLite notice: the header comment of %s no longer contains %q; review the SQLite license", sqliteHeader, sqliteDisclaimer)
	}
	return text, nil
}

// checkDockerfile verifies that every Alpine image in the Dockerfile uses the
// Alpine release whose musl COPYRIGHT is embedded, and that its go commands
// build with linuxReleaseTags.
func checkDockerfile(root string) error {
	data, err := readFile(root, dockerfile)
	if err != nil {
		return err
	}
	found := false
	for _, m := range dockerFrom.FindAllStringSubmatch(data, -1) {
		a := alpineRelease.FindStringSubmatch(m[1])
		if a == nil {
			continue
		}
		found = true
		if a[1] != muslAlpineRelease {
			return fmt.Errorf("%s uses Alpine %s (%s), but tools/licenses embeds the musl %s COPYRIGHT for Alpine %s; update musl-COPYRIGHT, muslVersion and muslAlpineRelease",
				dockerfile, a[1], m[1], muslVersion, muslAlpineRelease)
		}
	}
	if !found {
		return fmt.Errorf("%s has no Alpine image; update the musl section of tools/licenses", dockerfile)
	}
	tags := dockerTags.FindAllStringSubmatch(data, -1)
	if len(tags) == 0 {
		return fmt.Errorf("%s builds without -tags; tools/licenses lists the Linux modules for %s", dockerfile, linuxReleaseTags)
	}
	for _, t := range tags {
		if t[1] != linuxReleaseTags {
			return fmt.Errorf("%s builds with -tags %s, but tools/licenses lists the Linux modules for %s; update linuxReleaseTags", dockerfile, t[1], linuxReleaseTags)
		}
	}
	return nil
}

// includedIn describes the platforms a component is compiled for.
func includedIn(set map[platform]struct{}, all []platform) string {
	if len(set) == len(all) {
		return "all binaries"
	}
	names := map[string]string{"linux": "Linux", "darwin": "macOS", "windows": "Windows"}
	var parts []string
	var order []string
	byOS := map[string][]platform{}
	for _, p := range all {
		if _, ok := byOS[p.goos]; !ok {
			order = append(order, p.goos)
		}
		byOS[p.goos] = append(byOS[p.goos], p)
	}
	for _, goos := range order {
		var have []string
		for _, p := range byOS[goos] {
			if _, ok := set[p]; ok {
				have = append(have, p.goos+"/"+p.goarch)
			}
		}
		switch {
		case len(have) == 0:
		case len(have) == len(byOS[goos]):
			parts = append(parts, names[goos]+" binaries")
		default:
			parts = append(parts, have...)
		}
	}
	return strings.Join(parts, ", ")
}

// render writes the sections in a stable plain-text layout.
func render(sections []section) string {
	var b strings.Builder
	b.WriteString(`Third-party software licenses for Capsus
========================================

The Capsus release binaries include the software listed below. Each component
is distributed under its own license, reproduced in full in this file; the
Capsus license is in LICENSE and the attribution summary in NOTICE.txt. The
bundled tools are listed under their module paths, with their own LICENSE and
NOTICE files. A binary records the module versions compiled into it
(` + "`go version -m <binary>`" + ` lists them). The compiler runtime libraries that
the C toolchains link in (libgcc, LLVM's compiler-rt) are under the GCC Runtime
Library Exception or the LLVM exception, which ask for no notice; the macOS
binaries use Apple's system libraries without containing them.

This file is generated by ` + "`make licenses`" + ` (` + "`go run ./tools/licenses`" + `) and
checked by ` + "`go test ./tools/licenses`" + ` (or ` + "`go run ./tools/licenses -check`" + `);
do not edit it by hand.

Contents:
`)
	for _, s := range sections {
		b.WriteString("  - " + s.title + "\n")
	}
	rule := strings.Repeat("=", ruleWidth)
	for _, s := range sections {
		b.WriteString("\n" + rule + "\n")
		b.WriteString(s.title + "\n")
		b.WriteString("Included in: " + s.included + "\n")
		b.WriteString(rule + "\n")
		for _, t := range s.texts {
			b.WriteString("\n--- " + t.name + " ---\n\n")
			b.WriteString(t.text)
		}
	}
	return b.String()
}

// normalizeText returns text with a single trailing newline, LF line endings,
// no byte order mark, no trailing spaces and no leading or trailing blank lines.
func normalizeText(data []byte) (string, error) {
	if !utf8.Valid(data) {
		return "", errors.New("not valid UTF-8")
	}
	s := strings.TrimPrefix(string(data), string(rune(0xFEFF)))
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	s = strings.Trim(strings.Join(lines, "\n"), "\n")
	if strings.TrimSpace(s) == "" {
		return "", errors.New("empty license text")
	}
	return s + "\n", nil
}

// noticeProblems reports modules missing from NOTICE.txt and NOTICE.txt
// entries for modules that are no longer compiled in.
func noticeProblems(notice string, mods []*module) []string {
	notice = strings.ReplaceAll(notice, "\r\n", "\n")
	listed := map[string]struct{}{}
	for _, m := range noticeModuleLine.FindAllStringSubmatch(notice, -1) {
		listed[m[1]] = struct{}{}
	}
	compiled := map[string]struct{}{}
	var problems []string
	for _, m := range mods {
		compiled[m.path] = struct{}{}
		if _, ok := listed[m.path]; !ok {
			problems = append(problems, "does not list "+m.path)
		}
	}
	var extra []string
	for path := range listed {
		if _, ok := compiled[path]; !ok {
			extra = append(extra, path)
		}
	}
	sort.Strings(extra)
	for _, path := range extra {
		problems = append(problems, "lists "+path+", which no release binary includes")
	}
	return problems
}

// firstDifference returns the first line (1-based) at which a and b differ.
func firstDifference(a, b string) (int, bool) {
	if a == b {
		return 0, false
	}
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(al) && i < len(bl); i++ {
		if al[i] != bl[i] {
			return i + 1, true
		}
	}
	return min(len(al), len(bl)) + 1, true
}

// readFile reads the file name in dir: a repository file, a license file of
// the Go distribution or a file of a module. name is a single path element.
func readFile(dir, name string) (string, error) {
	data, err := fs.ReadFile(os.DirFS(dir), name)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", filepath.Join(dir, name), err)
	}
	return string(data), nil
}
