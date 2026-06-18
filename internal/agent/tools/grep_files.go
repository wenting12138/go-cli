package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go-cli/internal/agent/react"
)

type GrepFiles struct{}

func (t GrepFiles) Name() string {
	return "grep_files"
}

func (t GrepFiles) Description() string {
	return "Searches text files in the current workspace and returns matching lines with file paths and line numbers."
}

func (t GrepFiles) Parameters() react.ParameterSchema {
	return react.ParameterSchema{
		Type:     "object",
		Required: []string{"pattern"},
		Properties: map[string]react.ParameterSchema{
			"pattern": {
				Type:        "string",
				Description: "Regular expression pattern to search for.",
			},
			"path": {
				Type:        "string",
				Description: "Optional relative or absolute directory/file path inside the current workspace to limit the search scope.",
			},
			"file_glob": {
				Type:        "string",
				Description: "Optional file glob such as *.go or internal/*.go.",
			},
			"case_sensitive": {
				Type:        "boolean",
				Description: "Whether the search should be case-sensitive. Defaults to false.",
			},
			"max_results": {
				Type:        "integer",
				Description: "Maximum number of matching lines to return. Defaults to 50 and is capped at 200.",
			},
		},
		AdditionalProperties: false,
	}
}

func (t GrepFiles) ValidateArgs(args map[string]any) error {
	if err := t.Parameters().Validate(args); err != nil {
		return err
	}

	pattern, _ := args["pattern"].(string)
	if strings.TrimSpace(pattern) == "" {
		return fmt.Errorf("pattern must not be empty")
	}

	caseSensitive, _ := args["case_sensitive"].(bool)
	if _, err := compilePattern(pattern, caseSensitive); err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}

	maxResults := intArg(args, "max_results", 50)
	if maxResults < 1 {
		return fmt.Errorf("max_results must be at least 1")
	}
	if maxResults > 200 {
		return fmt.Errorf("max_results must be at most 200")
	}

	if path, _ := args["path"].(string); strings.TrimSpace(path) != "" {
		if _, err := os.Getwd(); err == nil {
			if _, err := resolveWorkspacePath(mustGetwd(), path); err != nil {
				return err
			}
		}
	}

	return nil
}

func (t GrepFiles) Run(ctx context.Context, args map[string]any) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	workspaceRoot, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}

	pattern, _ := args["pattern"].(string)
	caseSensitive, _ := args["case_sensitive"].(bool)
	re, err := compilePattern(pattern, caseSensitive)
	if err != nil {
		return "", fmt.Errorf("invalid pattern: %w", err)
	}

	searchRoot := workspaceRoot
	if path, _ := args["path"].(string); strings.TrimSpace(path) != "" {
		searchRoot, err = resolveWorkspacePath(workspaceRoot, path)
		if err != nil {
			return "", err
		}
	}

	fileGlob, _ := args["file_glob"].(string)
	maxResults := intArg(args, "max_results", 50)

	matches, err := grepPath(ctx, workspaceRoot, searchRoot, fileGlob, re, maxResults)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "No matches found.", nil
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("pattern: %s", pattern))
	lines = append(lines, fmt.Sprintf("results: %d", len(matches)))
	for _, match := range matches {
		lines = append(lines, fmt.Sprintf("%s:%d: %s", match.Path, match.Line, match.Text))
	}
	if len(matches) == maxResults {
		lines = append(lines, fmt.Sprintf("truncated: reached max_results=%d", maxResults))
	}
	return strings.Join(lines, "\n"), nil
}

type grepMatch struct {
	Path string
	Line int
	Text string
}

func grepPath(ctx context.Context, workspaceRoot, searchRoot, fileGlob string, re *regexp.Regexp, maxResults int) ([]grepMatch, error) {
	info, err := os.Stat(searchRoot)
	if err != nil {
		return nil, fmt.Errorf("stat path: %w", err)
	}

	var matches []grepMatch
	if !info.IsDir() {
		fileMatches, err := grepFile(workspaceRoot, searchRoot, re, maxResults)
		if err != nil {
			return nil, err
		}
		return fileMatches, nil
	}

	err = filepath.WalkDir(searchRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".go-cli" {
				return filepath.SkipDir
			}
			return nil
		}

		if fileGlob != "" {
			rel, err := filepath.Rel(workspaceRoot, path)
			if err != nil {
				return err
			}
			ok, err := matchGlob(fileGlob, rel)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
		}

		fileMatches, err := grepFile(workspaceRoot, path, re, maxResults-len(matches))
		if err != nil {
			return nil
		}
		matches = append(matches, fileMatches...)
		if len(matches) >= maxResults {
			return errMaxResultsReached
		}
		return nil
	})
	if err != nil && err != errMaxResultsReached {
		return nil, fmt.Errorf("walk files: %w", err)
	}
	if len(matches) > maxResults {
		matches = matches[:maxResults]
	}
	return matches, nil
}

var errMaxResultsReached = fmt.Errorf("max results reached")

func grepFile(workspaceRoot, path string, re *regexp.Regexp, maxResults int) ([]grepMatch, error) {
	if maxResults <= 0 {
		return nil, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	rel, err := filepath.Rel(workspaceRoot, path)
	if err != nil {
		rel = path
	}
	rel = filepath.ToSlash(rel)

	var matches []grepMatch
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if !utf8Valid(line) {
			return nil, nil
		}
		if re.MatchString(line) {
			matches = append(matches, grepMatch{
				Path: rel,
				Line: lineNo,
				Text: line,
			})
			if len(matches) >= maxResults {
				break
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil
	}
	return matches, nil
}

func compilePattern(pattern string, caseSensitive bool) (*regexp.Regexp, error) {
	if !caseSensitive {
		pattern = "(?i)" + pattern
	}
	return regexp.Compile(pattern)
}

func matchGlob(glob, rel string) (bool, error) {
	rel = filepath.ToSlash(rel)
	glob = filepath.ToSlash(strings.TrimSpace(glob))
	if strings.Contains(glob, "/") {
		return filepath.Match(glob, rel)
	}
	base := filepath.Base(rel)
	return filepath.Match(glob, base)
}

func utf8Valid(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] == 0 {
			return false
		}
	}
	return true
}

func mustGetwd() string {
	wd, _ := os.Getwd()
	return wd
}
