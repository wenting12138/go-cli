package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go-cli/internal/agentmode"
	"go-cli/internal/config"
	"go-cli/internal/modelclient/protocol"
)

const DynamicBoundary = "__SYSTEM_PROMPT_DYNAMIC_BOUNDARY__"

type Input struct {
	Model      config.ModelConfig
	Mode       agentmode.Mode
	WorkingDir string
	Tools      []protocol.ToolDefinition
}

type Section struct {
	Name     string
	Dynamic  bool
	Compute  func(Input) (string, error)
	CacheKey func(Input) (string, error)
}

type Builder struct {
	mu       sync.RWMutex
	sections []Section
	cache    map[string]string
}

func NewBuilder() *Builder {
	return &Builder{
		sections: defaultSections(),
		cache:    map[string]string{},
	}
}

func (b *Builder) Build(input Input) (string, error) {
	parts := make([]string, 0, len(b.sections)+1)
	boundaryInserted := false

	for _, section := range b.sections {
		if section.Dynamic && !boundaryInserted {
			parts = append(parts, DynamicBoundary)
			boundaryInserted = true
		}

		value, err := b.resolveSection(section, input)
		if err != nil {
			return "", err
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parts = append(parts, value)
	}

	return strings.Join(parts, "\n\n"), nil
}

func (b *Builder) resolveSection(section Section, input Input) (string, error) {
	if section.Dynamic {
		return section.Compute(input)
	}

	key := section.Name
	if section.CacheKey != nil {
		computedKey, err := section.CacheKey(input)
		if err != nil {
			return "", err
		}
		key = computedKey
	}

	b.mu.RLock()
	if cached, ok := b.cache[key]; ok {
		b.mu.RUnlock()
		return cached, nil
	}
	b.mu.RUnlock()

	value, err := section.Compute(input)
	if err != nil {
		return "", err
	}

	b.mu.Lock()
	b.cache[key] = value
	b.mu.Unlock()
	return value, nil
}

func defaultSections() []Section {
	return []Section{
		{
			Name:    "intro",
			Compute: func(Input) (string, error) { return introSection(), nil },
		},
		{
			Name:    "system",
			Compute: func(Input) (string, error) { return systemSection(), nil },
		},
		{
			Name:    "workflow",
			Compute: func(input Input) (string, error) { return workflowSection(input.Mode), nil },
		},
		{
			Name:    "repo_guidance",
			Compute: func(input Input) (string, error) { return repoGuidanceSection(input.WorkingDir) },
			CacheKey: func(input Input) (string, error) {
				paths, err := findAgentFiles(input.WorkingDir)
				if err != nil {
					return "", err
				}
				if len(paths) == 0 {
					return "repo_guidance:none:" + input.WorkingDir, nil
				}

				parts := make([]string, 0, len(paths))
				for _, path := range paths {
					info, err := os.Stat(path)
					if err != nil {
						return "", err
					}
					parts = append(parts, fmt.Sprintf("%s:%d", path, info.ModTime().UnixNano()))
				}
				return "repo_guidance:" + strings.Join(parts, "|"), nil
			},
		},
		{
			Name:    "tools",
			Dynamic: true,
			Compute: func(input Input) (string, error) { return toolsSection(input.Tools), nil },
		},
		{
			Name:    "environment",
			Dynamic: true,
			Compute: func(input Input) (string, error) { return environmentSection(input), nil },
		},
		{
			Name:    "session_guidance",
			Dynamic: true,
			Compute: func(Input) (string, error) { return sessionGuidanceSection(), nil },
		},
	}
}

func introSection() string {
	return strings.TrimSpace(`
# Role
You are an interactive CLI coding assistant. Help the user complete software engineering tasks inside the current workspace.
`)
}

func systemSection() string {
	return strings.TrimSpace(`
# System
- All non-tool text you produce is shown to the user.
- Use tools when they reduce uncertainty or perform deterministic work.
- Be concise, accurate, and explicit about verification results.
- Do not invent file contents, command output, or tool results.
`)
}

func workflowSection(mode agentmode.Mode) string {
	if mode == agentmode.Plan {
		return strings.TrimSpace(`
# Workflow
- You are in plan mode.
- Explore the codebase and design the implementation before making changes.
- Do not propose speculative code edits as if they were already applied.
- Prefer read-only tools to reduce uncertainty.
- End with a concrete implementation plan, critical files, and verification steps.
- When the plan is ready for review, call exit_plan_mode instead of asking for approval in plain text.
`)
	}

	return strings.TrimSpace(`
# Workflow
- Read relevant code before changing it.
- Keep changes scoped to the user's request.
- Prefer deterministic decisions in code over routing decisions through the model.
- If you make non-trivial changes, verify them before reporting completion.
`)
}

func repoGuidanceSection(workingDir string) (string, error) {
	paths, err := findAgentFiles(workingDir)
	if err != nil {
		return "", err
	}

	if len(paths) == 0 {
		return "", nil
	}

	blocks := []string{"# Repository Guidance"}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		text := strings.TrimSpace(string(data))
		if text == "" {
			continue
		}
		rel := path
		if relative, err := filepath.Rel(workingDir, path); err == nil {
			rel = relative
		}
		blocks = append(blocks, fmt.Sprintf("## %s\n%s", rel, text))
	}

	if len(blocks) == 1 {
		return "", nil
	}
	return strings.Join(blocks, "\n\n"), nil
}

func toolsSection(definitions []protocol.ToolDefinition) string {
	if len(definitions) == 0 {
		return ""
	}

	names := make([]string, 0, len(definitions))
	descriptions := make(map[string]string, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
		descriptions[definition.Name] = strings.TrimSpace(definition.Description)
	}
	sort.Strings(names)

	lines := []string{"# Tools", "Use the provided tools when needed. Available tools:"}
	for _, name := range names {
		line := "- " + name
		if description := descriptions[name]; description != "" {
			line += ": " + description
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func environmentSection(input Input) string {
	items := []string{
		"# Environment",
		fmt.Sprintf("- Working directory: %s", input.WorkingDir),
		fmt.Sprintf("- Today's date: %s", time.Now().Format("2006-01-02")),
		fmt.Sprintf("- Active model id: %s", input.Model.ID),
		fmt.Sprintf("- Provider: %s", input.Model.Provider),
		fmt.Sprintf("- Model name: %s", input.Model.Name),
		fmt.Sprintf("- Context window: %d", input.Model.ContextWindow),
	}
	return strings.Join(items, "\n")
}

func sessionGuidanceSection() string {
	return strings.TrimSpace(`
# Session Guidance
- Keep responses compact unless the task requires detail.
- If tool output is important for later reasoning, summarize the important part in your own words.
- When blocked, explain the blocker clearly instead of guessing.
`)
}

func findAgentFiles(workingDir string) ([]string, error) {
	dir, err := filepath.Abs(workingDir)
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}

	var paths []string
	for {
		path := filepath.Join(dir, "AGENTS.md")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			paths = append(paths, path)
		} else if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	for i, j := 0, len(paths)-1; i < j; i, j = i+1, j-1 {
		paths[i], paths[j] = paths[j], paths[i]
	}
	return paths, nil
}
