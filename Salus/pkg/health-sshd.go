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

package pkg

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// SSHDConfigEnv overrides the sshd_config file that the sshd misconfig rules
// read, for example a host's configuration mounted into a container.
const SSHDConfigEnv = "SALUS_SSHD_CONFIG"

// maxSSHDIncludeDepth matches OpenSSH's limit on nested Include directives.
const maxSSHDIncludeDepth = 16

// sshdKeywords are the (lowercase) keywords the rules read.
var sshdKeywords = map[string]bool{"permitrootlogin": true, "passwordauthentication": true}

// sshdConfigPath returns the sshd_config to read: SALUS_SSHD_CONFIG, or the
// platform default.
func sshdConfigPath() string {
	if path := os.Getenv(SSHDConfigEnv); path != "" {
		return path
	}
	if runtime.GOOS == "windows" {
		programData := os.Getenv("ProgramData")
		if programData == "" {
			return ""
		}
		return filepath.Join(programData, "ssh", "sshd_config")
	}
	return "/etc/ssh/sshd_config"
}

// sshdRootLogin warns when sshd accepts root logins with any authentication
// method, including passwords.
func sshdRootLogin() []string {
	path, settings, ok := readSSHDConfig()
	if !ok {
		return nil
	}
	if settings["permitrootlogin"] == "yes" {
		return []string{fmt.Sprintf("%s permits root login with any authentication method (PermitRootLogin yes; use prohibit-password or no)", path)}
	}
	return nil
}

// sshdPasswordAuth warns when sshd accepts passwords, which exposes the host
// to password guessing. OpenSSH enables it when the keyword is unset, so an
// unset keyword warns too (Q-013).
func sshdPasswordAuth() []string {
	path, settings, ok := readSSHDConfig()
	if !ok {
		return nil
	}
	switch value, set := settings["passwordauthentication"]; {
	case !set:
		return []string{fmt.Sprintf("%s does not set PasswordAuthentication, which OpenSSH enables by default (set PasswordAuthentication no)", path)}
	case value == "yes":
		return []string{fmt.Sprintf("%s enables password authentication (PasswordAuthentication yes; set it to no)", path)}
	}
	return nil
}

// readSSHDConfig returns the effective global values of sshdKeywords, as
// sshd would apply them to a connection that matches no Match block. It
// reports false, and the rules stay silent, when sshd is not configured on
// this host or any part of the configuration cannot be read; a guess could
// report a setting that an unreadable file overrides.
func readSSHDConfig() (string, map[string]string, bool) {
	path := sshdConfigPath()
	if path == "" {
		return "", nil, false
	}
	parser := sshdParser{settings: map[string]string{}, baseDir: filepath.Dir(path)}
	parser.relocated = os.Getenv(SSHDConfigEnv) != "" && filepath.ToSlash(filepath.Clean(parser.baseDir)) != "/etc/ssh"
	if err := parser.parseFile(path, 0); err != nil {
		return path, nil, false
	}
	return path, parser.settings, true
}

type sshdParser struct {
	settings map[string]string
	// baseDir resolves relative Include paths. OpenSSH resolves them against
	// /etc/ssh, which is the directory of the default configuration file.
	baseDir string
	// relocated means SALUS_SSHD_CONFIG names a configuration outside
	// /etc/ssh, such as a host's /etc/ssh mounted into a container. Its
	// absolute Include paths under /etc/ssh are then read from baseDir, and
	// other absolute Include paths cannot be resolved.
	relocated bool
}

