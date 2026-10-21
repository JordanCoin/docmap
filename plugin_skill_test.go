package main

import (
	"os"
	"testing"
)

// The marketplace plugin installs plugins/docmap only, so its SKILL.md must be
// the full skill, not a pointer back into the repo.
func TestPluginSkillMatchesRepoSkill(t *testing.T) {
	repo, err := os.ReadFile(".claude/skills/docmap/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	plugin, err := os.ReadFile("plugins/docmap/skills/docmap/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(repo) != string(plugin) {
		t.Fatal("plugins/docmap/skills/docmap/SKILL.md differs from .claude/skills/docmap/SKILL.md; copy the repo skill over it")
	}
	if len(plugin) < 1000 {
		t.Fatalf("plugin SKILL.md is %d bytes, looks like a stub", len(plugin))
	}
}
