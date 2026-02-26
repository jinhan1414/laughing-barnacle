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
		ID:          "a2a-config-maintainer",
		Name:        "A2A 接入维护",
		Description: "当用户要求新增/修改/删除/启停 A2A Agent 接入时使用",
		Prompt: strings.TrimSpace(
			"目标：用最少命令维护 A2A 接入，默认命令预算 3-4 条。\n" +
				"硬性约束：A2A 写接口必须使用 JSON body（Content-Type: application/json），禁止 --data-urlencode 表单写入。\n" +
				"硬性约束：Windows 下优先使用 PowerShell 的 Invoke-RestMethod + ConvertTo-Json；若使用 curl，必须用 curl.exe，避免 curl 别名行为差异。\n" +
				"步骤 1（必做）：先查现状：curl -sS http://127.0.0.1:8080/api/a2a/agents。\n" +
				"步骤 2（按需执行一个写分支）：\n" +
				"  a) 新增/更新（PowerShell）：$body=@{id=\"<optional_agent_id>\";name=\"<agent_name>\";description=\"<desc>\";endpoint=\"<a2a_endpoint>\";agent_card_url=\"<optional_card_url>\";auth_token=\"<optional_token>\";enabled=$true}|ConvertTo-Json -Compress; Invoke-RestMethod -Method Post -Uri \"http://127.0.0.1:8080/api/a2a/agents/save\" -ContentType \"application/json\" -Body $body。\n" +
				"  b) 启停（PowerShell）：$body=@{id=\"<agent_id>\";enabled=$true}|ConvertTo-Json -Compress; Invoke-RestMethod -Method Post -Uri \"http://127.0.0.1:8080/api/a2a/agents/toggle\" -ContentType \"application/json\" -Body $body。\n" +
				"  c) 删除（PowerShell）：$body=@{id=\"<agent_id>\"}|ConvertTo-Json -Compress; Invoke-RestMethod -Method Post -Uri \"http://127.0.0.1:8080/api/a2a/agents/delete\" -ContentType \"application/json\" -Body $body。\n" +
				"步骤 3（必做）：回读校验：curl -sS http://127.0.0.1:8080/api/a2a/agents；必要时再读单个详情：curl -sS \"http://127.0.0.1:8080/api/a2a/agents/read?id=<agent_id>\"。\n" +
				"默认自主闭环：目标明确时直接执行；仅在 endpoint/card_url 缺失、删除对象歧义或鉴权信息缺失时追问。",
		),
		Enabled: true,
		Source:  builtinSkillSource,
	},
	{
		ID:          "a2a-task-orchestrator",
		Name:        "A2A 任务编排",
		Description: "当用户要求调用已接入外部 Agent 完成任务时使用",
		Prompt: strings.TrimSpace(
			"目标：基于已接入 A2A Agent 完成任务编排与结果回读。\n" +
				"步骤 1：先读索引：curl -sS http://127.0.0.1:8080/api/a2a/agents，优先选 enabled=true 且最相关的 agent_id。\n" +
				"步骤 2：如索引不足，再读单个详情：curl -sS \"http://127.0.0.1:8080/api/a2a/agents/read?id=<agent_id>\"。\n" +
				"步骤 3：调用内置 A2A 工具执行：a2a__send -> 按需 a2a__get -> 需要中断时 a2a__cancel。\n" +
				"约束：禁止一次性读取全部 Agent 详情；单轮默认只读 1 个详情，不足再补读。\n" +
				"约束：只基于工具回读结果汇报，不得在无执行证据时声称“已完成”。",
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
		ID:          "schedule-config-maintainer",
		Name:        "定时任务配置维护",
		Description: "当用户要求查看/修改 Cron 定时任务时使用",
		Prompt: strings.TrimSpace(
			"目标：以最少命令完成定时任务配置并可验证，默认命令预算 3-4 条。\n" +
				"写入字段硬约束：/settings/schedules/save 仅接受 id,name,description,action=skill:<skill_id>,cron_expr,enabled=on；禁止 cron/prompt/action=reminder。\n" +
				"硬性约束：写接口必须使用 POST + 表单字段（--data-urlencode），禁止 JSON body。\n" +
				"硬性约束：创建/更新 Skill 固定使用 POST /settings/skills/save（禁止 /api/skills/save）。\n" +
				"硬性约束：action 必须是 skill:<skill_id>；skill_id 仅允许 [a-zA-Z0-9_-]，必须使用普通连字符 '-'。\n" +
				"硬性约束：用户提醒类任务（如打卡/会议/出行提醒）禁止绑定流程性内置 skill（project-memory-maintainer、context-archive-recall、mcp-config-maintainer、skills-config-maintainer、schedule-config-maintainer）。\n" +
				"硬性约束：提醒类任务必须先创建或复用专用 reminder skill（例如 punch-card-reminder），再用 action=skill:<reminder_skill_id> 绑定。\n" +
				"硬性约束：调用 linux__bash 时，工具参数键必须是 command（不是 cmd）。\n" +
				"硬性约束：Windows cmd 场景下 URL 使用正常双引号，禁止反斜杠转义引号（如 \\\"http://...\\\"）。\n" +
				"硬性约束：使用 --data-urlencode 时，每个字段必须写成 --data-urlencode \"key=value\"（禁止省略双引号）。\n" +
				"硬性约束：在 cmd + /settings/*/save 场景，优先使用单条 -d \"k=v&k2=v2\"（值先 URL 编码），避免 -F 与多段参数拆分。\n" +
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
