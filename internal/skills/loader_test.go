package skills

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseSkillFrontmatter(t *testing.T) {
	t.Parallel()

	raw := `---
name: repo-helper
description: Help with repository work
when_to_use: When editing repo files
context: fork
allowed-tools:
  - read_file
  - grep_files, execute_shell_command
user-invocable: true
---
# Repo Helper

Use this skill for repo tasks.
`

	skill, err := parseSkill("default-name", filepath.Join("skills", "repo-helper", "SKILL.md"), "project", raw)
	if err != nil {
		t.Fatalf("parseSkill returned error: %v", err)
	}

	if skill.Name != "repo-helper" {
		t.Fatalf("expected name repo-helper, got %q", skill.Name)
	}
	if skill.Description != "Help with repository work" {
		t.Fatalf("expected description to be parsed, got %q", skill.Description)
	}
	if skill.WhenToUse != "When editing repo files" {
		t.Fatalf("expected when_to_use to be parsed, got %q", skill.WhenToUse)
	}
	if skill.Context != "fork" {
		t.Fatalf("expected context fork, got %q", skill.Context)
	}

	wantTools := []string{"read_file", "grep_files", "execute_shell_command"}
	if !reflect.DeepEqual(skill.AllowedTools, wantTools) {
		t.Fatalf("expected allowed tools %v, got %v", wantTools, skill.AllowedTools)
	}
	if !skill.UserInvocable {
		t.Fatalf("expected skill to be user invocable")
	}
}

func TestParseSkillDefaults(t *testing.T) {
	t.Parallel()

	raw := `# Default Skill

First non-empty line becomes the description.
`

	skill, err := parseSkill("default-skill", filepath.Join("skills", "default-skill", "SKILL.md"), "user", raw)
	if err != nil {
		t.Fatalf("parseSkill returned error: %v", err)
	}

	if skill.Name != "default-skill" {
		t.Fatalf("expected default name, got %q", skill.Name)
	}
	if skill.Description != "# Default Skill" {
		t.Fatalf("expected first non-empty line as description, got %q", skill.Description)
	}
	if skill.Context != "session" {
		t.Fatalf("expected default context session, got %q", skill.Context)
	}
	if len(skill.AllowedTools) != 0 {
		t.Fatalf("expected no allowed tools by default, got %v", skill.AllowedTools)
	}
}

func TestLoadDirRecursivelyFindsNestedSkills(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	nested := filepath.Join(root, "backend", "java-maven")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested skill dir: %v", err)
	}

	content := `---
description: Maven helper
---
# Java Maven

Use for pom.xml and mvn issues.
`
	if err := os.WriteFile(filepath.Join(nested, skillFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write nested SKILL.md: %v", err)
	}

	skills, err := loadDir(root, "project")
	if err != nil {
		t.Fatalf("loadDir returned error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "java-maven" {
		t.Fatalf("expected nested dir name java-maven, got %q", skills[0].Name)
	}
	if skills[0].Scope != "project" {
		t.Fatalf("expected scope project, got %q", skills[0].Scope)
	}
}
