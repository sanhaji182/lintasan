package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoBuildTargetWritesToProdBinary keeps the Makefile from staging code into
// production as a side effect of building.
//
// ./lintasan is the path systemd executes (ExecStart=/home/ubuntu/lintasan-go/
// lintasan start). Anything that writes there is a deploy, whether or not it was
// meant as one: the next restart for ANY reason picks it up. On 9 Aug 2026 at
// 16:31:49 prod served v0.29.3-17-g23bbc7c, and seven seconds later a stop/start
// from a different worktree brought up v0.29.3-16-g389ec80 — an older commit —
// because a build had left its binary at the prod path. Nobody deployed; prod
// went backwards for 100 seconds.
//
// The fix was to send builds to dist-bin/ and make `deploy` the only target
// allowed at the prod path. That rule lives in the Makefile, which no test
// covers, so it is one careless -o away from silently regressing. This test is
// the enforcement.
func TestNoBuildTargetWritesToProdBinary(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Skipf("Makefile not present (%v) — nothing to check", err)
	}

	targets := parseMakeTargets(string(raw))
	if len(targets) == 0 {
		t.Fatal("parsed no targets from the Makefile — the parser or the file format changed")
	}
	for _, required := range []string{"build", "backend", "deploy", "clean", "release"} {
		if _, ok := targets[required]; !ok {
			t.Fatalf("target %q missing from the Makefile — this guard assumes it exists", required)
		}
	}

	// `go build -o <path>` / `-o=<path>`: the output path is what matters.
	outRe := regexp.MustCompile(`-o[= ]+(\S+)`)
	// A bare copy/move onto the prod path is a deploy by another name.
	cpRe := regexp.MustCompile(`\b(?:cp|mv|install)\b[^\n]*?\s(\.?/?\$\(BINARY\)|\.?/?lintasan)(\s|$)`)

	for name, body := range targets {
		if name == "deploy" {
			continue // the one target whose whole job is to write there
		}
		for _, m := range outRe.FindAllStringSubmatch(body, -1) {
			if isProdBinaryPath(m[1]) {
				t.Errorf("target %q builds to %s — that is the path systemd executes; "+
					"build into $(BUILD_DIR) and let `make deploy` install it", name, m[1])
			}
		}
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "@echo") {
				continue
			}
			if cpRe.MatchString(line) {
				t.Errorf("target %q copies onto the production binary path:\n\t%s\n"+
					"only `make deploy` may write there", name, line)
			}
		}
	}

	// `clean` deleting the prod binary turns the next restart into an outage:
	// systemd would have no file to execute.
	clean := targets["clean"]
	if regexp.MustCompile(`rm\s+(-\w+\s+)*(\./)?(\$\(BINARY\)|lintasan)(\s|$)`).MatchString(clean) {
		t.Errorf("target \"clean\" removes the production binary — the next restart would fail "+
			"with no file to execute:\n%s", clean)
	}

	// deploy must back up before overwriting, or a rollback becomes a rebuild.
	deploy := targets["deploy"]
	if !strings.Contains(deploy, ".bak-") {
		t.Error("target \"deploy\" does not take a timestamped backup of the current prod binary — " +
			"rollback should be a copy, not a rebuild")
	}
	if !strings.Contains(deploy, "systemctl stop") {
		t.Error("target \"deploy\" does not stop the service before replacing the binary — " +
			"overwriting a running executable gives \"text file busy\"")
	}
}

// parseMakeTargets maps target name -> recipe body (the indented lines under it).
func parseMakeTargets(src string) map[string]string {
	targets := map[string]string{}
	nameRe := regexp.MustCompile(`^([A-Za-z0-9_.-]+)\s*:(?:[^=]|$)`)

	var current string
	var body []string
	flush := func() {
		if current != "" {
			targets[current] = strings.Join(body, "\n")
		}
		current, body = "", nil
	}
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(line, "\t") {
			if current != "" {
				body = append(body, line)
			}
			continue
		}
		flush()
		if m := nameRe.FindStringSubmatch(line); m != nil && m[1] != ".PHONY" {
			current = m[1]
		}
	}
	flush()
	return targets
}

// isProdBinaryPath reports whether a make output path resolves to the binary
// systemd executes — bare "lintasan", "./lintasan", or "$(BINARY)" at the repo
// root. A path with a directory prefix (dist-bin/lintasan) is fine.
func isProdBinaryPath(p string) bool {
	p = strings.TrimPrefix(strings.Trim(p, `"'`), "./")
	return p == "lintasan" || p == "$(BINARY)"
}
