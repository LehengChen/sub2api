# Sub2API Codex 接手说明

本文件适用于应用仓库 `/home/ubuntu/sub2api`。目标是让新的 Codex 会话先恢复事实、边界和验证方法，再开始修改。部署专属信息位于独立的私有运维仓库 `infra/`；若该目录存在，还必须继续阅读 `infra/AGENTS.md`。

## 必读顺序

1. `AGENTS.md`
2. `docs/operations/README.md`
3. 与任务相关的专题文档：
   - 运行模式：`docs/operations/DEPLOYMENT_MODES.md`
   - upstream 同步和发布：`docs/operations/UPSTREAM_MAINTENANCE.md`
4. 涉及 AWS 或生产环境时：`infra/AGENTS.md` 和 `infra/docs/README.md`
5. 应用开发细节：`DEV_GUIDE.md`、对应代码和测试

文档可能滞后。可变事实必须以实时只读检查、发布清单和代码为准；发现漂移时在同一变更中更新文档。

## 工作区是两个 Git 仓库

| 路径 | 仓库 | 职责 |
|---|---|---|
| `.` | `LehengChen/sub2api`（公开 fork） | Go/Vue 应用、应用测试、通用产品文档 |
| `infra/` | `LehengChen/sub2api-ops`（私有） | AWS Terraform、出口镜像、发布清单、生产运行手册 |

`infra/` 是独立仓库，不是应用仓库的普通目录或 submodule。禁止在外层仓库执行 `git add infra`，禁止把 Terraform state、tfvars、backend 配置或生产清单误提交到公开 fork。两个仓库必须分别查看状态、分别提交。

## 每次会话开始

先执行只读检查：

```bash
pwd
git status --short
git branch --show-current
git remote -v
git log -1 --oneline --decorate

if [ -d infra/.git ]; then
  git -C infra status --short
  git -C infra branch --show-current
  git -C infra log -1 --oneline --decorate
fi
```

规则：

- 保留工作区已有修改；不覆盖、不 reset、不把无关修改混进当前任务。
- 搜索优先使用 `rg` / `rg --files`。
- 不根据聊天记录猜测线上版本、账号数量、分组或 AWS 身份。
- 用户只要求分析/诊断时，不执行部署、Terraform apply、账号修改或其他外部写操作。

## 产品和生产方向

- 应用业务运行模式只有 `simple` 与 `standard`。
- 当前生产模式和目标模式必须从私有运维基线核对，不能根据 upstream 默认值推断。
- 项目目标是受控的多用户 Standard：管理员创建用户、按平台分组和调度，公众注册保持关闭。
- `Backend Mode` 是独立站点开关，不是第三种 `RUN_MODE`。若普通用户需要登录和创建 Key，应保持关闭。
- 账号的固定代理绑定负责出口隔离；分组负责资源池、访问与计费。不要按出口 IP 重复拆分业务分组。

## 安全边界

- 绝不把 AI 账号 token、Cookie、sessionKey、OAuth code、API Key、AWS SecretString、数据库密码、Terraform state 内容写入聊天、文档、提交或命令输出。
- 可以输出 secret 的键名、资源 ARN 或脱敏前缀，但不能输出值。
- 不读取或展示 `terraform.tfvars`、`backend.hcl`、`*.tfstate*`、`*.tfplan` 的内容；确需操作时只通过 Terraform/AWS 命令读取必要的非敏感字段。
- 不使用长期 AWS Access Key；使用 SSO/AssumeRole 临时凭证。
- 任何生产变更前都要验证 AWS account、Region、Git revision、计划内容和回滚条件。
- 不使用 `latest` 镜像；生产镜像必须是不可变 tag 并记录 digest。
- 不执行 `terraform destroy`、`-target`、`-lock=false` 或未审查的 apply。

## 应用变更和 upstream

- `origin` 是 fork；`upstream` 仅 fetch，push 必须保持禁用。
- 已部署 release 分支和 tag 不重写历史。
- 不把 `upstream/main` 直接合入生产分支。以明确 upstream release tag 创建 integration 分支，再逐项审查和重放本地 patch queue。
- 能回馈 upstream 的通用修复优先提交 upstream；生产特有逻辑尽量留在配置或私有 ops 仓库。
- 当前 fork 维护策略和命令见 `docs/operations/UPSTREAM_MAINTENANCE.md`。

## 最小验证矩阵

按改动范围选择，不要无差别运行全部测试，也不要跳过相关测试。

| 改动 | 最少验证 |
|---|---|
| Go 代码 | `cd backend && go test -tags=unit ./...`；相关集成测试 |
| Ent schema | `cd backend && go generate ./ent`，检查生成差异和迁移 |
| Wire 依赖 | `cd backend && go generate ./cmd/server` |
| Vue/TS | `pnpm --dir frontend run lint:check`、`pnpm --dir frontend run typecheck`、相关 Vitest |
| 依赖 | 上述测试 + 安全扫描 + lockfile 一致性 |
| 文档 | 链接、路径、Git 忽略规则和事实快照检查 |
| AWS/Terraform | 私有 ops 仓库规则；validate/test/plan，明确批准后才 apply |

应用 release 还必须构建与生产一致的 `linux/amd64` 镜像，并在私有运维流程中记录源码 revision、镜像 digest 和配置 revision。

## 文档维护

- 通用应用事实写入 `docs/operations/`。
- 生产身份、资源、发布清单和 AWS runbook 只写入私有 `infra/docs/`。
- 每份可变状态文档标注 `observed_at` 或“截至日期”。
- 每次发布更新私有基线和 release manifest；每次模式/架构决策更新对应专题文档。
- 文档不得成为秘密存储位置。
