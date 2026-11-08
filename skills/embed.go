// Package skills embeds the AI agent skills shipped with devopspadcli.
package skills

import "embed"

// FS holds devopspad/SKILL.md and its workflow and reference files.
//
//go:embed devopspad
var FS embed.FS
