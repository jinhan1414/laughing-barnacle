package skills

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	autoSkillIDPrefix       = "auto-skill-"
	maxAutoSkillsRetained   = 24
	maxAutoSkillNameRunes   = 24
	maxAutoSkillPromptRunes = 180
	builtinSkillSource      = "builtin"
)

var (
	skillsSHSearchEndpoint        = "https://skills.sh/api/search"
	githubArchiveEndpointTemplate = "https://codeload.github.com/%s/%s/tar.gz/refs/heads/%s"
	gitCommandContext             = exec.CommandContext
	gitCloneRetryDelay            = 800 * time.Millisecond
	gitCloneMaxAttempts           = 2
)

var builtinSkills = []Skill{
	{
		ID:          "mcp-config-maintainer",
		Name:        "MCP 配置维护",
		Description: "当用户要求新增/修改/删除/启停 MCP 服务时使用",
		Prompt: strings.TrimSpace(
			"先查现状：用 linux__bash 执行 curl -s http://127.0.0.1:8080/api/mcp/services。\n" +
				"任何写操作前必须先输出“变更计划”（新增/修改/删除/启停的目标与参数），并等待用户明确确认（例如：确认新增、确认修改、确认删除）。\n" +
				"新增/更新：POST /settings/mcp/save，字段 name/transport/endpoint 或 command/args_json/enabled。\n" +
				"删除：POST /settings/mcp/delete(id)；启停：POST /settings/mcp/toggle(id,enabled)。\n" +
				"每次改后再次查询 /api/mcp/services，向用户汇报新增/更新/删除 diff。规则：先查后改，未确认不得写入，stdio 必填 command，参数不确定先问。",
		),
		Enabled: true,
		Source:  builtinSkillSource,
	},
	{
		ID:          "skills-config-maintainer",
		Name:        "Skills 配置维护",
		Description: "当用户要求安装/新增/删除/启停 Skill 时使用",
		Prompt: strings.TrimSpace(
			"先查现状：用 linux__bash 执行 curl -s http://127.0.0.1:8080/api/skills。\n" +
				"先搜索候选：GET /api/skills/catalog/search?q=<需求关键词>&limit=8，做模糊匹配并给出候选技能列表。\n" +
				"先让用户选定目标 skills.sh 链接并明确确认（例如：确认安装 <url>），未确认不得执行安装或删除。\n" +
				"skills.sh 安装：POST /settings/skills/install(skills_sh_url)。\n" +
				"手动新增/更新：POST /settings/skills/save(name,description,prompt,enabled=on)。\n" +
				"启停：POST /settings/skills/toggle(id,enabled)；删除：POST /settings/skills/delete(id)。\n" +
				"每次改后再次查询 /api/skills 并汇报 diff 与最终启用状态。规则：先查后改，未确认不得写入。",
		),
		Enabled: true,
		Source:  builtinSkillSource,
	},
	{
		ID:          "night-reflection-evolution",
		Name:        "夜间复盘与进化执行",
		Description: "定时执行夜间复盘，输出复盘结论并给出提示词与能力进化建议",
		Prompt: strings.TrimSpace(
			"你是数字分身夜间复盘与进化执行技能。\n" +
				"输入会提供：当前系统提示词、当前压缩提示词、历史摘要、最近对话。\n" +
				"请完成：\n" +
				"1) 输出今日复盘 reflection：生活/工作/学习三段，每段 1-2 行。\n" +
				"2) 输出升级后的 system_prompt 与 compression_system_prompt（可在当前版本基础上做最小改写，若无需调整可原样返回）。\n" +
				"3) 提炼 0-3 条可复用技能，写入 skills 数组。\n" +
				"约束：必须保持名字“傻毛”、女性、8 年全栈开发经验、不使用表情符号、务实稳定。\n" +
				"约束：system_prompt 不得包含“数字分身长期目标”“持续提升机制”“每次交互尽量给出 1-3 条可执行改进建议”等已由内置技能承担的流程性要求。\n" +
				"约束：system_prompt 仅保留身份、沟通风格与通用回答策略；晨间规划、夜间复盘、技能维护、归档召回等流程交给内置技能。\n" +
				"输出严格 JSON 字段：reflection, system_prompt, compression_system_prompt, skills。\n" +
				"skills 每项字段：name, prompt；name 2-20 字，prompt 单行且不超过 120 字。\n" +
				"禁止输出 markdown 代码块，禁止输出额外字段。",
		),
		Enabled: true,
		Source:  builtinSkillSource,
	},
	{
		ID:          "morning-planning",
		Name:        "晨间规划执行",
		Description: "定时执行晨间任务回顾与今日 Top3 规划",
		Prompt: strings.TrimSpace(
			"你是数字分身晨间规划执行技能。\n" +
				"输入会提供：历史摘要、最近对话。\n" +
				"输出严格 JSON：{\"plan\":\"...\"}。\n" +
				"plan 必须包含三部分：\n" +
				"1) 任务进度回顾（昨天完成/未完成）；\n" +
				"2) 今日 Top 3 任务（按优先级）；\n" +
				"3) 学习与能力提升 1 条。\n" +
				"输出中文纯文本，不要 markdown 代码块，不要额外字段。",
		),
		Enabled: true,
		Source:  builtinSkillSource,
	},
	{
		ID:          "schedule-config-maintainer",
		Name:        "定时任务配置维护",
		Description: "当用户要求查看/修改 Cron 定时任务时使用",
		Prompt: strings.TrimSpace(
			"先查现状：用 linux__bash 执行 curl -s http://127.0.0.1:8080/api/schedules。\n" +
				"创建/修改前先用 linux__bash 执行 curl -s http://127.0.0.1:8080/api/skills，确认 action 中的 skill_id 存在且已启用。\n" +
				"若 skill 不存在：在用户确认后，先创建任务 skill（POST /settings/skills/save(name,description,prompt,enabled=on)），再重新查询 /api/skills 获取实际 skill_id，然后再写入定时任务 action=skill:<skill_id>。\n" +
				"若 skill 已存在但未启用：在用户确认后先启用（POST /settings/skills/toggle(id,enabled=true)），再写入定时任务。\n" +
				"禁止写入引用不存在或未启用 skill 的 action。\n" +
				"任何写操作前必须先输出“变更计划”（任务 id、action、cron、启停状态），并等待用户明确确认（例如：确认修改 morning-planning 为 0 9 * * *）。\n" +
				"新增/更新：POST /settings/schedules/save(id,name,description,action,cron_expr,enabled=on)。\n" +
				"启停：POST /settings/schedules/toggle(id,enabled)。\n" +
				"立即执行：POST /settings/schedules/run(id)。\n" +
				"action 默认使用 skill:<skill_id>（例如 skill:morning-planning）。\n" +
				"Cron 规则使用 5 段：分 时 日 月 周（例如 30 8 * * *）。\n" +
				"每次改后再次查询 /api/schedules 并汇报 diff 与最终生效配置。规则：先查后改，未确认不得写入。",
		),
		Enabled: true,
		Source:  builtinSkillSource,
	},
	{
		ID:          "context-archive-recall",
		Name:        "上下文归档召回",
		Description: "当历史摘要信息不足，需要回看被压缩原文片段时使用",
		Prompt: strings.TrimSpace(
			"仅在当前摘要无法支撑回答时触发，且必须按需最小化读取。\n" +
				"步骤 1：先通过 linux__bash 执行 curl -s \"http://127.0.0.1:8080/api/context/archive/index?archive_id=<archive_id>\" 读取归档索引（标题与分节）。\n" +
				"步骤 2：根据问题只选择必要 section_id，再执行 curl -s \"http://127.0.0.1:8080/api/context/archive/section?archive_id=<archive_id>&section_id=<section_id>\" 拉取具体分节。\n" +
				"禁止一次性拉取全部归档正文；禁止把整段历史原文回填进提示词。\n" +
				"拉取后只提炼与当前问题直接相关的事实、约束和时间点，再继续回答。",
		),
		Enabled: true,
		Source:  builtinSkillSource,
	},
}

