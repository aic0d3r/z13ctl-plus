package cmd

// setup_test.go — the shipped contrib files must match what `setup` generates
// for the default group; otherwise packages and `setup` grant different access.

import (
	"os"
	"strings"
	"testing"
)

// effectiveLines drops comments and blank lines so only behaviour is compared.
func effectiveLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestContribMatchesGeneratedSetupFiles(t *testing.T) {
	for _, tc := range []struct {
		name, path, generated string
	}{
		{"udev rules", "../contrib/udev/99-z13ctl-plus.rules", buildRulesContent("users")},
		{"perms service", "../contrib/systemd/system/z13ctl-plus-perms.service", buildServiceContent("users")},
	} {
		data, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		got, want := effectiveLines(string(data)), effectiveLines(tc.generated)
		if len(got) != len(want) {
			t.Errorf("%s: %d effective lines in contrib, %d generated", tc.name, len(got), len(want))
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s line %d differs:\n contrib:   %s\n generated: %s", tc.name, i+1, got[i], want[i])
			}
		}
	}
}
