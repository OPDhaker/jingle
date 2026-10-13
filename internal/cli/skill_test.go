package cli

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/OPDhaker/jingle/internal/builtin"
	"github.com/OPDhaker/jingle/internal/config"
	skill "github.com/OPDhaker/jingle/plugin/skills/jingle"
)

// TestSkillMatchesCLI keeps the agent skill honest: every jingle command,
// flag, config key, and built-in sound it shows in code must exist.
func TestSkillMatchesCLI(t *testing.T) {
	md := skill.Markdown
	if !strings.HasPrefix(md, "---\nname: jingle\ndescription: ") {
		t.Fatalf("SKILL.md frontmatter must start with name and description:\n%.80s", md)
	}
	var jsonMode bool
	root := newRootCmd(&jsonMode)

	// Code: fenced blocks line by line, plus inline `spans`.
	var code []string
	inBlock := false
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inBlock = !inBlock
			continue
		}
		if inBlock {
			code = append(code, line)
			continue
		}
		for _, m := range regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(line, -1) {
			code = append(code, m[1])
		}
	}

	checked := 0
	for _, c := range code {
		if i := strings.Index(c, " #"); i >= 0 {
			c = c[:i]
		}
		for _, seg := range regexp.MustCompile(`&&|;|\|`).Split(c, -1) {
			args := strings.Fields(seg)
			if len(args) < 2 || args[0] != "jingle" {
				continue // not a jingle invocation (or a path ending in /jingle)
			}
			args = args[1:]
			if i := slices.Index(args, ">"); i >= 0 {
				args = args[:i]
			}
			cmd, rest, err := root.Find(args)
			if err != nil || cmd == root {
				t.Errorf("SKILL.md: `jingle %s` is not a command", strings.Join(args, " "))
				continue
			}
			checked++
			for _, a := range rest {
				if name, ok := strings.CutPrefix(a, "--"); ok && cmd.Flag(name) == nil {
					t.Errorf("SKILL.md: `%s` has no flag --%s", cmd.CommandPath(), name)
				}
			}
			if (cmd.Name() == "set" || cmd.Name() == "get" || cmd.Name() == "add") && len(rest) > 0 &&
				!strings.HasPrefix(rest[0], "-") && !slices.Contains(config.Keys, rest[0]) {
				t.Errorf("SKILL.md: %q is not a config key", rest[0])
			}
		}
	}
	if checked < 10 {
		t.Fatalf("only %d jingle invocations found in SKILL.md; is the parser broken?", checked)
	}
	for _, m := range regexp.MustCompile(`builtin:([a-z0-9]+)`).FindAllStringSubmatch(md, -1) {
		if !builtin.Exists(m[1]) {
			t.Errorf("SKILL.md: builtin:%s is not a built-in sound", m[1])
		}
	}
	for _, name := range builtin.Names() {
		if !strings.Contains(md, builtin.Prefix+name) {
			t.Errorf("SKILL.md does not list builtin:%s", name)
		}
	}
}