type Skill struct {
	ID          string
	Name        string
	Description string
	Prompt      string
	Enabled     bool
	Source      string
	UpdatedAt   time.Time
}

type CatalogSkill struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	SkillID  string `json:"skill_id"`
	Name     string `json:"name"`
	Installs int    `json:"installs"`
	URL      string `json:"url"`
}

type skillStateRecord struct {
	Enabled   bool      `json:"enabled"`
	Source    string    `json:"source,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type stateFile struct {
	Skills map[string]skillStateRecord `json:"skills"`
}

type Store struct {
	dir       string
	statePath string

	mu    sync.RWMutex
	state stateFile
}

func NewStore(dir, statePath string) (*Store, error) {
	dir = strings.TrimSpace(dir)
	statePath = strings.TrimSpace(statePath)
	if dir == "" {
		return nil, fmt.Errorf("skills directory is required")
	}
	if statePath == "" {
		return nil, fmt.Errorf("skills state file path is required")
	}

	s := &Store{dir: dir, statePath: statePath}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) ListSkills() []Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()

	skills, err := s.listSkillsLocked()
	if err != nil {
		return nil
	}
	return cloneSkills(skills)
}

func (s *Store) ListEnabledSkillIndex() []string {
	skills := s.ListSkills()
	out := make([]string, 0, len(skills))
	for _, skill := range skills {
		if !skill.Enabled {
			continue
		}
		id := strings.TrimSpace(skill.ID)
		name := strings.TrimSpace(skill.Name)
		description := strings.TrimSpace(skill.Description)
		prompt := strings.TrimSpace(skill.Prompt)
		if id == "" || name == "" || prompt == "" {
			continue
		}
		out = append(out, fmt.Sprintf(
			"skill_id=%s | name=%s | description=%s",
			id,
			name,
			trimSkillText(description, 28),
		))
	}
	return out
}

func (s *Store) ReadEnabledSkillPrompt(skillID string) (string, bool) {
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return "", false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	skills, err := s.listSkillsLocked()
	if err != nil {
		return "", false
	}

	for _, skill := range skills {
		if !skill.Enabled {
			continue
		}
		if strings.TrimSpace(skill.ID) != skillID {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, skill.ID, "SKILL.md"))
		if err != nil {
			return "", false
		}
		markdown := strings.TrimSpace(string(data))
		return markdown, markdown != ""
	}

	matched := Skill{}
	found := false
	for _, skill := range skills {
		if !skill.Enabled {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(skill.Name), skillID) {
			continue
		}
		if found {
			return "", false
		}
		matched = skill
		found = true
	}
	if !found {
		return "", false
	}

	data, err := os.ReadFile(filepath.Join(s.dir, matched.ID, "SKILL.md"))
	if err != nil {
		return "", false
	}
	markdown := strings.TrimSpace(string(data))
	return markdown, markdown != ""
}

func (s *Store) UpsertSkill(skill Skill) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upsertSkillLocked(skill)
}

func (s *Store) upsertSkillLocked(skill Skill) error {
	skills, err := s.listSkillsLocked()
	if err != nil {
		return err
	}

	skill.ID = strings.TrimSpace(skill.ID)
	skill.Name = strings.TrimSpace(skill.Name)
	skill.Prompt = strings.TrimSpace(skill.Prompt)
	skill.Description = normalizeSkillDescription(skill.Description, skill.Name, skill.Prompt)

	if skill.ID == "" {
		skill.ID = findSkillIDForUpdate(skills, skill)
	}
	if skill.ID == "" {
		skill.ID = generateUniqueSkillID(skills, skill.Name, skill.Prompt)
	}
	if skill.Name == "" {
		skill.Name = skill.ID
	}
	if skill.Description == "" {
		skill.Description = normalizeSkillDescription("", skill.Name, skill.Prompt)
	}
	if strings.TrimSpace(skill.Prompt) == "" {
		return fmt.Errorf("skill prompt is required")
	}
	if err := validateSkillID(skill.ID); err != nil {
		return err
	}

	now := time.Now()
	markdown := renderSkillMarkdown(skill)
	dirPath := filepath.Join(s.dir, skill.ID)
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dirPath, "SKILL.md"), []byte(markdown+"\n"), 0o600); err != nil {
		return fmt.Errorf("write skill file: %w", err)
	}

	record := s.state.Skills[skill.ID]
	record.Enabled = skill.Enabled
	record.UpdatedAt = now
	if src := strings.TrimSpace(skill.Source); src != "" {
		record.Source = src
	}
	s.state.Skills[skill.ID] = record
	return s.persistLocked()
}

func (s *Store) DeleteSkill(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("skill id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.RemoveAll(filepath.Join(s.dir, id)); err != nil {
		return fmt.Errorf("delete skill dir: %w", err)
	}
	delete(s.state.Skills, id)
	return s.persistLocked()
}

func (s *Store) SetSkillEnabled(id string, enabled bool) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("skill id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(filepath.Join(s.dir, id, "SKILL.md")); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("skill %q not found", id)
		}
		return fmt.Errorf("read skill: %w", err)
	}

	record := s.state.Skills[id]
	record.Enabled = enabled
	record.UpdatedAt = time.Now()
	s.state.Skills[id] = record
	return s.persistLocked()
}

func (s *Store) UpsertAutoSkill(name, prompt string) error {
	name = trimSkillText(name, maxAutoSkillNameRunes)
	prompt = trimSkillText(prompt, maxAutoSkillPromptRunes)
	if name == "" {
		return fmt.Errorf("auto skill name is required")
	}
	if prompt == "" {
		return fmt.Errorf("auto skill prompt is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	skills, err := s.listSkillsLocked()
	if err != nil {
		return err
	}

	id := findAutoSkillIDByName(skills, name)
	if id == "" {
		id = generateAutoSkillID(skills, name, prompt)
	}

	if err := s.upsertSkillLocked(Skill{
		ID:          id,
		Name:        name,
		Description: normalizeSkillDescription("", name, prompt),
		Prompt:      prompt,
		Enabled:     true,
		Source:      "auto-evolved",
	}); err != nil {
		return err
	}

	s.trimAutoSkillsLocked(maxAutoSkillsRetained)
	return s.persistLocked()
}

func (s *Store) InstallFromSkillsSH(ctx context.Context, rawURL string) (Skill, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return Skill{}, fmt.Errorf("skills.sh url is required")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return Skill{}, fmt.Errorf("invalid skills.sh url: %w", err)
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Host))
	if host != "skills.sh" && host != "www.skills.sh" {
		return Skill{}, fmt.Errorf("url host must be skills.sh")
	}

	segments := splitPathSegments(parsed.Path)
	if len(segments) < 3 {
		return Skill{}, fmt.Errorf("skills.sh url must be /{owner}/{repo}/{skill}")
	}
	owner := segments[0]
	repo := segments[1]
	skillID := sanitizeIdentifier(segments[2])
	if skillID == "" {
		return Skill{}, fmt.Errorf("invalid skill id from url")
	}

	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
	return s.installFromRepo(ctx, repoURL, skillID, rawURL)
}

func (s *Store) SearchSkillsCatalog(ctx context.Context, query string, limit int) ([]CatalogSkill, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if limit <= 0 {
		limit = 8
	}
	if limit > 30 {
		limit = 30
	}

	reqURL := skillsSHSearchEndpoint + "?q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build search request: %w", err)
	}
	resp, err := (&http.Client{Timeout: 12 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("search skills.sh: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("skills.sh search failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Skills []struct {
			ID       string `json:"id"`
			Source   string `json:"source"`
			SkillID  string `json:"skillId"`
			Name     string `json:"name"`
			Installs int    `json:"installs"`
		} `json:"skills"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode skills.sh search response: %w", err)
	}

	out := make([]CatalogSkill, 0, minInt(limit, len(payload.Skills)))
	for _, item := range payload.Skills {
		source := strings.TrimSpace(item.Source)
		skillID := strings.TrimSpace(item.SkillID)
		if source == "" && strings.TrimSpace(item.ID) != "" {
			parts := splitPathSegments(item.ID)
			if len(parts) >= 3 {
				source = strings.TrimSpace(parts[0] + "/" + parts[1])
				if skillID == "" {
					skillID = strings.TrimSpace(parts[2])
				}
			}
		}
		if source == "" || skillID == "" {
			continue
		}
		out = append(out, CatalogSkill{
			ID:       strings.TrimSpace(item.ID),
			Source:   source,
			SkillID:  skillID,
			Name:     strings.TrimSpace(item.Name),
			Installs: item.Installs,
			URL:      fmt.Sprintf("https://skills.sh/%s/%s", source, skillID),
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *Store) installFromRepo(ctx context.Context, repoURL, skillID, source string) (Skill, error) {
	repoURL = strings.TrimSpace(repoURL)
	skillID = sanitizeIdentifier(skillID)
	if repoURL == "" || skillID == "" {
		return Skill{}, fmt.Errorf("repo url and skill id are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tmpRoot, err := os.MkdirTemp("", "skills-install-*")
	if err != nil {
		return Skill{}, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpRoot)

	repoPath := filepath.Join(tmpRoot, "repo")
	if err := fetchRepoSnapshot(ctx, repoURL, repoPath); err != nil {
		return Skill{}, fmt.Errorf("fetch repository snapshot failed: %w", err)
	}

	srcDir, err := findSkillDir(repoPath, skillID)
	if err != nil {
		return Skill{}, err
	}
	if _, err := os.Stat(filepath.Join(srcDir, "SKILL.md")); err != nil {
		return Skill{}, fmt.Errorf("skill file not found in repo: %w", err)
	}

	dstDir := filepath.Join(s.dir, skillID)
	if err := os.RemoveAll(dstDir); err != nil {
		return Skill{}, fmt.Errorf("clear existing skill dir: %w", err)
	}
	if err := copyDir(srcDir, dstDir); err != nil {
		return Skill{}, err
	}

	record := s.state.Skills[skillID]
	record.Enabled = true
	record.Source = strings.TrimSpace(source)
	record.UpdatedAt = time.Now()
	s.state.Skills[skillID] = record
	if err := s.persistLocked(); err != nil {
		return Skill{}, err
	}

	skills, err := s.listSkillsLocked()
	if err != nil {
		return Skill{}, err
	}
	for _, skill := range skills {
		if skill.ID == skillID {
			return skill, nil
		}
	}
	return Skill{}, fmt.Errorf("installed skill %q not found", skillID)
}

func fetchRepoSnapshot(ctx context.Context, repoURL, repoPath string) error {
	repoURL = strings.TrimSpace(repoURL)
	if repoURL == "" {
		return fmt.Errorf("repo url is required")
	}
	if repoPath == "" {
		return fmt.Errorf("repo path is required")
	}
	if err := os.RemoveAll(repoPath); err != nil {
		return fmt.Errorf("clear temp repo path: %w", err)
	}

	cloneErr := cloneRepoDepth1(ctx, repoURL, repoPath)
	if cloneErr == nil {
		return nil
	}

	owner, repo, ok := parseGitHubOwnerRepo(repoURL)
	if !ok {
		return fmt.Errorf("clone failed and repo is not a github.com url: %w", cloneErr)
	}

	var archiveErr error
	for _, branch := range []string{"main", "master"} {
		if err := os.RemoveAll(repoPath); err != nil {
			return fmt.Errorf("clear temp repo path before archive download: %w", err)
		}
		if err := downloadAndExtractGitHubArchive(ctx, owner, repo, branch, repoPath); err == nil {
			return nil
		} else {
			archiveErr = err
		}
	}

	if archiveErr != nil {
		return fmt.Errorf("clone error: %v; archive fallback error: %w", cloneErr, archiveErr)
	}
	return cloneErr
}

func cloneRepoDepth1(ctx context.Context, repoURL, repoPath string) error {
	attempts := gitCloneMaxAttempts
	if attempts <= 0 {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		cmd := gitCommandContext(ctx, "git", "clone", "--depth", "1", repoURL, repoPath)
		out, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		lastErr = fmt.Errorf("git clone attempt %d/%d failed: %v (%s)", attempt, attempts, err, strings.TrimSpace(string(out)))

		if attempt >= attempts || ctx.Err() != nil {
			break
		}
		_ = os.RemoveAll(repoPath)
		timer := time.NewTimer(gitCloneRetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("%w: %v", ctx.Err(), lastErr)
		case <-timer.C:
		}
	}
	return lastErr
}

func parseGitHubOwnerRepo(repoURL string) (owner, repo string, ok bool) {
	repoURL = strings.TrimSpace(repoURL)
	if repoURL == "" {
		return "", "", false
	}

	parsed, err := url.Parse(repoURL)
	if err != nil {
		return "", "", false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Host))
	if host != "github.com" && host != "www.github.com" {
		return "", "", false
	}
	parts := splitPathSegments(parsed.Path)
	if len(parts) < 2 {
		return "", "", false
	}
	owner = strings.TrimSpace(parts[0])
	repo = strings.TrimSuffix(strings.TrimSpace(parts[1]), ".git")
	if owner == "" || repo == "" {
		return "", "", false
	}
	return owner, repo, true
}

func downloadAndExtractGitHubArchive(ctx context.Context, owner, repo, branch, repoPath string) error {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	branch = strings.TrimSpace(branch)
	if owner == "" || repo == "" || branch == "" {
		return fmt.Errorf("owner/repo/branch are required")
	}

	archiveURL := fmt.Sprintf(githubArchiveEndpointTemplate, owner, repo, branch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
	if err != nil {
		return fmt.Errorf("build archive request: %w", err)
	}
	resp, err := (&http.Client{Timeout: 25 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("download archive: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("archive http status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		return fmt.Errorf("create repo path: %w", err)
	}
	if err := extractGitHubTarGz(resp.Body, repoPath); err != nil {
		return fmt.Errorf("extract archive: %w", err)
	}
	return nil
}

func extractGitHubTarGz(src io.Reader, dst string) error {
	gzipReader, err := gzip.NewReader(src)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}
	defer gzipReader.Close()

	dst = filepath.Clean(dst)
	tr := tar.NewReader(gzipReader)
	for {
		header, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		name := strings.TrimSpace(header.Name)
		if name == "" {
			continue
		}
		cleaned := filepath.Clean(name)
		parts := strings.Split(cleaned, "/")
		if len(parts) < 2 {
			continue
		}
		rel := filepath.Join(parts[1:]...)
		target := filepath.Join(dst, rel)
		target = filepath.Clean(target)
		if target != dst && !strings.HasPrefix(target, dst+string(filepath.Separator)) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Chmod(0o600); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create skills directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.statePath), 0o755); err != nil {
		return fmt.Errorf("create skills state directory: %w", err)
	}

	data, err := os.ReadFile(s.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			s.state = stateFile{Skills: map[string]skillStateRecord{}}
			return s.ensureBuiltinSkillsLocked()
		}
		return fmt.Errorf("read skills state: %w", err)
	}

	if strings.TrimSpace(string(data)) == "" {
		s.state = stateFile{Skills: map[string]skillStateRecord{}}
		return s.ensureBuiltinSkillsLocked()
	}

	var parsed stateFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("decode skills state: %w", err)
	}
	if parsed.Skills == nil {
		parsed.Skills = map[string]skillStateRecord{}
	}
	s.state = parsed
	return s.ensureBuiltinSkillsLocked()
}

func (s *Store) persistLocked() error {
	if s.state.Skills == nil {
		s.state.Skills = map[string]skillStateRecord{}
	}

	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode skills state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.statePath), 0o755); err != nil {
		return fmt.Errorf("create skills state directory: %w", err)
	}
	tmpPath := s.statePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write temp skills state: %w", err)
	}
	if err := os.Rename(tmpPath, s.statePath); err != nil {
		return fmt.Errorf("rename skills state: %w", err)
	}
	return nil
}

