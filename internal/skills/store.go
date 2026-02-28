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
			"目标：用最少工具调用维护 MCP 服务，默认预算 3-4 次调用。\n" +
				"硬性约束：本地 API 读写禁止走 bash；读取用 context__read，写入用 maintenance__write。\n" +
				"步骤 1（必做）：先查现状：context__read(resource=\"mcp\", action=\"list\")。\n" +
				"步骤 2（三选一，仅一次写入）：\n" +
				"  a) 新增/更新 streamable_http：maintenance__write(resource=\"mcp\", operation=\"save\", payload={id,name,transport:\"streamable_http\",endpoint,enabled})。\n" +
				"  b) 新增/更新 stdio：maintenance__write(resource=\"mcp\", operation=\"save\", payload={id,name,transport:\"stdio\",command,args,enabled})。\n" +
				"  c) 启停/删除：maintenance__write(resource=\"mcp\", operation=\"toggle|delete\", payload={id,enabled?})。\n" +
				"步骤 3（必做）：回读校验：context__read(resource=\"mcp\", action=\"list\")，并仅基于回读结果汇报 diff。\n" +
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
			"目标：用最少工具调用维护 A2A 接入，默认预算 3-4 次调用。\n" +
				"硬性约束：本地 API 读写禁止走 bash；读取用 context__read，写入用 maintenance__write。\n" +
				"步骤 1（必做）：先查现状：context__read(resource=\"a2a\", action=\"list\")。\n" +
				"步骤 2（按需执行一个写分支）：\n" +
				"  a) 新增/更新：maintenance__write(resource=\"a2a\", operation=\"save\", payload={id,name,description,endpoint,agent_card_url,auth_token,enabled})。\n" +
				"  b) 启停：maintenance__write(resource=\"a2a\", operation=\"toggle\", payload={id,enabled})。\n" +
				"  c) 删除：maintenance__write(resource=\"a2a\", operation=\"delete\", payload={id})。\n" +
				"步骤 3（必做）：回读校验：context__read(resource=\"a2a\", action=\"list\")；必要时再读详情：context__read(resource=\"a2a\", action=\"read\", id=\"<agent_id>\")。\n" +
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
				"步骤 1：优先使用系统已注入的“已启用 A2A Agent 索引”选择最相关且 enabled=true 的 agent_id（默认不额外读取列表）。\n" +
				"步骤 2：如索引不足，再读单个详情：context__read(resource=\"a2a\", action=\"read\", id=\"<agent_id>\")。\n" +
				"步骤 3：统一通过后台任务网关执行：async_task__submit(task_type=a2a, request, agent_id, agent_input)。\n" +
				"步骤 4：按需回读进度：async_task__get(task_id)；需要中断时使用 async_task__cancel(task_id)。\n" +
				"约束：async_task__submit.request 仅写稳定任务摘要，禁止“再次/重新/继续/再”等轮次词，以及“调用 codex-local”这类过程措辞。\n" +
				"约束：agent_input 直接描述目标任务与验收标准，禁止出现“调用 <agent_id>”“让 <agent_id> 执行”等调度语句。\n" +
				"约束：request 与 agent_input 语义一致但更短（建议 <= 60 字）；示例：request=“分析项目技术栈与风险”，agent_input=“分析 E:\\\\projects\\\\ai\\\\work-notiy，输出技术栈、启动构建、核心模块、风险与优化建议”。\n" +
				"约束：仅在用户明确要求刷新列表或执行前需要一致性校验时，才读取列表：context__read(resource=\"a2a\", action=\"list\")。\n" +
				"约束：禁止一次性读取全部 Agent 详情；单轮默认只读 1 个详情，不足再补读。\n" +
				"约束：禁止调用 a2a__send/a2a__get/a2a__cancel。\n" +
				"约束：只基于工具回读结果汇报，不得在无执行证据时声称“已完成”。",
		),
		Enabled: true,
		Source:  builtinSkillSource,
	},
	{
		ID:          "async-task-orchestrator",
		Name:        "后台任务编排",
		Description: "当任务耗时较长或需要回合外持续跟踪时使用",
		Prompt: strings.TrimSpace(
			"目标：由模型自主决定是否将任务转后台，并通过内置 async_task 工具闭环执行。\n" +
				"执行入口固定：async_task__submit；查询入口：async_task__get；取消入口：async_task__cancel。\n" +
				"submit 参数硬约束：task_type 与 request 必填；当 task_type=a2a 时，agent_id 与 agent_input 必填。\n" +
				"submit 文案约束：request 仅写稳定任务摘要，禁止“再次/重新/继续/马上”等轮次词；详细要求写入 agent_input 或后续工具参数。\n" +
				"submit 文案约束：task_type=a2a 时，agent_input 禁止“调用 <agent_id>”类调度语句，必须直接描述要执行的业务任务。\n" +
				"默认 notify_on_finish=true，任务终态后系统会主动通知用户。\n" +
				"仅在需要排障时才读取日志窗口：async_task__get(include_logs=true, log_cursor, log_limit<=200)。\n" +
				"禁止通过 Skill 直接发 HTTP 执行后台任务。\n" +
				"禁止在无执行证据时声称“已完成”。",
		),
		Enabled: true,
		Source:  builtinSkillSource,
	},
	{
		ID:          "skills-config-maintainer",
		Name:        "Skills 配置维护",
		Description: "当用户要求安装/新增/删除/启停 Skill 时使用",
		Prompt: strings.TrimSpace(
			"目标：用最少工具调用维护 Skill，默认预算 3-4 次调用。\n" +
				"硬性约束：本地 API 读写禁止走 bash；读取用 context__read，写入用 maintenance__write。\n" +
				"步骤 1（必做）：先查现状：context__read(resource=\"skills\", action=\"list\")。\n" +
				"步骤 2（按需执行一个写分支）：\n" +
				"  a) skills.sh 安装：maintenance__write(resource=\"skills\", operation=\"install\", payload={skills_sh_url})。\n" +
				"  b) 手动新增/更新：maintenance__write(resource=\"skills\", operation=\"save\", payload={id,name,description,prompt,enabled})。\n" +
				"  c) 启停/删除：maintenance__write(resource=\"skills\", operation=\"toggle|delete\", payload={id,enabled?})。\n" +
				"步骤 3（必做）：回读校验：context__read(resource=\"skills\", action=\"list\")，并仅基于回读结果汇报 diff 与启用状态。\n" +
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
			"目标：以最少工具调用完成定时任务配置并可验证，默认预算 3-4 次调用。\n" +
				"写入字段硬约束：maintenance__write(resource=\"schedules\", operation=\"save\") 的 payload 仅接受 id,name,description,action=skill:<skill_id>,cron_expr,enabled；禁止 cron/prompt/action=reminder。\n" +
				"硬性约束：本地 API 读写禁止走 bash；读取用 context__read，写入用 maintenance__write。\n" +
				"硬性约束：创建/更新 Skill 固定使用 maintenance__write(resource=\"skills\", operation=\"save\")。\n" +
				"硬性约束：action 必须是 skill:<skill_id>；skill_id 仅允许 [a-zA-Z0-9_-]，必须使用普通连字符 '-'。\n" +
				"硬性约束：用户提醒类任务（如打卡/会议/出行提醒）禁止绑定流程性内置 skill（project-memory-maintainer、context-archive-recall、mcp-config-maintainer、skills-config-maintainer、schedule-config-maintainer）。\n" +
				"硬性约束：提醒类任务必须先创建或复用专用 reminder skill（例如 punch-card-reminder），再用 action=skill:<reminder_skill_id> 绑定。\n" +
				"硬性约束：定时任务列表固定使用 context__read(resource=\"schedules\", action=\"list\")。\n" +
				"步骤 1（必做）：先查技能：context__read(resource=\"skills\", action=\"list\")；仅在需要更新已有任务时再查一次 context__read(resource=\"schedules\", action=\"list\")。\n" +
				"步骤 2（按分支执行一次写入）：\n" +
				"  a) skill 不存在：先创建 skill：maintenance__write(resource=\"skills\", operation=\"save\", payload={id,name,description,prompt,enabled})。\n" +
				"  b) skill 已存在但禁用：先启用：maintenance__write(resource=\"skills\", operation=\"toggle\", payload={id,enabled:true})。\n" +
				"  c) 保存任务：maintenance__write(resource=\"schedules\", operation=\"save\", payload={id,name,description,action,cron_expr,enabled})。\n" +
				"  d) 立即执行（仅用户明确要求时）：maintenance__write(resource=\"schedules\", operation=\"run\", payload={id})。\n" +
				"步骤 3（必做）：回读一次：context__read(resource=\"schedules\", action=\"list\")；仅基于回读结果汇报是否生效。\n" +
				"失败处理约束：若接口调用失败，只允许重试同一 API 或先查 /healthz；禁止改为目录扫描、系统全盘搜索或 Linux 命令探测。\n" +
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
				"步骤 1：先调用 context__read(resource=\"memory\", action=\"read\", path=\"/conversation/archive/<segment_id>/index\") 读取归档索引（标题与分节）。\n" +
				"步骤 2：根据问题只选择必要 section_id，再调用 context__read(resource=\"memory\", action=\"section\", path=\"/conversation/archive/<segment_id>/index\", section_id=\"<section_id>\") 拉取具体分节。\n" +
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
				"先查项目目录索引：context__read(resource=\"memory\", action=\"index\", path=\"/projects\")。\n" +
				"必要时按需读取详情：context__read(resource=\"memory\", action=\"read\", path=\"/projects/<project_id>/overview\")。\n" +
				"仅当当前对话存在明确项目变更时写入；信息不确定时禁止写入。\n" +
				"写入接口：POST /api/memory/upsert（JSON body，需 Content-Type: application/json）；path 采用 /projects/<project_id>/overview|milestones|risks|decisions。\n" +
				"低置信候选会进入 /api/memory/inbox，可通过 POST /api/memory/inbox/review 做 confirm/reject。\n" +
				"facts/sections 支持结构化增量写入；优先小步更新，不要一次性重写全部项目。\n" +
				"写入后再次调用 context__read(resource=\"memory\", action=\"read\", path=\"<path>\") 做结果校验，再向用户汇报“已记录哪些变化”；必要时可 POST /api/memory/maintenance/run 做维护。",
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
