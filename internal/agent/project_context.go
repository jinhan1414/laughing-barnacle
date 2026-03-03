package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	memory, ok := a.memory.(projectIndexMemory)
	if !ok {
		return nil
	}
	rootDir := a.readProjectRootDir()
	if strings.TrimSpace(rootDir) == "" {
		return nil
	}
	return memory.ListProjectIndexLines(limit, rootDir)
}

func (a *Agent) prepareCodexLocalMetadata(metadata map[string]any) (map[string]any, error) {
	out := cloneAnyMap(metadata)
	if text := strings.TrimSpace(readMapString(out, "working_dir")); text != "" {
		return out, nil
	}
	projectID := strings.TrimSpace(readMapString(out, "project_id"))
	if projectID == "" {
		return nil, fmt.Errorf("codex-local requires metadata.working_dir or metadata.project_id")
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
		return nil, fmt.Errorf("project not found: %s", projectID)
	}
	out["working_dir"] = workingDir
	return out, nil
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