func (s *Store) ensureBuiltinSkillsLocked() error {
	if s.state.Skills == nil {
		s.state.Skills = map[string]skillStateRecord{}
	}

	changed := false
	for _, builtin := range builtinSkills {
		id := strings.TrimSpace(builtin.ID)
		if id == "" {
			continue
		}

		skillDir := filepath.Join(s.dir, id)
		skillPath := filepath.Join(skillDir, "SKILL.md")
		shouldWriteFile := false
		if _, err := os.Stat(skillPath); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("stat builtin skill %q: %w", id, err)
			}
			if err := os.MkdirAll(skillDir, 0o755); err != nil {
				return fmt.Errorf("create builtin skill dir %q: %w", id, err)
			}
			shouldWriteFile = true
		}

		record, exists := s.state.Skills[id]
		if !exists {
			record.Enabled = true
			record.Source = builtinSkillSource
			record.UpdatedAt = time.Now()
			s.state.Skills[id] = record
			changed = true
			shouldWriteFile = true
		}
		if strings.EqualFold(strings.TrimSpace(record.Source), builtinSkillSource) {
			shouldWriteFile = true
		}
		if strings.TrimSpace(record.Source) == "" {
			record.Source = builtinSkillSource
			s.state.Skills[id] = record
			changed = true
		}
		if shouldWriteFile {
			if err := os.WriteFile(skillPath, []byte(renderSkillMarkdown(builtin)+"\n"), 0o600); err != nil {
				return fmt.Errorf("write builtin skill %q: %w", id, err)
			}
			changed = true
		}
	}

	if !changed {
		return nil
	}
	return s.persistLocked()
}

