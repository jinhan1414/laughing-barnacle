package skills

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"laughing-barnacle/internal/localapi"
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
			"目标：用最少命令维护 MCP 服务，默认命令预算 3-4 条。\n" +
				"硬性约束：写接口必须使用 POST + 表单字段（--data-urlencode），禁止 JSON body。\n" +
				"步骤 1（必做）：先查现状：curl -sS http://127.0.0.1:8080/api/mcp/services。\n" +
				"步骤 2（三选一，仅一次写入）：\n" +
				"  a) 新增/更新 streamable_http：curl -sS -X POST http://127.0.0.1:8080/settings/mcp/save --data-urlencode 'name=<service_name>' --data-urlencode 'transport=streamable_http' --data-urlencode 'endpoint=<endpoint>' --data-urlencode 'enabled=on'。\n" +
				"  b) 新增/更新 stdio：curl -sS -X POST http://127.0.0.1:8080/settings/mcp/save --data-urlencode 'name=<service_name>' --data-urlencode 'transport=stdio' --data-urlencode 'command=<command>' --data-urlencode 'args_json=<json_array>' --data-urlencode 'enabled=on'。\n" +
				"  c) 启停：curl -sS -X POST http://127.0.0.1:8080/settings/mcp/toggle --data-urlencode 'id=<service_id>' --data-urlencode 'enabled=true|false'；删除：curl -sS -X POST http://127.0.0.1:8080/settings/mcp/delete --data-urlencode 'id=<service_id>'。\n" +
				"步骤 3（必做）：回读校验：curl -sS http://127.0.0.1:8080/api/mcp/services，并仅基于回读结果汇报 diff。\n" +
				"默认自主闭环：目标明确时直接完成，不要求用户二次确认；仅在关键参数缺失、权限不足或删除对象歧义时追问。",
		),
		Enabled: true,
		Source:  builtinSkillSource,
	},
	{
		ID:          "skills-config-maintainer",
		Name:        "Skills 配置维护",
		Description: "当用户要求安装/新增/删除/启停 Skill 时使用",
		Prompt: strings.TrimSpace(
			"目标：用最少命令维护 Skill，默认命令预算 3-4 条。\n" +
				"硬性约束：写接口必须使用 POST + 表单字段（--data-urlencode），禁止 JSON body。\n" +
				"步骤 1（必做）：先查现状：curl -sS http://127.0.0.1:8080/api/skills。\n" +
				"步骤 2（按需执行一个写分支）：\n" +
				"  a) skills.sh 安装：curl -sS -X POST http://127.0.0.1:8080/settings/skills/install --data-urlencode 'skills_sh_url=<url>'。\n" +
				"  b) 手动新增/更新：curl -sS -X POST http://127.0.0.1:8080/settings/skills/save --data-urlencode 'name=<skill_slug>' --data-urlencode 'description=<desc>' --data-urlencode 'prompt=<prompt>' --data-urlencode 'enabled=on'。\n" +
				"  c) 启停：curl -sS -X POST http://127.0.0.1:8080/settings/skills/toggle --data-urlencode 'id=<skill_id>' --data-urlencode 'enabled=true|false'；删除：curl -sS -X POST http://127.0.0.1:8080/settings/skills/delete --data-urlencode 'id=<skill_id>'。\n" +
				"步骤 3（必做）：回读校验：curl -sS http://127.0.0.1:8080/api/skills，并仅基于回读结果汇报 diff 与启用状态。\n" +
				"仅当目标不明确时，才额外调用 GET /api/skills/catalog/search?q=<关键词>&limit=8；目标明确时禁止先搜索再重复查询。\n" +
				"默认自主闭环：目标明确时直接执行；仅在重名冲突、删除对象不唯一或关键信息缺失时追问。",
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
			"目标：以最少命令完成定时任务配置并可验证，默认命令预算 3-4 条。\n" +
				"写入字段硬约束：/settings/schedules/save 仅接受 id,name,description,action=skill:<skill_id>,cron_expr,enabled=on；禁止 cron/prompt/action=reminder。\n" +
				"硬性约束：写接口必须使用 POST + 表单字段（--data-urlencode），禁止 JSON body。\n" +
				"硬性约束：action 必须是 skill:<skill_id>；skill_id 仅允许 [a-zA-Z0-9_-]，必须使用普通连字符 '-'。\n" +
				"硬性约束：调用 linux__bash 时，工具参数键必须是 command（不是 cmd）。\n" +
				"硬性约束：Windows cmd 场景下 URL 使用正常双引号，禁止反斜杠转义引号（如 \\\"http://...\\\"）。\n" +
				"硬性约束：使用 --data-urlencode 时，每个字段必须写成 --data-urlencode \"key=value\"（禁止省略双引号）。\n" +
				"硬性约束：定时任务列表接口固定为 GET /api/schedules（禁止 /api/schedules/list）。\n" +
				"步骤 1（必做）：先查技能：curl -sS http://127.0.0.1:8080/api/skills；仅在需要更新已有任务时再查一次 /api/schedules。\n" +
				"步骤 2（按分支执行一次写入）：\n" +
				"  a) skill 不存在：先创建 skill：curl -sS -X POST http://127.0.0.1:8080/settings/skills/save --data-urlencode 'name=<skill_slug>' --data-urlencode 'description=<desc>' --data-urlencode 'prompt=<prompt>' --data-urlencode 'enabled=on'。\n" +
				"  b) skill 已存在但禁用：先启用：curl -sS -X POST http://127.0.0.1:8080/settings/skills/toggle --data-urlencode 'id=<skill_id>' --data-urlencode 'enabled=true'。\n" +
				"  c) 保存任务：curl -sS -X POST http://127.0.0.1:8080/settings/schedules/save --data-urlencode 'id=<schedule_id>' --data-urlencode 'name=<name>' --data-urlencode 'description=<desc>' --data-urlencode 'action=skill:<skill_id>' --data-urlencode 'cron_expr=<cron>' --data-urlencode 'enabled=on'。\n" +
				"  d) 立即执行（仅用户明确要求时）：curl -sS -X POST http://127.0.0.1:8080/settings/schedules/run --data-urlencode 'id=<schedule_id>'。\n" +
				"步骤 3（必做）：回读一次：curl -sS http://127.0.0.1:8080/api/schedules；仅基于回读结果汇报是否生效。\n" +
				"失败处理约束：若接口调用失败，只允许重试同一 API 或先查 /healthz；禁止改为目录扫描、系统全盘搜索或 Linux 命令探测。\n" +
				"若工具回显 shell: cmd，后续仅允许使用 cmd 兼容命令（curl、dir、findstr、schtasks、echo）。\n" +
				"禁止写入引用不存在或未启用 skill 的 action。\n" +
				"Cron 规则使用 5 段：分 时 日 月 周（例如 30 8 * * *）。\n" +
				"默认自主闭环：时间与提醒目标明确时直接完成；仅在 cron、目标动作或提醒文案缺失时追问。",
		),
		Enabled: true,
		Source:  builtinSkillSource,
	},
	{
		ID:          "context-archive-recall",
		Name:        "上下文归档召回",
		Description: "当历史摘要信息不足，需要回看被压缩原文片段时使用",
		Prompt: strings.TrimSpace(
			"仅在当前摘要无法支撑回答时触发，且必须按需最小化读取（单轮最多 3 条命令：1 次索引 + 最多 2 次分节）。\n" +
				"步骤 1：先通过 linux__bash 执行 curl -s \"http://127.0.0.1:8080/api/memory/read?path=/conversation/archive/<segment_id>/index\" 读取归档索引（标题与分节）。\n" +
				"步骤 2：根据问题只选择必要 section_id，再执行 curl -s \"http://127.0.0.1:8080/api/memory/section?path=/conversation/archive/<segment_id>/index&section_id=<section_id>\" 拉取具体分节。\n" +
				"禁止一次性拉取全部归档正文；禁止把整段历史原文回填进提示词。\n" +
				"拉取后只提炼与当前问题直接相关的事实、约束和时间点，再继续回答。",
		),
		Enabled: true,
		Source:  builtinSkillSource,
	},
	{
		ID:          "project-memory-maintainer",
		Name:        "项目记忆维护",
		Description: "当用户更新项目进展、风险、里程碑或决策时使用",
		Prompt: strings.TrimSpace(
			"目标：维护结构化项目记忆，服务单用户长期演进。\n" +
				"默认命令预算 3-4 条：1 次索引读取 + 最多 1 次详情读取 + 1 次写入 + 1 次回读。\n" +
				"先查项目目录索引：用 linux__bash 执行 curl -sS \"http://127.0.0.1:8080/api/memory/index?path=/projects\"。\n" +
				"必要时按需读取详情：curl -sS \"http://127.0.0.1:8080/api/memory/read?path=/projects/<project_id>/overview\"。\n" +
				"仅当当前对话存在明确项目变更时写入；信息不确定时禁止写入。\n" +
				"写入接口：POST /api/memory/upsert（JSON body，需 Content-Type: application/json）；path 采用 /projects/<project_id>/overview|milestones|risks|decisions。\n" +
				"写入命令模板：curl -sS -X POST http://127.0.0.1:8080/api/memory/upsert -H 'Content-Type: application/json' -d '{\"mode\":\"merge\",\"path\":\"<path>\",\"title\":\"<title>\",\"type\":\"note\",\"summary\":\"<summary>\",\"facts\":[\"<fact1>\"]}'。\n" +
				"低置信候选会进入 /api/memory/inbox，可通过 POST /api/memory/inbox/review 做 confirm/reject。\n" +
				"facts/sections 支持结构化增量写入；优先小步更新，不要一次性重写全部项目。\n" +
				"写入后再次读取 /api/memory/read?path=<path> 做结果校验，再向用户汇报“已记录哪些变化”；必要时可 POST /api/memory/maintenance/run 做维护。",
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
	localAPI  string

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

	s := &Store{
		dir:       dir,
		statePath: statePath,
		localAPI:  localapi.DefaultBaseURL,
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}
