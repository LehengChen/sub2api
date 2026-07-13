# Sub2API 本地开发指南

本文只说明本地开发和工程验证。项目接手、upstream、生产发布和 AWS 规则分别以 [`AGENTS.md`](AGENTS.md)、[`docs/operations/`](docs/operations/README.md) 和私有 `infra/AGENTS.md` 为准；不要从本文件推导生产操作。

Windows/PostgreSQL 手工配置仅是开发者个人环境选择，不是项目基线。开发数据库必须使用本地专用凭据，禁止复用生产 secret、导出生产账号数据或临时把认证改成 `trust` 后遗忘恢复。

## 工程事实来源

| 项目 | 权威来源 |
|---|---|
| Go 版本 | `backend/go.mod` 的 `go` directive；CI 使用 `go-version-file` |
| golangci-lint 版本 | `.github/workflows/backend-ci.yml` |
| Node/pnpm | workflow 与 `frontend/pnpm-lock.yaml` |
| 后端命令 | `backend/Makefile` |
| 前端命令 | `frontend/package.json` |
| 运行配置 | `backend/internal/config/` 与 `deploy/config.example.yaml` |

截至 2026-07-13，`go.mod` 为 Go 1.26.5，fork CI 配置 golangci-lint v2.9。版本变化时修改代码/workflow，不在文档维护另一套固定常量。

## 本地启动

### Docker Compose

仓库提供从本地源码构建的开发 Compose：

```bash
cd deploy
docker compose -f docker-compose.dev.yml up --build
```

先在未提交的本地 `.env` 中设置 Compose 要求的密码/secret。不要把 `.env`、数据库 volume、token、Cookie 或账号导出提交到 Git。

其他部署文件的定位：

- `docker-compose.yml`：完整单机部署；
- `docker-compose.standalone.yml`：应用外接 PostgreSQL/Redis；
- `docker-compose.local.yml`：本地镜像/配置场景；
- `docker-compose.dev.yml`：从当前工作区 build。

### 直接运行

若 PostgreSQL/Redis 已在本地准备好：

```bash
cd backend
go run ./cmd/server/
```

运行参数以配置代码和示例为准。开发环境也不要在命令行历史中直接展开真实凭据。

## 常用验证

```bash
# backend
(cd backend && go test -tags=unit ./...)
(cd backend && go test -tags=integration ./...)
(cd backend && golangci-lint run ./...)

# frontend
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend run lint:check
pnpm --dir frontend run typecheck
pnpm --dir frontend run test:run
pnpm --dir frontend run build
```

当前 `.github/workflows/backend-ci.yml` 实际执行 backend unit/integration、frontend lint/typecheck 加关键 Vitest 子集，以及 golangci-lint。`.github/workflows/security-scan.yml` 实际执行 govulncheck 和带例外文件的 production pnpm audit；它当前没有 gosec 或容器扫描。release candidate 仍要按 [`docs/operations/PROJECT_MAINTENANCE.md`](docs/operations/PROJECT_MAINTENANCE.md) 执行完整验证，不能把 CI 子集写成完整发布证明。

## 生成代码

修改 Ent schema：

```bash
cd backend
go generate ./ent
git diff -- ent migrations
```

修改 Wire providers：

```bash
cd backend
go generate ./cmd/server
git diff -- cmd/server
```

生成文件只能由生成器更新。已执行的 SQL migration 不修改 checksum；新变化增加 migration，并完成 N/N-1 和回滚审查。

## 常见工程问题

### Lockfile

修改 `frontend/package.json` 后使用 pnpm 重生成并提交 `pnpm-lock.yaml`。不要混用 npm 生成的 lockfile；CI 安装使用 `--frozen-lockfile`。

### Interface 与 test doubles

Go interface 新增方法后，所有 stub/mock 必须一起补全。优先用 `rg 'type .*Stub|type .*Mock' backend/internal` 查找受影响实现，并运行相关 package tests。

### 模型映射的批量修改

不要同时批量修改不同平台账号的模型白名单/映射。上游模型变化快于默认映射时，可以在单个平台的 canary 账号上增加经过验证的临时透传映射，但必须记录原因、测试和删除条件；先验证真实 API 请求、分组调度与计费，再推广到生产账号池。

### Windows

Windows 本地环境建议优先使用 Docker Desktop/WSL。PowerShell 会解释 `$`，因此 bcrypt、token 等值不要拼进双引号命令；使用参数化脚本或仅限本地、受忽略且权限受控的输入文件。`localhost` 有 IPv4/IPv6 差异时显式使用预期地址。

## Git 边界

普通功能分支从 fork `main` 建立并通过 PR 合并。upstream integration 不使用这里的普通 Git 快捷方式；必须完整执行 [`docs/operations/UPSTREAM_MAINTENANCE.md`](docs/operations/UPSTREAM_MAINTENANCE.md)。尤其禁止：

- 直接 `git merge upstream/main` 到 `main`/release；
- 直接 `git rebase upstream/main` 后覆盖 fork 历史；
- 向 upstream remote push；
- 用当前 `HEAD` 猜测已部署源码。

## PR 最小检查

- 工作区没有混入无关修改或 secret。
- 相关 unit/integration/Vitest 通过。
- lint、typecheck 和 build 与改动匹配。
- `go.mod`/lockfile/生成文件一致。
- schema/config 变化有兼容和回滚结论。
- Frenzy runtime 差异已更新稳定 Patch ID ledger。
- upstream、发布或生产变化已进入各自权威 runbook，不在 PR 描述里临时发明流程。