func (s *Store) listSkillsLocked() ([]Skill, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("read skills directory: %w", err)
	}

	out := make([]Skill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillID := strings.TrimSpace(entry.Name())
		if skillID == "" {
			continue
		}
		skillPath := filepath.Join(s.dir, skillID, "SKILL.md")
		data, err := os.ReadFile(skillPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", skillPath, err)
		}

		name, description, prompt := parseSkillMarkdown(string(data))
		if strings.TrimSpace(name) == "" {
			name = skillID
		}
		description = normalizeSkillDescription(description, name, prompt)

		record, hasRecord := s.state.Skills[skillID]
		enabled := true
		if hasRecord {
			enabled = record.Enabled
		}
		updatedAt := record.UpdatedAt
		if updatedAt.IsZero() {
			if info, statErr := os.Stat(skillPath); statErr == nil {
				updatedAt = info.ModTime()
			}
		}

		out = append(out, Skill{
			ID:          skillID,
			Name:        strings.TrimSpace(name),
			Description: strings.TrimSpace(description),
			Prompt:      strings.TrimSpace(prompt),
			Enabled:     enabled,
			Source:      strings.TrimSpace(record.Source),
			UpdatedAt:   updatedAt,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) trimAutoSkillsLocked(limit int) {
	skills, err := s.listSkillsLocked()
	if err != nil {
		return
	}
	autos := make([]Skill, 0)
	for _, skill := range skills {
		if strings.HasPrefix(strings.TrimSpace(skill.ID), autoSkillIDPrefix) {
			autos = append(autos, skill)
		}
	}
	if len(autos) <= limit {
		return
	}

	sort.Slice(autos, func(i, j int) bool {
		if autos[i].UpdatedAt.Equal(autos[j].UpdatedAt) {
			return autos[i].ID < autos[j].ID
		}
		return autos[i].UpdatedAt.Before(autos[j].UpdatedAt)
	})

	removeCount := len(autos) - limit
	for i := 0; i < removeCount; i++ {
		id := autos[i].ID
		_ = os.RemoveAll(filepath.Join(s.dir, id))
		delete(s.state.Skills, id)
	}
}

func parseSkillMarkdown(markdown string) (name, description, prompt string) {
	text := strings.TrimSpace(strings.ReplaceAll(markdown, "\r\n", "\n"))
	if text == "" {
		return "", "", ""
	}
	if !strings.HasPrefix(text, "---\n") {
		return "", "", text
	}

	rest := strings.TrimPrefix(text, "---\n")
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return "", "", text
	}
	header := rest[:idx]
	body := strings.TrimSpace(rest[idx+5:])

	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			if unquoted, err := strconv.Unquote(value); err == nil {
				value = unquoted
			}
		}
		switch key {
		case "name":
			name = value
		case "description":
			description = value
		}
	}
	return strings.TrimSpace(name), strings.TrimSpace(description), body
}

