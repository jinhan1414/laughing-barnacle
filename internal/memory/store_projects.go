package memory

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

const projectOverviewSchemaKind = "project_overview"

func (s *Store) ListProjectIndexLines(limit int, rootDir string) []string {
	rootDir = strings.TrimSpace(rootDir)
	if rootDir == "" {
		return nil
	}
	items := s.listProjectRecords()
	out := make([]string, 0, len(items))
	for _, item := range items {
		line := fmt.Sprintf(
			"project_id=%s | working_dir=%s | relative_path=%s | aliases=%s | summary=%s",
			item.ProjectID,
			filepath.Join(rootDir, filepath.FromSlash(item.RelativePath)),
			item.RelativePath,
			strings.Join(item.Aliases, ","),
			trimRunes(item.Summary, 36),
		)
		out = append(out, line)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func (s *Store) ResolveProjectWorkingDir(rootDir, projectID string) (string, bool, error) {
	rootDir = strings.TrimSpace(rootDir)
	projectID = strings.TrimSpace(strings.ToLower(projectID))
	if rootDir == "" || projectID == "" {
		return "", false, nil
	}
	for _, item := range s.listProjectRecords() {
		if item.ProjectID != projectID {
			continue
		}
		return filepath.Join(rootDir, filepath.FromSlash(item.RelativePath)), true, nil
	}
	return "", false, nil
}

func (s *Store) UpsertProject(projectID, relativePath string, aliases []string, summary string) error {
	projectID = normalizeProjectID(projectID)
	relativePath = normalizeStoredRelativePath(relativePath)
	if projectID == "" {
		return fmt.Errorf("project id is required")
	}
	if relativePath == "" {
		return fmt.Errorf("relative path is required")
	}
	path := "/projects/" + projectID + "/overview"
	refs := []Ref{
		{Kind: "project_id", Value: projectID},
		{Kind: "relative_path", Value: relativePath},
	}
	for _, alias := range normalizeProjectAliases(aliases) {
		refs = append(refs, Ref{Kind: "alias", Value: alias})
	}
	if _, err := s.UpsertNode(UpsertRequest{
		Mode:          "replace",
		Path:          path,
		Type:          NodeTypeFile,
		Title:         "项目概览",
		SchemaKind:    projectOverviewSchemaKind,
		SchemaVersion: 1,
		Source:        "system",
		Confidence:    1,
		Summary:       trimRunes(summary, 200),
		Refs:          refs,
	}); err != nil {
		return err
	}
	return nil
}

type projectRecord struct {
	ProjectID    string
	RelativePath string
	Aliases      []string
	Summary      string
}

func (s *Store) listProjectRecords() []projectRecord {
	nodes := s.ListNodes(0)
	out := make([]projectRecord, 0, len(nodes))
	for _, node := range nodes {
		record, ok := projectRecordFromNode(node)
		if !ok {
			continue
		}
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ProjectID < out[j].ProjectID
	})
	return out
}

func projectRecordFromNode(node Node) (projectRecord, bool) {
	if node.Type != NodeTypeFile || node.Content == nil {
		return projectRecord{}, false
	}
	if node.SchemaKind != projectOverviewSchemaKind {
		return projectRecord{}, false
	}
	projectID := normalizeProjectID(refValue(node.Content.Refs, "project_id"))
	if projectID == "" {
		projectID = normalizeProjectID(pathProjectID(node.Path))
	}
	relativePath := normalizeStoredRelativePath(refValue(node.Content.Refs, "relative_path"))
	if projectID == "" || relativePath == "" {
		return projectRecord{}, false
	}
	return projectRecord{
		ProjectID:    projectID,
		RelativePath: relativePath,
		Aliases:      collectProjectAliases(node.Content.Refs),
		Summary:      strings.TrimSpace(node.Content.Summary),
	}, true
}

func pathProjectID(path string) string {
	path = strings.TrimSpace(strings.ToLower(path))
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 || parts[0] != "projects" {
		return ""
	}
	return parts[1]
}

func normalizeProjectID(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.ReplaceAll(raw, "/", "-")
	raw = strings.ReplaceAll(raw, "\\", "-")
	raw = strings.ReplaceAll(raw, " ", "-")
	if !pathSegPattern.MatchString(raw) {
		return ""
	}
	return raw
}

func normalizeStoredRelativePath(raw string) string {
	parts := strings.FieldsFunc(strings.TrimSpace(raw), func(r rune) bool {
		return r == '/' || r == '\\'
	})
	if len(parts) == 0 {
		return ""
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.ToLower(part))
		if part == "" {
			continue
		}
		if !pathSegPattern.MatchString(part) {
			return ""
		}
		out = append(out, part)
	}
	return strings.Join(out, "/")
}

func normalizeProjectAliases(in []string) []string {
	return normalizeStringList(in, 8, 60)
}

func collectProjectAliases(refs []Ref) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if strings.EqualFold(strings.TrimSpace(ref.Kind), "alias") {
			out = append(out, strings.TrimSpace(ref.Value))
		}
	}
	return normalizeProjectAliases(out)
}
