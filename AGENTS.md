# AGENTS.md

本文件为人类开发者/Coding Agent 所写，用于规范化本项目开发过程中的一些细节

Written for human developers and coding agents, to lay out the details of how this project is developed.

## 0. 先看什么 / Read first

1. `README.md` 与 `README_zh-CN.md` —— 面向用户的使用文档
2. `docs/tech/technical-design.md` —— 技术设计文档
3. `docs/` 文件夹下的全部文档 —— 对应功能的介绍文档，更新频次较快，故不在此作硬编码文件路径，请自行查找并查看
3. 要改哪个模块，先读那个文件开头的注释

本项目每个源文件开头均说明了其编写原因。请遵守第三条，不要凭猜测推断代码意图，也不要进行破坏性修改

1. `README.md` and `README_zh-CN.md` — user-facing documentation
2. `docs/tech/technical-design.md` — the technical design document
3. Every document under `docs/` — feature documentation; it changes frequently, so no hard-coded file paths here, look them up yourself
3. Before changing a module, read the comment at the top of that file

Every source file in this project states at the top why it was written. Follow the third point: do not infer
intent from guesswork, and do not make destructive changes.

## 1. 这是什么 / What this is

OptiRoute 是一个面向延迟与稳定性优化的分布式四层反向代理系统

核心角色：

- **Center**：控制面，负责 Edge 管理、拓扑汇总、密钥轮换与容量控制
- **Edge**：数据面与引导面，负责客户端引导、链路探测、选路、Token 校验和 L4 转发
- **Client Agent**：运行在用户侧，负责本地接入、探测与透传
- **Server Agent**：运行在源站侧，负责认证接入、解析真实 IP 并转发到上游

目标平台：

- Windows / Linux / macOS
- IPv4 / IPv6 双栈

OptiRoute is a distributed Layer 4 reverse proxy system optimised for latency and stability.

Core roles:

- **Center**: control plane, responsible for Edge management, topology aggregation, key rotation and capacity control
- **Edge**: data plane and bootstrap plane, responsible for client bootstrap, path probing, route selection, Token validation and L4 forwarding
- **Client Agent**: runs on the user side, responsible for local access, probing and passthrough
- **Server Agent**: runs on the origin side, responsible for authenticated access, resolving the real IP and forwarding to the upstream

Target platforms:

- Windows / Linux / macOS
- IPv4 / IPv6 dual stack


## 2. 架构不变量

以下约束是项目骨架，修改前必须理解并保持

These constraints are the project's skeleton; understand and preserve them before changing anything

### 2.1 Center 是控制面权威

- 所有 Edge 必须向 Center 注册
- Center 是 shared_secret 的生成、轮换和下发方
- Center 负责限制在线 Edge 数量与注册频率
- 拓扑和密钥状态以 Center 为准

- All Edges must register with Center
- Center generates, rotates and distributes the shared_secret
- Center limits the number of online Edges and the registration rate
- Topology and key state are authoritative at Center

### 2.2 Edge 是数据面，不是信任源

- 客户端探测结果不可信，必须校验范围
- 客户端 Token 只允许目标 Edge 验证
- Edge 的注册信息必须经过 Center 校验后才能进入拓扑

- Client probe results are untrusted and must be range-checked
- A client Token may only be verified by the target Edge
- An Edge's registration data must be validated by Center before it enters the topology

### 2.3 Client Agent 保持不透明

- Client Agent 只负责连接、探测和透传
- 它不应解析或理解拓扑语义、路由策略和 Token 内部结构
- 协议演进应尽量保持 Client Agent 的“不透明转发”特性

- Client Agent stays opaque
- The Client Agent only connects, probes and passes traffic through
- It should not parse or understand topology semantics, routing policy or the internal structure of a Token
- Protocol evolution should preserve the Client Agent's "opaque forwarding" property as far as possible

### 2.4 Server Agent 只做源站侧职责

- 校验 Edge 接入身份
- 解析 PPv2 真实 IP
- 按配置决定是否向上游注入 PPv2
- 不参与选路和拓扑决策

- Validate the Edge's access identity
- Resolve the PPv2 real IP
- Decide from configuration whether to inject PPv2 upstream
- Take no part in route selection or topology decisions

### 2.5 引导与数据面分离

- 引导连接负责下发探测列表并返回目标节点
- 业务连接负责 Token 校验和 L4 转发
- 两者可以复用端口，但协议判定必须严格、超时必须明确

- The bootstrap connection delivers the probe list and returns the target node
- The service connection handles Token validation and L4 forwarding
- The two may share a port, but protocol detection must be strict and timeouts explicit