func renderSkillMarkdown(skill Skill) string {
	name := strings.TrimSpace(skill.Name)
	if name == "" {
		name = strings.TrimSpace(skill.ID)
	}
	description := normalizeSkillDescription(skill.Description, name, skill.Prompt)
	return strings.TrimSpace(
		"---\n" +
			"name: " + quoteYAMLString(name) + "\n" +
			"description: " + quoteYAMLString(description) + "\n" +
			"---\n\n" +
			strings.TrimSpace(skill.Prompt),
	)
}

func quoteYAMLString(v string) string {
	return strconv.Quote(strings.TrimSpace(strings.ReplaceAll(v, "\n", " ")))
}

func validateSkillID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("skill id is required")
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return fmt.Errorf("skill id must match [a-zA-Z0-9_-]+")
	}
	return nil
}

func findSkillIDForUpdate(existing []Skill, skill Skill) string {
	name := strings.TrimSpace(skill.Name)
	if name == "" {
		return ""
	}
	matched := ""
	for _, item := range existing {
		if !strings.EqualFold(strings.TrimSpace(item.Name), name) {
			continue
		}
		if matched != "" {
			return ""
		}
		matched = item.ID
	}
	return matched
}

func findAutoSkillIDByName(existing []Skill, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	for _, skill := range existing {
		if !strings.HasPrefix(strings.TrimSpace(skill.ID), autoSkillIDPrefix) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(skill.Name), name) {
			return skill.ID
		}
	}
	return ""
}

