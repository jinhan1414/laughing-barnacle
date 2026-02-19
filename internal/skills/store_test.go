package skills

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestBuiltinSkillsConformSkillSpec(t *testing.T) {
	for _, builtin := range builtinSkills {
		id := strings.TrimSpace(builtin.ID)
		if id == "" {
			t.Fatalf("builtin skill id should not be empty")
		}
		if got := sanitizeIdentifier(id); got != id {
			t.Fatalf("builtin skill id should be lowercase/hyphen form: id=%q sanitized=%q", id, got)
		}

		md := renderSkillMarkdown(builtin)
		if !strings.HasPrefix(md, "---\n") {
			t.Fatalf("builtin skill %q should use SKILL.md frontmatter", id)
		}
		name, description, body := parseSkillMarkdown(md)
		if strings.TrimSpace(name) == "" {
			t.Fatalf("builtin skill %q name missing", id)
		}
		if strings.TrimSpace(description) == "" {
			t.Fatalf("builtin skill %q description missing", id)
		}
		if strings.TrimSpace(body) == "" {
			t.Fatalf("builtin skill %q body missing", id)
		}
	}
}

func TestBuiltinScheduledExecutionSkillsPresent(t *testing.T) {
	foundNight := false
	foundMorning := false
	for _, builtin := range builtinSkills {
		switch strings.TrimSpace(builtin.ID) {
		case "night-reflection-evolution":
			foundNight = true
			if !strings.Contains(strings.ToLower(builtin.Prompt), "json") {
				t.Fatalf("night scheduled skill should require json output")
			}
			if !strings.Contains(builtin.Prompt, "不得包含“数字分身长期目标”") {
				t.Fatalf("night scheduled skill should block deprecated long-term-goal sections")
			}
			if !strings.Contains(builtin.Prompt, "内置技能") {
				t.Fatalf("night scheduled skill should avoid duplicating built-in skill responsibilities")
			}
		case "morning-planning":
			foundMorning = true
			if !strings.Contains(strings.ToLower(builtin.Prompt), "json") {
				t.Fatalf("morning scheduled skill should require json output")
			}
		}
	}
	if !foundNight || !foundMorning {
		t.Fatalf("expected builtin scheduled execution skills, found night=%v morning=%v", foundNight, foundMorning)
	}
}

func TestBuiltinScheduleConfigMaintainerRequiresSkillValidation(t *testing.T) {
	found := false
	for _, builtin := range builtinSkills {
		if strings.TrimSpace(builtin.ID) != "schedule-config-maintainer" {
			continue
		}
		found = true
		prompt := strings.ToLower(strings.TrimSpace(builtin.Prompt))
		if !strings.Contains(prompt, "/api/skills") {
			t.Fatalf("schedule-config-maintainer should check /api/skills before save")
		}
		if !strings.Contains(prompt, "/settings/skills/save") {
			t.Fatalf("schedule-config-maintainer should support creating missing task skill")
		}
		if !strings.Contains(prompt, "/settings/skills/toggle") {
			t.Fatalf("schedule-config-maintainer should support enabling existing disabled task skill")
		}
		if !strings.Contains(prompt, "禁止写入") || !strings.Contains(prompt, "未启用 skill") {
			t.Fatalf("schedule-config-maintainer should forbid invalid skill action")
		}
	}
	if !found {
		t.Fatalf("expected builtin schedule-config-maintainer skill")
	}
}

func TestStoreUpsertAndReload(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertSkill(Skill{
		Name:        "Research Mode",
		Description: "用于先检索后回答",
		Prompt:      "先检索，再总结，最后给出处。",
		Enabled:     true,
	}); err != nil {
		t.Fatalf("UpsertSkill error: %v", err)
	}

	reloaded, err := NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("reload NewStore error: %v", err)
	}

	skills := reloaded.ListSkills()
	if len(skills) < 4 {
		t.Fatalf("expected builtin + custom skills, got %d", len(skills))
	}
	var custom Skill
	found := false
	for _, item := range skills {
		if strings.EqualFold(item.Name, "Research Mode") {
			custom = item
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected custom skill Research Mode")
	}
	if !custom.Enabled {
		t.Fatalf("expected enabled skill")
	}
	if strings.TrimSpace(custom.Prompt) == "" {
		t.Fatalf("expected prompt")
	}
	prompt, ok := reloaded.ReadEnabledSkillPrompt(custom.ID)
	if !ok || !strings.Contains(prompt, "description:") {
		t.Fatalf("expected readable SKILL.md, got ok=%v, prompt=%q", ok, prompt)
	}
}