## 3. 安全模型

### 3.1 资源与拒绝服务防护

任何面向不可信网络的入口必须具备：

- 连接超时
- 读写超时或消息大小上限
- 并发握手上限
- 按来源限速
- 明确的拒绝路径和日志

Any entry point facing an untrusted network must have:

- a connection timeout
- a read/write timeout or a message size limit
- a cap on concurrent handshakes
- per-source rate limiting
- an explicit rejection path and logging

### 3.2 本地持久化安全

- 缓存中如包含 secret，文件权限必须最小化
- 磁盘写入不得阻塞控制面消息循环
- 进程退出前应冲刷必要状态，但不能用空状态覆盖有效缓存

- If the cache contains a secret, file permissions must be minimal
- Disk writes must not block the control-plane message loop
- Necessary state should be flushed before exit, but a valid cache must not be overwritten with empty state

## 4. 版本与兼容策略

项目尚未进入 STABLE 版本

- 不承担旧版本向后兼容义务
- 协议、配置、Token 格式可以随版本演进
- 旧格式兼容代码应尽快移除，而不是长期保留
- 升级路径和降级路径需要明确文档说明

Project has not yet reached a STABLE release

- No obligation to stay backward compatible with older versions
- Protocol, configuration and Token format may evolve with the version
- Compatibility code for old formats should be removed soon rather than kept long term
- Upgrade and downgrade paths need explicit documentation

## 5. 配置管理约束

新配置项必须包含：

- 明确默认值
- 范围校验
- 文档说明

其他要求：

- 0 值具有特殊语义时，必须使用可区分“未配置”与“显式 0”的方式
- 配置变更必须同步所有角色文档和示例配置
- 安全相关开关默认值应选择更安全的一侧

New configuration items must include:

- an explicit default value
- range validation
- documentation

Other requirements:

- When 0 has a special meaning, use a representation that distinguishes "unset" from "explicit 0"
- A configuration change must update every role's documentation and the example configuration
- Security-related switches should default to the safer side

### 6. 提交信息 / Commit messages

