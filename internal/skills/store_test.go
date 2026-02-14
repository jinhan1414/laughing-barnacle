package skills

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
	if len(skills) < 3 {
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
	if len(skills) < 3 {
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
	if len(all) < 2 {
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