// parseFile reads one configuration file. A Match line makes the lines after
// it conditional, until a "Match all" line makes them global again; sshd
// applies "Match all" blocks at startup. sshd restores the global state after
// an included file, so a Match block in an included file ends with that file.
// Conditional lines, including Include lines, do not apply to every
// connection and are skipped.
func (p *sshdParser) parseFile(path string, depth int) error {
	if depth > maxSSHDIncludeDepth {
		return fmt.Errorf("include depth exceeds %d at %s", maxSSHDIncludeDepth, path)
	}
	// A FIFO or device would block or never end; sshd_config is a regular file.
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }() // read-only; a close error cannot lose data

	global := true
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		keyword, args := splitSSHDLine(scanner.Text())
		switch {
		case keyword == "":
			continue
		case keyword == "match":
			global = len(args) == 1 && strings.EqualFold(args[0], "all")
		case !global:
			continue
		case keyword == "include":
			for _, pattern := range args {
				files, err := p.includeFiles(pattern)
				if err != nil {
					return err
				}
				for _, file := range files {
					if err := p.parseFile(file, depth+1); err != nil {
						return err
					}
				}
			}
		case sshdKeywords[keyword] && len(args) > 0:
			if _, seen := p.settings[keyword]; !seen {
				// sshd uses the first value it reads for these keywords.
				p.settings[keyword] = strings.ToLower(args[0])
			}
		}
	}
	return scanner.Err()
}

// includeFiles expands one Include argument to the files it names, in
// lexical order, as glob(3) does: wildcards do not match a leading dot. A
// pattern that matches nothing is not an error. A directory that exists but
// cannot be listed is, because the files it hides may change the result.
func (p *sshdParser) includeFiles(pattern string) ([]string, error) {
	const sshDir = "/etc/ssh/"
	switch slashed := filepath.ToSlash(pattern); {
	case p.relocated && strings.HasPrefix(slashed, sshDir):
		pattern = filepath.Join(p.baseDir, filepath.FromSlash(strings.TrimPrefix(slashed, sshDir)))
	case p.relocated && (filepath.IsAbs(pattern) || strings.HasPrefix(slashed, "/")):
		return nil, fmt.Errorf("cannot resolve Include %s for a configuration outside /etc/ssh", pattern)
	case !filepath.IsAbs(pattern):
		pattern = filepath.Join(p.baseDir, pattern)
	}

	dir, base := filepath.Split(pattern)
	if hasGlobMeta(dir) {
		// Wildcards in directory components are rare; filepath.Glob handles
		// them but ignores unreadable directories.
		return filepath.Glob(pattern)
	}
	if !hasGlobMeta(base) {
		if _, err := os.Stat(pattern); errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return []string{pattern}, nil
	}

	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".") {
			continue
		}
		if ok, err := filepath.Match(base, name); err != nil {
			return nil, err
		} else if ok && !entry.IsDir() {
			files = append(files, filepath.Join(dir, name))
		}
	}
	sort.Strings(files)
	return files, nil
}

// hasGlobMeta reports whether s contains a filepath.Match metacharacter.
// Backslash escapes only outside Windows, where it separates paths.
func hasGlobMeta(s string) bool {
	if runtime.GOOS == "windows" {
		return strings.ContainsAny(s, "*?[")
	}
	return strings.ContainsAny(s, `*?[\`)
}

// splitSSHDLine returns the lowercase keyword and the arguments of one
// sshd_config line. Keywords are separated from arguments by whitespace or
// "="; arguments may be in double or single quotes; "#" starts a comment at
// the beginning of a line or of an argument. Blank and comment lines return
// "". Backslash escapes are not interpreted.
func splitSSHDLine(line string) (string, []string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", nil
	}
	end := strings.IndexFunc(line, func(r rune) bool { return r == '=' || r == ' ' || r == '\t' })
	if end < 0 {
		return strings.ToLower(line), nil
	}
	keyword := strings.ToLower(line[:end])
	rest := strings.TrimLeft(line[end:], " \t")
	rest = strings.TrimLeft(strings.TrimPrefix(rest, "="), " \t")

	var args []string
	for rest != "" {
		var arg string
		if quote := rest[0]; quote == '"' || quote == '\'' {
			closing := strings.IndexByte(rest[1:], quote)
			if closing < 0 {
				arg, rest = rest[1:], ""
			} else {
				arg, rest = rest[1:closing+1], rest[closing+2:]
			}
		} else {
			if rest[0] == '#' {
				break
			}
			end := strings.IndexAny(rest, " \t")
			if end < 0 {
				end = len(rest)
			}
			arg, rest = rest[:end], rest[end:]
		}
		args = append(args, arg)
		rest = strings.TrimLeft(rest, " \t")
	}
	return keyword, args
}