提交信息**用英文写**，遵循[约定式提交](https://www.conventionalcommits.org/)：
`<type>(<scope>): <description>`，**只写一行**，简洁有力地说明这次改了什么

英语是通用语言，使用英语作为提交信息可以覆盖更多人群，便于后期其他外部开发者做出贡献

不写多行小作文
把写小作文的精力放到**完善注释**上去
提交信息只需要让人知道"这一步做了什么"，而"为什么这么做"应当写在代码注释里，那里才是下一个读代码的人会看的地方

常用 type：`feat` `fix` `refactor` `perf` `docs` `test` `build` `ci` `chore` `style`

```
feat(update): use the Semver library for version comparison
docs: document the .dev/ convention in AGENTS.md
```

Commit messages are **written in English** and follow
[Conventional Commits](https://www.conventionalcommits.org/):
`<type>(<scope>): <description>`, **on a single line**, stating concisely what changed.

English is the lingua franca.
So writing commit messages in English reaches a wider audience and makes it easier for other external developers to contribute later.

No multi-paragraph essays.
Spend that effort on **improving the comments** instead.
The commit messageonly needs to say what this step did, while the reasoning belongs in a code comment, which is where the
next person to read the code will actually look.

Common types: `feat`, `fix`, `refactor`, `perf`, `docs`, `test`, `build`, `ci`, `chore`, `style`

### 7. 测试数据与示例值 / Test data and fixtures

**代码与测试数据应该使用规范、清晰易懂的通用名称进行命名**

- 示例值一律用 `example` / `Example` / `EXAMPLE` / `examples` 这类**通用占位**，
  例如用户名用 `example`，域名用 `example.com`，仓库用 `username/repository`
- **不得**使用真实域名、仓库名、用户名、设备序列号或使用开发机器上的绝对路径来编写代码/测试用例/注释，也不得使用**由它们截取或变体而来的名字**

**Code and test data should use standard, clear and widely understood generic names**

- Fixtures always use **generic placeholders** such as `example` / `Example` / `EXAMPLE` / `examples`,
  e.g. `example` for a username, `example.com` for a domain, `username/repository` for a repository
- **Never** use real domains, repository names, usernames, device serial numbers or absolute paths from a development machine in code, tests or comments, nor names **derived or truncated from them**

## 8. 针对 Agent 的工作方式建议 / Working style for agents

- **先说明再修改** —— 阐明目标与影响范围，涉及高风险变更时优先作出方案规划，之后再执行
- **存在不确定时先行确认，禁止猜测**
- **保持透明** —— 每批工作完成后报告：已完成事项、验证结果、遗留风险

### 提交与版本控制

- **分批修改、小步提交** —— 将任务拆分为多次小幅度修改，适当多用 commit，避免一次性提交大量代码；一批只做一件事，并明确修改范围
- **可维护性** —— 每次 commit 必须完全可构建，并附清晰易懂的 commit 消息，便于后期 review 与问题回溯
- **轻易不推送** —— 可以多次 commit 便于后期整理、排错，但在确认提交代码无问题 (进行实机测试) 前禁止 push 到远端；另外，推送分支、打 tag 与发布 Release 均由维护者决定，不要轻易操作

### 代码与文件规范

- 每个源文件开头须编写注释，说明该文件的用途
- 只格式化**本次修改的文件**，不进行全仓库格式化，不改动行尾
- **临时脚本一律放 `.dev/`** —— 该路径为供测试使用的忽略路径，测试脚本、临时工具均可放在此处
- **严禁修改 AGENTS.md 文件**
- **保持兼容性** —— 禁止在任何情况下使用仅某个平台可用的方案，项目需保证跨系统、跨架构可用，而不是依赖某个平台的特殊功能来实现

### 注释与文档

- **中英双语** —— 全部代码注释与文档、文案统一使用中英双语；变更代码内容时同步更新对应注释
- **双语排版** —— 多行注释写成连续的整段中文 + 连续的整段英文，不得中英逐行交替
- **句子不跨行** —— 一句话必须在一行内写完，严禁分多行书写
- **语言简洁** —— 使用简洁易懂的语言，降低维护成本
- **标点** —— 注释行尾不加句号，除非一行内有两个句子需要区分
- **收尾检查** —— 完成任务后排查本次变更是否遗漏注释，如有遗漏须补全
- **禁止妄下定论** —— 注释、文案与说明中不得使用「永久」「永远」「绝不」「一定会」这类无限定范围的断言，也不得把当前观察到的情况说成普遍规律；应写清边界：在什么条件下、哪个函数走哪条分支、因此不会出现什么结果
- **时效性** —— 修改任何功能后，必须检查 docs 文件夹、双语 README.md 及其他全部文档，更新其中全部与修改部分相关的说明；新增模块时，应补充其对应的说明文档，风格仿照现有文档

- **Explain before modifying** — state the goal and the blast radius; for high-risk changes, plan first and execute afterwards
- **Ask when uncertain, never guess**
- **Stay transparent** — after each batch of work, report what was done, what was verified and what risks remain

### Commits and version control

- **Small batches, small steps** — split work into several small changes and commit often; avoid landing a large amount of code at once, keep one concern per batch and state its scope
- **Maintainability** — every commit must build cleanly and carry a clear commit message, to make later review and root-cause analysis easier
- **Do not push lightly** — multiple commits are fine for later cleanup and debugging, but pushing to the remote is forbidden until the committed code is confirmed good (tested on real machines); pushing branches, tagging and publishing releases are the maintainer's call, do not do them casually

### Code and file conventions

- Every source file must start with a comment explaining the file's purpose
- Format **only the files changed in this batch**, never the whole repository, and do not touch line endings
- **Temporary scripts go in `.dev/`** — that path is ignored and meant for testing; test scripts and throwaway tools belong there
- **Never modify AGENTS.md**
- **Stay portable** — never use a solution that only works on one platform; the project must stay usable across systems and architectures rather than relying on platform-specific features

### Comments and documentation

- **Bilingual** — all code comments, documentation and copy are written in both Chinese and English; update the corresponding comments whenever the code changes
- **Bilingual layout** — multi-line comments are written as one continuous Chinese block followed by one continuous English block, never alternating line by line
- **No sentence spans lines** — a sentence must fit on a single line, never split across lines
- **Keep it concise** — use plain, easy-to-read language to lower maintenance cost
- **Punctuation** — do not end a comment line with a period unless two sentences on that line need separating
- **Final pass** — after finishing a task, check whether this change left any comment behind and fill the gap
- **No absolute claims** — comments, copy and explanations must not use unbounded assertions such as "permanent", "forever", "never" or "always", nor present a current observation as a general rule; state the boundary instead: under which condition, which branch of which function runs, and therefore what result cannot occur
- **Timeliness** — after changing any feature, check the docs folder, both README.md files and every other document, and update all descriptions related to the change; when adding a module, add its documentation in the style of the existing docs