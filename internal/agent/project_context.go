package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const codexLocalAgentID = "codex-local"

var projectSegmentPattern = regexp.MustCompile(`[^a-z0-9._-]+`)

type projectIndexMemory interface {
	ListProjectIndexLines(limit int, rootDir string) []string
	ResolveProjectWorkingDir(rootDir, projectID string) (string, bool, error)
	UpsertProject(projectID, relativePath string, aliases []string, summary string) error
}

func (a *Agent) listProjectIndexLines(limit int) []string {
	rootDir := a.readProjectRootDir()
	if strings.TrimSpace(rootDir) == "" {
		return nil
	}
	memory, ok := a.memory.(projectIndexMemory)
	if ok {
		lines := memory.ListProjectIndexLines(limit, rootDir)
		if len(lines) > 0 {
			return lines
		}
	}
	return scanProjectRootIndexLines(rootDir, limit)
}

func (a *Agent) prepareCodexLocalMetadata(metadata map[string]any) (map[string]any, error) {
	out := cloneAnyMap(metadata)
	if out == nil {
		out = make(map[string]any, 2)
	}
	if text := strings.TrimSpace(readMapString(out, "working_dir")); text != "" {
		normalized, err := a.normalizeCodexLocalWorkingDir(text)
		if err != nil {
			return nil, err
		}
		out["working_dir"] = normalized
		return out, nil
	}
	projectID := strings.TrimSpace(readMapString(out, "project_id"))
	if projectID == "" {
		fallbackDir, err := a.deriveFallbackWorkingDir()
		if err != nil {
			return nil, err
		}
		out["working_dir"] = fallbackDir
		return out, nil
	}
	rootDir, err := a.requireProjectRootDir()
	if err != nil {
		return nil, err
	}
	memory, ok := a.memory.(projectIndexMemory)
	if !ok {
		return nil, fmt.Errorf("project registry is unavailable")
	}
	if readMapBool(out, "create_project") {
		workingDir, relativePath, createErr := createProjectWorkingDir(rootDir, projectID, readMapString(out, "relative_path"))
		if createErr != nil {
			return nil, createErr
		}
		if err := memory.UpsertProject(projectID, relativePath, readMapStringList(out, "aliases"), readMapString(out, "project_summary")); err != nil {
			return nil, err
		}
		out["relative_path"] = relativePath
		out["working_dir"] = workingDir
		return out, nil
	}
	workingDir, found, err := memory.ResolveProjectWorkingDir(rootDir, projectID)
	if err != nil {
		return nil, err
	}
	if !found {
		candidateDir := filepath.Join(rootDir, filepath.FromSlash(normalizeProjectRelativePath(projectID)))
		info, statErr := os.Stat(candidateDir)
		if statErr == nil && info.IsDir() {
			out["working_dir"] = candidateDir
			return out, nil
		}
		return nil, fmt.Errorf("project not found: %s", projectID)
	}
	out["working_dir"] = workingDir
	return out, nil
}

func (a *Agent) normalizeCodexLocalWorkingDir(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "", fmt.Errorf("metadata.working_dir must not be empty")
	}
	rootDir := a.readProjectRootDir()
	if rootDir == "" {
		if filepath.IsAbs(text) {
			return text, nil
		}
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working_dir from cwd: %w", err)
		}
		return filepath.Join(cwd, filepath.FromSlash(text)), nil
	}
	rootAbs, err := a.requireProjectRootDir()
	if err != nil {
		return "", err
	}
	candidate := text
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(rootAbs, filepath.FromSlash(candidate))
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve metadata.working_dir: %w", err)
	}
	if !isPathWithinRoot(rootAbs, candidateAbs) {
		return "", fmt.Errorf("metadata.working_dir must be under APP_PROJECT_ROOT_DIR: %s", rootAbs)
	}
	return candidateAbs, nil
}

