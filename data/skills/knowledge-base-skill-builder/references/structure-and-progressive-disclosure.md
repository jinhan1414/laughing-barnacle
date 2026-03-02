# 结构与渐进式披露

知识库型 Skill 的核心原则：
- 主 `SKILL.md` 只做导航和行为约束。
- 细节全部拆到 `references/`。
- 只在必要时读取具体 reference 文件。

推荐目录：

```text
my-knowledge-base-skill/
├── SKILL.md
└── references/
    ├── authority.md
    ├── concepts.md
    ├── protocol-or-sdk.md
    ├── repo-implementation.md
    └── troubleshooting.md
```

主 `SKILL.md` 只保留这些内容：
- 触发条件
- Skill 目标
- 权威入口
- 每类问题应该先读哪个 reference
- 回答框架
- 禁止事项

常见 reference 拆分方法：
- `authority.md`
  - 权威来源和优先级
- `concepts.md`
  - 核心概念、对象模型、术语区别
- `protocol-lifecycle-and-transport.md` / `sdk-and-debugging.md`
  - 协议、SDK、调试、版本等强主题内容
- `repo-implementation.md`
  - 本仓库代码现状
- `troubleshooting.md`
  - 排障顺序和证据链

渐进式披露的落地口径：
1. 索引层：只让模型看到 `name` / `description`
2. 命中层：读取 `SKILL.md`
3. 深读层：再按需读 `references/<file>.md`

判断一个信息该放哪一层：
- 若它决定“这个 Skill 什么时候被触发”，放 frontmatter description。
- 若它决定“Skill 被触发后先做什么”，放 `SKILL.md`。
- 若它是具体事实、概念细节、版本差异、代码路径，放 `references/`。

反例：
- 把所有官方链接、全部概念解释、仓库代码路径、排障方法都塞进主 `SKILL.md`。
- 结果是主文过重，失去渐进式披露价值。