func generateAutoSkillID(existing []Skill, name, prompt string) string {
	seed := sanitizeIdentifier(name)
	if seed == "" {
		seed = sanitizeIdentifier(prompt)
	}
	if seed == "" {
		seed = "skill"
	}

	candidate := autoSkillIDPrefix + seed
	used := make(map[string]struct{}, len(existing))
	for _, skill := range existing {
		used[skill.ID] = struct{}{}
	}
	if _, ok := used[candidate]; !ok {
		return candidate
	}
	for i := 2; ; i++ {
		next := fmt.Sprintf("%s%s-%d", autoSkillIDPrefix, seed, i)
		if _, ok := used[next]; !ok {
			return next
		}
	}
}

func generateUniqueSkillID(existing []Skill, name, prompt string) string {
	used := make(map[string]struct{}, len(existing))
	for _, skill := range existing {
		used[skill.ID] = struct{}{}
	}
	base := sanitizeIdentifier(name)
	if base == "" {
		base = sanitizeIdentifier(prompt)
	}
	if base == "" {
		base = "skill"
	}
	if _, ok := used[base]; !ok {
		return base
	}
	for i := 2; ; i++ {
		next := fmt.Sprintf("%s-%d", base, i)
		if _, ok := used[next]; !ok {
			return next
		}
	}
}

