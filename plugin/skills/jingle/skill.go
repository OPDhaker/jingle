// Package skill embeds the agent skill (SKILL.md, next to this file) so
// `jingle skill` prints the guide that matches the binary. The same file is
// the Claude Code plugin's skill, so there is one copy to edit.
package skill

import _ "embed"

// Markdown is SKILL.md, frontmatter included.
//
//go:embed SKILL.md
var Markdown string