func (a *Agent) deriveFallbackWorkingDir() (string, error) {
	rootDir, err := a.requireProjectRootDir()
	if err != nil {
		return "", err
	}
	cwd, cwdErr := os.Getwd()
	if cwdErr == nil {
		cwdAbs, absErr := filepath.Abs(cwd)
		if absErr == nil {
			rel, relErr := filepath.Rel(rootDir, cwdAbs)
			if relErr == nil && (rel == "." || !strings.HasPrefix(rel, "..")) {
				return cwdAbs, nil
			}
		}
	}
	return rootDir, nil
}

func (a *Agent) requireProjectRootDir() (string, error) {
	rootDir := a.readProjectRootDir()
	if strings.TrimSpace(rootDir) == "" {
		return "", fmt.Errorf("project root directory is not configured")
	}
	info, err := os.Stat(rootDir)
	if err != nil {
		return "", fmt.Errorf("project root directory is invalid: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project root directory is not a directory: %s", rootDir)
	}
	return rootDir, nil
}

func (a *Agent) readProjectRootDir() string {
	if a == nil || a.projectCfg == nil {
		return ""
	}
	return strings.TrimSpace(a.projectCfg.GetProjectRootDir())
}

func createProjectWorkingDir(rootDir, projectID, rawRelativePath string) (string, string, error) {
	relativePath := normalizeProjectRelativePath(rawRelativePath)
	if relativePath == "" {
		relativePath = normalizeProjectRelativePath(projectID)
	}
	if relativePath == "" {
		return "", "", fmt.Errorf("project_id or metadata.relative_path must produce a valid path")
	}
	workingDir := filepath.Join(rootDir, filepath.FromSlash(relativePath))
	if _, err := os.Stat(workingDir); err == nil {
		return "", "", fmt.Errorf("project working directory already exists: %s", workingDir)
	} else if !os.IsNotExist(err) {
		return "", "", fmt.Errorf("stat project working directory: %w", err)
	}
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create project working directory: %w", err)
	}
	return workingDir, relativePath, nil
}

func normalizeProjectRelativePath(raw string) string {
	parts := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(raw)), func(r rune) bool {
		return r == '/' || r == '\\'
	})
	if len(parts) == 0 {
		return ""
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, ". ")
		part = projectSegmentPattern.ReplaceAllString(part, "-")
		part = strings.Trim(part, "-")
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return strings.Join(out, "/")
}

func scanProjectRootIndexLines(rootDir string, limit int) []string {
	entries, err := os.ReadDir(rootDir)
	if err != nil || len(entries) == 0 {
		return nil
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectID := normalizeProjectIDFromName(entry.Name())
		if projectID == "" {
			continue
		}
		relativePath := normalizeProjectRelativePath(entry.Name())
		if relativePath == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf(
			"project_id=%s | working_dir=%s | relative_path=%s | aliases= | summary=%s",
			projectID,
			filepath.Join(rootDir, filepath.FromSlash(relativePath)),
			relativePath,
			"root-scan",
		))
	}
	sort.Strings(lines)
	if limit > 0 && len(lines) > limit {
		lines = lines[:limit]
	}
	return lines
}

func normalizeProjectIDFromName(raw string) string {
	normalizedPath := normalizeProjectRelativePath(raw)
	if normalizedPath == "" {
		return ""
	}
	return strings.ReplaceAll(normalizedPath, "/", "-")
}

func isPathWithinRoot(rootDir, candidate string) bool {
	rel, err := filepath.Rel(rootDir, candidate)
	if err != nil {
		return false
	}
	rel = filepath.Clean(rel)
	if rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func readMapString(in map[string]any, key string) string {
	if len(in) == 0 {
		return ""
	}
	value, _ := in[key].(string)
	return strings.TrimSpace(value)
}

func readMapBool(in map[string]any, key string) bool {
	if len(in) == 0 {
		return false
	}
	value, _ := in[key].(bool)
	return value
}

func readMapStringList(in map[string]any, key string) []string {
	if len(in) == 0 {
		return nil
	}
	raw, ok := in[key].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, _ := item.(string)
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
