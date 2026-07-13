# Frenzy Fork 项目维护模型

本文说明应用 fork 内部怎样接收需求、修改代码、管理本地补丁并把候选版本交给私有运维仓库。它不包含任何生产凭证或 AWS 资源细节。

## 维护目标

1. upstream 始终可以重新同步，不把生产差异堆成一次巨型 merge。
2. 每个本地差异都能回答“为什么存在、怎样验证、何时删除”。
3. 应用、运行配置、基础设施和数据迁移分开发布。
4. 已部署的源码、镜像和配置都可追溯且不可变。
5. 新的 Codex 先恢复事实与边界，再修改代码或生产状态。

## 四条事实轴

不要把所有信息排成一条会互相覆盖的“优先级”。接手和发布时要并列核对四条轴：

| 轴 | 回答的问题 | 权威来源 |
|---|---|---|
| Desired | 应该是什么 | Git 中应用代码、Terraform 和版本化非敏感环境配置 |
| Actual | 实际是什么 | 只读应用/AWS API 与 Terraform remote state |
| Artifact | 运行的是什么 | app tag/SHA、ECR digest、AMI、CA checksum、SSM version |
| Approved/history | 谁批准、发生过什么 | change record、release manifest、annotated tags、测试证据 |

实时状态能证明现实，但不会自动变成获准的期望状态。四条轴不一致时报告 drift，不能让其中一条静默覆盖另一条。

## 永久约束

- `upstream` 只允许 fetch，push URL 必须保持 `DISABLED`。
- `origin` 是 Frenzy fork；生产部署的 release 分支和 tag 不 rebase、不 force-push。
- AWS、Terraform、域名、资源 ID、账号配置快照和 release manifest 只进入私有 `infra/` 仓库。
- 通用应用修复尽量贡献 upstream；生产专属配置尽量留在私有运维仓库。
- 已经执行过的 SQL migration 不修改；新变化必须新增 migration。
- 不使用 `latest` 作为生产工件；源码 revision、镜像 tag 和 digest 必须同时记录。
- 一次发布只选择一个主轴，避免应用升级、模式切换、数据层 HA 和账号配置同时变化。

## 仓库与分支生命周期

```text
upstream tag
   |
   +-- integration/v<upstream>-frenzy.<n>   冲突处理、补丁重放、测试
           |
           +-- release/<upstream>-frenzy.<n>  审批后的不可变发布源码
                    |
                    +-- frenzy/app/v<upstream>-frenzy.<n>
```

分支职责：

| 分支 | 用途 | 允许操作 |
|---|---|---|
| `main` | fork 默认控制/接手入口与长期开发入口 | PR 合并；必须含 AGENTS/CI；不得直接代表生产版本 |
| `integration/*` | 一次 upstream 或大版本集成 | 可修复冲突和重放补丁；合并后删除 |
| `release/*` | 已批准或已部署源码 | 固定 candidate 后不再追加代码或文档；不改写历史 |
| `contrib/*` | 可回馈 upstream 的通用修复 | 保持单一主题，便于向 upstream 提 PR |

当前 GitHub 分支保护状态属于可变事实。开始 release 前必须实时查询，不能因为文档写了“应该保护”就假定保护已经生效。

## 代码与配置所有权

| 变化 | 权威位置 | 不应放置的位置 |
|---|---|---|
| 通用 Go/Vue 行为 | 本应用仓库 | 私有 Terraform 模板中的临时 patch |
| 通用默认配置示例 | `deploy/` 与应用文档 | 生产 secret |
| Frenzy 本地代码补丁 | 独立 commit + `PATCH_QUEUE.md` | 一个无法拆分的长期 merge commit |
| AWS 运行配置 | `infra/environments/` 与 Terraform | 应用仓库 |
| AI 账号、用户、组、Key | 应用数据库 | Git、聊天或 shell 历史 |
| 生产发布事实 | `infra/releases/` | 可变的 README 段落 |

## 变更分类

收到任务后先归类，再决定测试和发布路径。

| 类别 | 示例 | 最小处理 |
|---|---|---|
| 文档 | AGENTS、runbook、模式说明 | 链接/事实/命令校验；不触发应用发布 |
| 应用行为 | handler、service、调度、前端 | 定向测试 + 完整 release gate |
| 依赖/工具链 | `go.mod`、lockfile、Actions | 构建、测试、安全扫描、容器扫描 |
| Schema | 新 SQL migration、Ent schema | migration 副本演练 + 前后版本兼容审查 |
| 运行配置 | 新配置项、默认值、环境变量 | 默认值、缺失值和旧配置兼容测试 |
| 生产配置 | RUN_MODE、镜像激活、域名 | 转入私有 ops 流程；不得从应用仓库直接部署 |
| 账号数据 | 账号、组、用户、Key | 脱敏盘点、备份、应用 API 验收；不进入 Git |

同一提交可以涉及多个文件，但应只有一个清晰目的。若一个候选同时跨越 Schema、运行配置和基础设施，应拆成可独立验证和回滚的发布阶段。

## 标准维护流程

### 1. 恢复基线

```bash
git status --short --branch
git remote -v
git log -5 --oneline --decorate
git tag --points-at HEAD
git diff --stat
```

若存在 `infra/`，再运行私有仓库的 `bin/codex-context.sh`。不要覆盖已有修改，也不要把聊天记录当作线上事实。

