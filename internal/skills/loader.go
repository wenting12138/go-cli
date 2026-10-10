package skills

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const skillFileName = "SKILL.md"

type Skill struct {
	Name          string
	Description   string
	WhenToUse     string
	Content       string
	Path          string
	Scope         string
	Context       string
	AllowedTools  []string
	UserInvocable bool
}

func Load(workingDir string) ([]Skill, error) {
	dirs, err := skillDirs(workingDir)
	if err != nil {
		return nil, err
	}

	loaded := make(map[string]Skill)
	order := make([]string, 0, 16)
	for _, dir := range dirs {
		skills, err := loadDir(dir.path, dir.scope)
		if err != nil {
			return nil, err
		}
		for _, skill := range skills {
			if _, exists := loaded[skill.Name]; !exists {
				order = append(order, skill.Name)
			}
			loaded[skill.Name] = skill
		}
	}

	out := make([]Skill, 0, len(order))
	seen := make(map[string]struct{}, len(order))
	for i := len(order) - 1; i >= 0; i-- {
		name := order[i]
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, loaded[name])
	}
	reverse(out)
	return out, nil
}

func Find(workingDir, name string) (Skill, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Skill{}, fmt.Errorf("skill name cannot be empty")
	}
	skills, err := Load(workingDir)
	if err != nil {
		return Skill{}, err
	}
	for _, skill := range skills {
		if skill.Name == name {
			return skill, nil
		}
	}
	return Skill{}, fmt.Errorf("skill %q not found", name)
}

type skillDir struct {
	path  string
	scope string
}

func skillDirs(workingDir string) ([]skillDir, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	return []skillDir{
		{path: filepath.Join(home, ".go-cli", "skills"), scope: "user"},
		{path: filepath.Join(workingDir, ".go-cli", "skills"), scope: "project"},
	}, nil
}

func loadDir(root, scope string) ([]Skill, error) {
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat skills dir %s: %w", root, err)
	}

	out := make([]Skill, 0, 16)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(d.Name(), skillFileName) {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		defaultName := filepath.Base(filepath.Dir(path))
		skill, parseErr := parseSkill(defaultName, path, scope, string(data))
		if parseErr != nil {
			return parseErr
		}
		out = append(out, skill)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk skills dir %s: %w", root, err)
	}
	return out, nil
}

func parseSkill(defaultName, path, scope, raw string) (Skill, error) {
	frontmatter, body := splitFrontmatter(raw)
	name := defaultName
	description := ""
	whenToUse := ""
	contextMode := "session"
	allowedTools := []string{}
	userInvocable := true

	for key, values := range parseFrontmatter(frontmatter) {
		switch key {
		case "name":
			if len(values) > 0 && strings.TrimSpace(values[0]) != "" {
				name = strings.TrimSpace(values[0])
			}
		case "description":
			if len(values) > 0 {
				description = strings.TrimSpace(values[0])
			}
		case "when_to_use":
			if len(values) > 0 {
				whenToUse = strings.TrimSpace(values[0])
			}
		case "context":
			if len(values) > 0 {
				contextMode = normalizeContext(values[0])
			}
		case "allowed-tools", "allowed_tools":
			allowedTools = normalizeList(values)
		case "user-invocable":
			if len(values) > 0 {
				userInvocable = strings.EqualFold(strings.TrimSpace(values[0]), "true")
			}
		}
	}

	body = strings.TrimSpace(body)
	if body == "" {
		return Skill{}, fmt.Errorf("skill %s is empty", path)
	}
	if description == "" {
		description = firstNonEmptyLine(body)
	}

	return Skill{
		Name:          name,
		Description:   description,
		WhenToUse:     whenToUse,
		Content:       body,
		Path:          path,
		Scope:         scope,
		Context:       contextMode,
		AllowedTools:  allowedTools,
		UserInvocable: userInvocable,
	}, nil
}

func splitFrontmatter(raw string) (string, string) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(raw, "---\n") {
		return "", raw
	}
	rest := strings.TrimPrefix(raw, "---\n")
	index := strings.Index(rest, "\n---\n")
	if index < 0 {
		return "", raw
	}
	return rest[:index], rest[index+5:]
}

func parseFrontmatter(raw string) map[string][]string {
	result := make(map[string][]string)
	if strings.TrimSpace(raw) == "" {
		return result
	}

	scanner := bufio.NewScanner(strings.NewReader(raw))
	currentKey := ""
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), " \t")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") && currentKey != "" {
			result[currentKey] = append(result[currentKey], strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
			continue
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			currentKey = ""
			continue
		}
		currentKey = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if value == "" {
			if _, exists := result[currentKey]; !exists {
				result[currentKey] = nil
			}
			continue
		}
		result[currentKey] = []string{value}
	}
	return result
}

func normalizeContext(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "fork":
		return "fork"
	default:
		return "session"
	}
}

func normalizeList(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			item := strings.TrimSpace(part)
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}

func firstNonEmptyLine(text string) string {
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			return line
		}
	}
	return ""
}

func reverse[T any](items []T) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}
