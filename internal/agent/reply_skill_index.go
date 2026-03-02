package agent

import (
	"fmt"
	"strings"
)

func buildSkillIndexPrompt(allSkillIndex []string, section int) string {
	if len(allSkillIndex) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## %d. Skills 索引 (共 %d 条)\n", section, len(allSkillIndex)))
	b.WriteString("如需技能详情，仅在必要时调用：context__read(resource=\"skills\", action=\"read\", id=\"<skill_id>\")。\n")
	b.WriteString("若技能正文引用了 references 或 scripts，先读取资源索引：context__read(resource=\"skills\", action=\"index\", id=\"<skill_id>\")。\n")
	b.WriteString("读取引用文档时使用：context__read(resource=\"skills\", action=\"read\", id=\"<skill_id>\", path=\"references/<file>.md\")。\n")
	b.WriteString("Skill 调用规则：每轮先按用户请求语义判断是否命中某个 skill_id；一旦命中，无需用户点名，先读取该 skill 详情再执行。\n")
	b.WriteString("为节省上下文，单轮默认只读取 1 个最相关技能；若仍不足，再按需补充读取。\n")
	b.WriteString("若 skill 含脚本，仅在 skill 已声明且确有必要时使用 bash 执行；不要默认读取 scripts 正文。\n")
	b.WriteString("若未命中或技能不适用，再按普通问答流程回复。\n")

	injectCandidates := compactSkillIndexByIDs(allSkillIndex, nil)
	injected := 0
	for _, line := range injectCandidates {
		if line == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("%d. %s\n", injected+1, line))
		injected++
	}
	return strings.TrimSpace(b.String())
}