func TestStoreFolderDiscovery_DefaultEnabled(t *testing.T) {
	root := t.TempDir()
	skillsDir := filepath.Join(root, "skills")
	if err := os.MkdirAll(filepath.Join(skillsDir, "demo"), 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	content := "---\nname: \"demo\"\ndescription: \"demo skill\"\n---\n\nrun demo"
	if err := os.WriteFile(filepath.Join(skillsDir, "demo", "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("write SKILL.md error: %v", err)
	}

	store, err := NewStore(skillsDir, filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	skills := store.ListSkills()
	if len(skills) < 4 {
		t.Fatalf("expected builtin + discovered skills, got %d", len(skills))
	}
	foundDemo := false
	for _, item := range skills {
		if item.ID != "demo" {
			continue
		}
		foundDemo = true
		if !item.Enabled {
			t.Fatalf("expected default enabled=true when no state override")
		}
	}
	if !foundDemo {
		t.Fatalf("expected discovered skill demo")
	}
}

func TestInstallFromSkillsSH_InvalidURL(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if _, err := store.InstallFromSkillsSH(context.Background(), "https://example.com/foo/bar/baz"); err == nil {
		t.Fatalf("expected host validation error")
	}
}

func TestInstallFromRepo_LocalGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "skills", "demo-skill"), 0o755); err != nil {
		t.Fatalf("mkdir repo skill dir error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "skills", "demo-skill", "SKILL.md"), []byte("---\nname: \"demo\"\ndescription: \"demo\"\n---\n\nbody"), 0o600); err != nil {
		t.Fatalf("write repo skill file error: %v", err)
	}

	runGit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, strings.TrimSpace(string(out)))
		}
	}
	runGit("init")
	runGit("add", ".")
	runGit("commit", "-m", "init")

	store, err := NewStore(filepath.Join(root, "skills-home"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	installed, err := store.installFromRepo(context.Background(), repo, "demo-skill", "https://skills.sh/demo/repo/demo-skill")
	if err != nil {
		t.Fatalf("installFromRepo error: %v", err)
	}
	if installed.ID != "demo-skill" {
		t.Fatalf("unexpected installed id: %q", installed.ID)
	}
	if !installed.Enabled {
		t.Fatalf("expected installed skill enabled")
	}
	if !strings.Contains(installed.Source, "skills.sh") {
		t.Fatalf("unexpected installed source: %q", installed.Source)
	}
}

func TestStoreHasBuiltinConfigSkills(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	all := store.ListSkills()
	if len(all) < 3 {
		t.Fatalf("expected builtin skills to exist, got %d", len(all))
	}

	findByID := func(id string) (Skill, bool) {
		for _, item := range all {
			if item.ID == id {
				return item, true
			}
		}
		return Skill{}, false
	}

	mcpSkill, ok := findByID("mcp-config-maintainer")
	if !ok {
		t.Fatalf("builtin skill mcp-config-maintainer missing")
	}
	if !mcpSkill.Enabled {
		t.Fatalf("builtin mcp skill should be enabled by default")
	}
	if mcpSkill.Source != "builtin" {
		t.Fatalf("builtin mcp skill source mismatch: %q", mcpSkill.Source)
	}
	if !strings.Contains(mcpSkill.Prompt, "/api/mcp/services") {
		t.Fatalf("builtin mcp skill prompt mismatch: %q", mcpSkill.Prompt)
	}
	if !strings.Contains(mcpSkill.Prompt, "未确认不得写入") {
		t.Fatalf("builtin mcp skill should require user confirmation, got: %q", mcpSkill.Prompt)
	}

	skillsSkill, ok := findByID("skills-config-maintainer")
	if !ok {
		t.Fatalf("builtin skill skills-config-maintainer missing")
	}
	if !skillsSkill.Enabled {
		t.Fatalf("builtin skills skill should be enabled by default")
	}
	if skillsSkill.Source != "builtin" {
		t.Fatalf("builtin skills skill source mismatch: %q", skillsSkill.Source)
	}
	if !strings.Contains(skillsSkill.Prompt, "/api/skills") {
		t.Fatalf("builtin skills skill prompt mismatch: %q", skillsSkill.Prompt)
	}
	if !strings.Contains(skillsSkill.Prompt, "/api/skills/catalog/search") {
		t.Fatalf("builtin skills skill should include catalog search endpoint, got: %q", skillsSkill.Prompt)
	}
	if !strings.Contains(skillsSkill.Prompt, "未确认不得执行安装或删除") {
		t.Fatalf("builtin skills skill should require user confirmation, got: %q", skillsSkill.Prompt)
	}

	scheduleSkill, ok := findByID("schedule-config-maintainer")
	if !ok {
		t.Fatalf("builtin skill schedule-config-maintainer missing")
	}
	if !scheduleSkill.Enabled {
		t.Fatalf("builtin schedule skill should be enabled by default")
	}
	if scheduleSkill.Source != "builtin" {
		t.Fatalf("builtin schedule skill source mismatch: %q", scheduleSkill.Source)
	}
	if !strings.Contains(scheduleSkill.Prompt, "/api/schedules") {
		t.Fatalf("builtin schedule skill prompt mismatch: %q", scheduleSkill.Prompt)
	}
	if !strings.Contains(scheduleSkill.Prompt, "/settings/schedules/save") {
		t.Fatalf("builtin schedule skill should include save endpoint, got: %q", scheduleSkill.Prompt)
	}
	if !strings.Contains(scheduleSkill.Prompt, "/settings/schedules/run") {
		t.Fatalf("builtin schedule skill should include run endpoint, got: %q", scheduleSkill.Prompt)
	}
	if !strings.Contains(scheduleSkill.Prompt, "未确认不得写入") {
		t.Fatalf("builtin schedule skill should require user confirmation, got: %q", scheduleSkill.Prompt)
	}

	archiveSkill, ok := findByID("context-archive-recall")
	if !ok {
		t.Fatalf("builtin skill context-archive-recall missing")
	}
	if !archiveSkill.Enabled {
		t.Fatalf("builtin archive recall skill should be enabled by default")
	}
	if archiveSkill.Source != "builtin" {
		t.Fatalf("builtin archive recall skill source mismatch: %q", archiveSkill.Source)
	}
	if !strings.Contains(archiveSkill.Prompt, "/api/context/archive/index") {
		t.Fatalf("builtin archive recall skill should include archive index endpoint, got: %q", archiveSkill.Prompt)
	}
	if !strings.Contains(archiveSkill.Prompt, "/api/context/archive/section") {
		t.Fatalf("builtin archive recall skill should include archive section endpoint, got: %q", archiveSkill.Prompt)
	}
	if !strings.Contains(archiveSkill.Prompt, "禁止一次性拉取全部归档正文") {
		t.Fatalf("builtin archive recall skill should enforce minimal retrieval, got: %q", archiveSkill.Prompt)
	}

	projectSkill, ok := findByID("project-memory-maintainer")
	if !ok {
		t.Fatalf("builtin skill project-memory-maintainer missing")
	}
	if !projectSkill.Enabled {
		t.Fatalf("builtin project maintainer skill should be enabled by default")
	}
	if projectSkill.Source != "builtin" {
		t.Fatalf("builtin project maintainer skill source mismatch: %q", projectSkill.Source)
	}
	if !strings.Contains(projectSkill.Prompt, "/api/projects") {
		t.Fatalf("builtin project maintainer should include /api/projects endpoint, got: %q", projectSkill.Prompt)
	}
	if !strings.Contains(projectSkill.Prompt, "/api/projects/upsert") {
		t.Fatalf("builtin project maintainer should include upsert endpoint, got: %q", projectSkill.Prompt)
	}
	if !strings.Contains(projectSkill.Prompt, "信息不确定时禁止写入") {
		t.Fatalf("builtin project maintainer should forbid uncertain writes, got: %q", projectSkill.Prompt)
	}
}

func TestListEnabledSkillIndex_MinimalProgressiveDisclosureFields(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if err := store.UpsertSkill(Skill{
		Name:        "日志诊断",
		Description: "用于查看与分析日志",
		Prompt:      "先查日志，再定位根因，再给修复路径。",
		Enabled:     true,
	}); err != nil {
		t.Fatalf("UpsertSkill error: %v", err)
	}

	index := store.ListEnabledSkillIndex()
	if len(index) == 0 {
		t.Fatalf("expected non-empty skill index")
	}
	found := false
	for _, line := range index {
		if !strings.Contains(line, "skill_id=") || !strings.Contains(line, "name=") || !strings.Contains(line, "description=") {
			t.Fatalf("index line should contain skill_id/name/description: %q", line)
		}
		if strings.Contains(line, "brief=") {
			t.Fatalf("index line should not include prompt brief: %q", line)
		}
		if strings.Contains(line, "path=") {
			t.Fatalf("index line should not include path for token efficiency: %q", line)
		}
		if strings.Contains(line, "日志诊断") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected custom skill in index")
	}
}

func TestSearchSkillsCatalog(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" {
			http.NotFound(w, r)
			return
		}
		if got := strings.TrimSpace(r.URL.Query().Get("q")); got != "react" {
			http.Error(w, "bad query", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"query":"react",
			"skills":[
				{"id":"vercel-labs/agent-skills/vercel-react-best-practices","source":"vercel-labs/agent-skills","skillId":"vercel-react-best-practices","name":"vercel-react-best-practices","installs":123},
				{"id":"anthropics/skills/frontend-design","source":"anthropics/skills","skillId":"frontend-design","name":"frontend-design","installs":99}
			]
		}`))
	}))
	defer mockServer.Close()

	prev := skillsSHSearchEndpoint
	skillsSHSearchEndpoint = mockServer.URL + "/api/search"
	defer func() { skillsSHSearchEndpoint = prev }()

	root := t.TempDir()
	store, err := NewStore(filepath.Join(root, "skills"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	items, err := store.SearchSkillsCatalog(context.Background(), "react", 1)
	if err != nil {
		t.Fatalf("SearchSkillsCatalog error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 result by limit, got %d", len(items))
	}
	if items[0].Source != "vercel-labs/agent-skills" {
		t.Fatalf("unexpected source: %q", items[0].Source)
	}
	if items[0].SkillID != "vercel-react-best-practices" {
		t.Fatalf("unexpected skill id: %q", items[0].SkillID)
	}
	if items[0].URL != "https://skills.sh/vercel-labs/agent-skills/vercel-react-best-practices" {
		t.Fatalf("unexpected skill url: %q", items[0].URL)
	}
}

func TestInstallFromRepo_FallbackToGitHubArchive(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(filepath.Join(root, "skills-home"), filepath.Join(root, "skills_state.json"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	archiveBody := buildTestTarGz(t, "skills-main-abcdef", map[string]string{
		"skills/demo-skill/SKILL.md": "---\nname: \"demo-skill\"\ndescription: \"demo\"\n---\n\nbody",
	})
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/acme/skills/tar.gz/refs/heads/main" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(archiveBody)
	}))
	defer mockServer.Close()

	prevGitCmd := gitCommandContext
	gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "false")
	}
	defer func() { gitCommandContext = prevGitCmd }()

	prevArchiveTemplate := githubArchiveEndpointTemplate
	githubArchiveEndpointTemplate = mockServer.URL + "/%s/%s/tar.gz/refs/heads/%s"
	defer func() { githubArchiveEndpointTemplate = prevArchiveTemplate }()

	installed, err := store.installFromRepo(
		context.Background(),
		"https://github.com/acme/skills.git",
		"demo-skill",
		"https://skills.sh/acme/skills/demo-skill",
	)
	if err != nil {
		t.Fatalf("installFromRepo with archive fallback error: %v", err)
	}
	if installed.ID != "demo-skill" {
		t.Fatalf("unexpected installed id: %q", installed.ID)
	}
	if !installed.Enabled {
		t.Fatalf("expected installed skill enabled")
	}
	skillPath := filepath.Join(root, "skills-home", "demo-skill", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("expected installed SKILL.md at %s: %v", skillPath, err)
	}
}

func TestParseGitHubOwnerRepo(t *testing.T) {
	cases := []struct {
		name      string
		repoURL   string
		wantOwner string
		wantRepo  string
		wantOK    bool
	}{
		{
			name:      "https with git suffix",
			repoURL:   "https://github.com/acme/skills.git",
			wantOwner: "acme",
			wantRepo:  "skills",
			wantOK:    true,
		},
		{
			name:      "https without git suffix",
			repoURL:   "https://github.com/acme/skills",
			wantOwner: "acme",
			wantRepo:  "skills",
			wantOK:    true,
		},
		{
			name:    "non github host",
			repoURL: "https://example.com/acme/skills.git",
			wantOK:  false,
		},
		{
			name:    "invalid path",
			repoURL: "https://github.com/acme",
			wantOK:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			owner, repo, ok := parseGitHubOwnerRepo(tc.repoURL)
			if ok != tc.wantOK {
				t.Fatalf("unexpected ok: got=%v want=%v", ok, tc.wantOK)
			}
			if owner != tc.wantOwner {
				t.Fatalf("unexpected owner: got=%q want=%q", owner, tc.wantOwner)
			}
			if repo != tc.wantRepo {
				t.Fatalf("unexpected repo: got=%q want=%q", repo, tc.wantRepo)
			}
		})
	}
}

func buildTestTarGz(t *testing.T, topDir string, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	if err := tw.WriteHeader(&tar.Header{
		Name:     strings.TrimSuffix(topDir, "/") + "/",
		Mode:     0o755,
		Typeflag: tar.TypeDir,
	}); err != nil {
		t.Fatalf("write tar root header error: %v", err)
	}

	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, rel := range paths {
		content := files[rel]
		name := strings.TrimSuffix(topDir, "/") + "/" + strings.TrimPrefix(rel, "/")
		body := []byte(content)
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o600,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatalf("write tar file header error: %v", err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatalf("write tar file body error: %v", err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer error: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip writer error: %v", err)
	}
	return buf.Bytes()
}