func sanitizeIdentifier(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range input {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_':
			if b.Len() == 0 || lastDash {
				continue
			}
			b.WriteRune('-')
			lastDash = true
		default:
			if b.Len() == 0 || lastDash {
				continue
			}
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func trimSkillText(v string, max int) string {
	v = strings.TrimSpace(v)
	if v == "" || max <= 0 {
		return ""
	}
	runes := []rune(v)
	if len(runes) <= max {
		return v
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return strings.TrimSpace(string(runes[:max-3])) + "..."
}

func normalizeSkillDescription(description, name, prompt string) string {
	description = strings.TrimSpace(strings.ReplaceAll(description, "\n", " "))
	if description != "" {
		return trimSkillText(description, 140)
	}

	base := strings.TrimSpace(prompt)
	if base == "" {
		base = strings.TrimSpace(name)
	}
	if base == "" {
		return ""
	}
	base = strings.ReplaceAll(base, "\n", " ")
	return trimSkillText(base, 140)
}

func splitPathSegments(path string) []string {
	trimmed := strings.Trim(path, " /")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func findSkillDir(repoPath, skillID string) (string, error) {
	candidates := []string{
		filepath.Join(repoPath, "skills", skillID),
		filepath.Join(repoPath, skillID),
	}
	for _, dir := range candidates {
		if stat, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil && !stat.IsDir() {
			return dir, nil
		}
	}

	best := ""
	bestDepth := 1 << 30
	walkErr := filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if filepath.Base(path) != skillID {
			return nil
		}
		if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(repoPath, path)
		if relErr != nil {
			return nil
		}
		depth := len(strings.Split(rel, string(filepath.Separator)))
		if depth < bestDepth {
			bestDepth = depth
			best = path
		}
		return nil
	})
	if walkErr != nil {
		return "", fmt.Errorf("scan repo skills: %w", walkErr)
	}
	if best == "" {
		return "", fmt.Errorf("skill %q not found in repository", skillID)
	}
	return best, nil
}

func copyDir(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("create destination skill dir: %w", err)
	}
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create destination file dir: %w", err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy file: %w", err)
	}
	if err := out.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod destination file: %w", err)
	}
	return nil
}

func cloneSkills(in []Skill) []Skill {
	if len(in) == 0 {
		return nil
	}
	out := make([]Skill, len(in))
	copy(out, in)
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
