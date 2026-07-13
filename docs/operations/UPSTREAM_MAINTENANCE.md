# Fork、Upstream 与应用发布维护

目标是让本地定制保持为小而清晰的 patch queue，并让每次生产版本都能追溯到明确的 upstream tag、fork revision、镜像 digest 和私有配置 revision。

## 仓库角色

```text
upstream  https://github.com/Wei-Shaw/sub2api   只读上游
origin    https://github.com/LehengChen/sub2api fork
```

`upstream` 的 push URL 应保持禁用。私有 AWS 代码属于 `LehengChen/sub2api-ops`，不能提交到公开应用 fork。

## 截至 2026-07-13 的基线快照

- 当前部署分支：`release/e316ebf5-frenzy.1`
- upstream base：`e316ebf5`，对应 v0.1.151 时期
- fork release tag：`frenzy/app/v0.1.151-e316ebf5.1`
- 已部署源码 tag 在 base 上有 3 个本地代码提交
- upstream 最新正式 tag：`v0.1.153`
- `upstream/main`：`7d239d62`，比 v0.1.153 再前进 5 个提交
- 已部署源码 revision 相对 `upstream/main`：本地 3 个、上游 85 个提交
- release 分支在已部署 revision 之后还有运维文档提交；这些提交没有进入当前运行镜像

这只是观察快照。开始升级前必须重新 fetch，并分别计算“已部署源码 tag”和“目标集成分支”的差距；不能把后续纯文档提交误认为已部署应用代码。

## 当前本地 patch queue

| Commit | 目的 | 下一次升级处理 |
|---|---|---|
| `1f2caaba` | 代理出口探测仅使用 HTTPS | 检查 upstream 是否已等价实现；否则保留并争取贡献 upstream |
| `c9be8c6e` | 隔离环境的离线 pricing 模式 | upstream pricing 代码已变化，需按新结构重新审查，禁止盲目 cherry-pick |
| `3c39b35c` | 运行时依赖安全升级 | 重新扫描新 upstream；若已包含等价版本则删除本地补丁 |

长期目标是把 patch queue 缩到最小。通用修复进入 upstream 后，在下一个 fork release 中删除本地版本。

## 分支和 tag 规范

建议：

```text
integration/v<upstream>-frenzy.<n>   临时集成和测试
release/<upstream>-frenzy.<n>       已批准的生产源码
frenzy/app/v<upstream>-frenzy.<n>    不可变应用 tag
```

规则：

- deployed release 分支和 tag 不 rebase、不 force-push。
- 不在生产 release 分支直接开发。
- 不把 `upstream/main` 直接 merge 到生产；优先选择正式 upstream tag。
- 一个 release 只做一种升级：upstream 应用升级与 `RUN_MODE`/基础设施迁移分开。

## Upstream 升级流程

### 1. 只读盘点

```bash
git fetch upstream --prune --tags
git fetch origin --prune
git status --short
git rev-list --left-right --count HEAD...upstream/main
git log --oneline --decorate HEAD..upstream/main
git cherry upstream/main HEAD
```

确认工作区无意外修改，并记录目标 upstream tag 的 release notes、安全修复和迁移文件。

### 2. 从明确 tag 创建集成分支

示例：

```bash
git switch --create integration/v0.1.153-frenzy.1 v0.1.153
```

不要先 merge 当前 release；先逐个判断三个本地补丁是否仍需要。需要的补丁以独立 commit 重放；已被 upstream 吸收或不再适用的补丁直接删除。冲突解决后使用 `git range-diff` 审查语义，而不只看是否能编译。

```bash
git range-diff <old-base>..<old-release> v0.1.153..<integration-branch>
```

### 3. 迁移与兼容性审查

重点检查：

- `backend/migrations/` 新增文件及是否为 forward-only；
- `backend/go.mod`、`go.sum` 和 Go toolchain；
- `frontend/pnpm-lock.yaml`；
- OAuth、模型列表、计费和调度变更；
- 代理处理、TLS/HTTP2、WebSocket 和 SSE；
- 配置项新增、删除或默认值变化；
- 现有本地 patch 是否与新实现重叠。

数据库 migration 只能前进时，旧镜像回滚不一定安全。发布前必须明确“仅镜像回滚”是否成立；否则回滚方案必须包含数据库恢复点。

### 4. 应用验证

最低门槛：

```bash
cd backend
go test -tags=unit ./...
go test -tags=integration ./...
golangci-lint run ./...

cd ../frontend
pnpm install --frozen-lockfile
pnpm run lint:check
pnpm run typecheck
pnpm run test:run
pnpm run build
```

另需执行与本地补丁和生产链路相关的定向测试：

- HTTPS-only proxy probe；
- offline pricing；
- Anthropic Setup Token 账号测试；
- 三个 OpenAI OAuth 账号测试；
- API Key → 分组 → 调度 → 固定代理的 E2E；
- 数据库迁移副本验证。

### 5. 安全与供应链

- 使用 fork CI；不能把“没有 check”当作通过。
- 执行 `govulncheck`、`gosec`、依赖审计和容器扫描。
- 构建上下文必须来自 clean、已提交的 revision。
- 镜像必须为 `linux/amd64`、不可变 tag，并记录 digest。

### 6. 发布边界

应用仓库负责产生：

- fork release commit/tag；
- 测试和扫描证据；
- 可复现应用镜像。

私有 ops 仓库负责：

- 把镜像推入私有 ECR；
- Terraform/SSM 激活明确 tag；
- rollout、smoke 和回滚；
- 写入 release manifest：upstream base、fork revision、image digest、config revision、验证结果。

发布顺序和生产限制见私有 `infra/docs/RELEASE_RUNBOOK.md`。

## Fork CI 与保护策略

至少应配置：

- fork 的 backend、frontend、lint 和 security workflow 可运行；
- `main` 和 release 分支禁止 force push；
- 合并需要 CI checks；
- production tag 不允许覆盖；
- 私有 ops `main` 也需要 Terraform fmt/validate/test 和 secret scan。

上游 `release.yml` 会针对 `v*` tag 执行上游发布逻辑，甚至可能写回默认分支，不应原样用于 fork 的 `frenzy/*` 发布。fork 应使用独立、只构建不反写源码的 release workflow。

## 回滚原则

- 保留至少前两个已验证应用镜像 digest。
- 没有 schema 变化且兼容时，可回滚镜像并复跑 smoke。
- 有不兼容 migration 时，停止写流量，恢复发布前数据库快照/PITR，再恢复旧镜像。
- 回滚后也要产生新的 ops 记录，不能静默修改 SSM 参数或复用旧 manifest。

## 维护节奏

- 每周：fetch upstream、审查新 release/security 变更，不自动部署。
- 每次 release：重新计算 ahead/behind 和 patch queue，不沿用旧数字。
- 每月：验证恢复流程、旧镜像可用性、依赖扫描和 fork branch protection。
- 每季度：检查本地补丁是否可删除或贡献 upstream。