识别线上版本时从私有 stable release manifest 开始，沿 `deployed tag → full source SHA → image digest → ops revision` 闭环回读。当前 branch `HEAD` 可能只比运行版本多文档或 CI 提交，不能替代 deployed tag。

### 2. 写明变化契约

在改代码前明确：

- 用户可观察到的变化；
- 哪些旧客户端、旧配置和旧数据库必须继续兼容；
- 是否新增 migration；
- 新旧版本能否同时运行；
- 镜像回滚是否仍安全；
- 本地 patch 是长期差异还是准备贡献 upstream。

### 3. 小步实现

- 通用修复优先在 `contrib/*` 独立完成。
- Frenzy 差异保持为可单独 cherry-pick/reimplement 的 commit。
- 不顺手格式化无关目录，不把生成文件和业务改动混成不可审查的大 diff。
- 配置新增必须同时处理显式值、默认值、非法值和向后兼容。

### 4. 按风险验证

| 改动 | 必须执行 |
|---|---|
| Go | `cd backend && go test -tags=unit ./...`；相关 integration tests |
| Ent | `cd backend && go generate ./ent`，检查生成 diff 和 migration |
| Wire | `cd backend && go generate ./cmd/server` |
| Vue/TS | `pnpm --dir frontend run lint:check`、`typecheck`、相关 Vitest |
| 依赖 | 完整测试、`govulncheck`、前端 audit、容器扫描 |
| release 候选 | 后端 unit/integration、完整前端 tests/build、lint、安全和生产链路 E2E |

当前 CI 只是下限，不是完整发布证明；workflow/checks 未实际运行时更不能声称 CI 通过。release 候选不得因为 GitHub 显示绿色就跳过生产特有的代理、分组、计费和真实模型 smoke。GitHub 的期望与实际状态见 [`GITHUB_GOVERNANCE.md`](GITHUB_GOVERNANCE.md)。

### 5. 更新 patch ledger

任何新增、删除、重写本地运行时补丁，都必须同步更新 [`PATCH_QUEUE.md`](PATCH_QUEUE.md)：

- upstream base；
- 稳定 Patch ID、commit、stable patch-id 与目的；
- 生产依赖；
- 定向测试；
- upstream 状态；
- 删除条件。

纯文档/CI 提交不属于运行时 patch queue。它们应进入 `main`，不要追加到已经固定的 release 分支；历史 release 已存在这种差异时，要明确它没有进入当前运行镜像。

### 6. 交付 release 候选

应用仓库只交付：

- clean、已提交的源码 revision；
- 不可变 `frenzy/app/*` tag；
- 测试、安全扫描、migration 兼容结论；
- 可复现的 `linux/amd64` 镜像输入。

镜像推入私有 ECR、激活、Terraform、smoke、观察和回滚属于 `infra/docs/RELEASE_RUNBOOK.md`。

## 数据库 migration 规则

- 已应用 migration 的 filename 和 checksum 是不可变接口。
- 默认 migration 在事务中执行；`*_notx.sql` 仅用于明确需要非事务的操作，例如并发索引。
- 每个 migration 必须判断锁表时间、数据规模、失败重试和旧版本兼容。
- 需要低中断滚动发布时使用 expand → migrate/backfill → contract：
  1. 先增加兼容字段/表/索引；
  2. 新旧应用同时支持两种结构；
  3. 独立回填并验证；
  4. 所有旧实例退出后，另一个发布再删除旧结构。
- 任何 drop/rename/约束收紧都不能和首次读取新结构的应用放在同一滚动发布中。

应用层滚动发布契约见 [`ROLLING_RELEASE_CONTRACT.md`](ROLLING_RELEASE_CONTRACT.md)。

## 依赖与安全维护

- 依赖升级必须说明是功能需要、安全修复还是工具链对齐。
- 不机械保留旧 patch：先确认目标 upstream 是否已经包含等价修复。
- `go.mod` 的 Go 版本是权威值；文档和 CI 不应维护另一套容易漂移的版本常量。
- fork 当前 security workflow 覆盖 `govulncheck` 与前端 audit；其他扫描能力必须以实时 workflow 为准。
- 安全例外到期后必须重新评审，不能自动延期；validator 必须拒绝全局过期条目和占位 owner。
- 容器 Inspector 结果属于具体 digest，不可沿用到新镜像。

## 维护节奏

| 节奏 | 内容 | 是否自动部署 |
|---|---|---|
| 每周 | fetch upstream、release/security diff、依赖告警、CI 状态 | 否 |
| 每次变更 | 定向测试、patch ledger、兼容性和回滚说明 | 否 |
| 每月 | 完整测试、安全扫描、旧镜像可用性、文档链接 | 否 |
| 每季度 | 删除已被 upstream 吸收的 patch、恢复演练、分支治理审查 | 否 |

## Codex 交接完成条件

结束一次维护时必须让下一位 Codex 能回答：

1. 当前运行的源码/tag/digest是什么？
2. 当前 branch HEAD 与运行 revision 是否相同，为什么不同？
3. 本地 patch queue 有哪些，分别如何验证和删除？
4. 本次变化是否包含 migration，旧镜像还能否回滚？
5. 应用候选是否已经部署；若是，私有 release manifest 在哪里？
6. 两个仓库是否 clean，远端是否已推送？

答不清这些问题时，不应把任务标记完成。
