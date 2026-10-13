package cli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/OPDhaker/jingle/internal/output"
	skill "github.com/OPDhaker/jingle/plugin/skills/jingle"
)

type skillDoc struct {
	Skill    string `json:"skill"`
	Markdown string `json:"markdown"`
}

func newSkillCmd(printer func(*cobra.Command) *output.Printer) *cobra.Command {
	return &cobra.Command{
		Use:   "skill",
		Short: "Print setup instructions for AI agents (a Claude Code skill)",
		Long: `Print jingle's agent skill: step-by-step instructions an AI coding agent
follows to install, configure, check, and remove jingle. It is a Claude Code
skill (SKILL.md with frontmatter), and plain markdown for any other agent. It
matches this jingle version.

Claude Code users can also install it from the plugin marketplace:
  /plugin marketplace add OPDhaker/jingle
  /plugin install jingle@jingle`,
		Example: `  jingle skill
  mkdir -p ~/.claude/skills/jingle && jingle skill > ~/.claude/skills/jingle/SKILL.md
  jingle skill --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return printer(cmd).Result(skillDoc{Skill: "jingle", Markdown: skill.Markdown}, func(w io.Writer) {
				_, _ = io.WriteString(w, skill.Markdown)
			})
		},
	}
}
