# v0.1.169 Upstream 集成兼容性记录

状态：`integration`，不可 promotion。观察时区：`Asia/Tokyo`；观察日期：2026-07-31。
本记录描述源码集成和验证边界，不代表生产已更新。生产部署仍必须从私有 ops 的
stable release manifest 闭环 `app tag -> source SHA -> image digest -> config revision -> ops revision`。

## 身份

```yaml
observed_at_utc: 2026-07-31
old_app_tag: frenzy/app/v0.1.151-e316ebf5.1
old_app_sha: 3c39b35c81b8e4664d9110b9e68939c66b263817
old_upstream_base: e316ebf52838a89d57fc790981cce7520f819ac8
target_upstream_tag: v0.1.169
target_tag_object: 830b5f507396b858874b171feae1cbcfce1caded
target_peeled_commit: 26d894ef4f50645a4bf1030e378ac892f17d0223
candidate_sha: not-frozen-integration
integration_branch: integration/v0.1.169-frenzy.1
release_branch: not-created
patch_decisions: FZ-001..FZ-008 documented in PATCH_QUEUE.md
reviewer: Frenzy maintenance agent
```

上游 tag 的 `backend/cmd/server/VERSION` 仍是 `0.1.168`。候选将版本文件显式设为
`0.1.169`，这是标识修正，不是对 upstream tag 内容的隐瞒；release freeze 时必须重新
记录完整候选 SHA。

## Migration 与数据

相对已部署 v0.1.151，目标 tag 增加了 172、174--191 范围的表、列、约束、触发器、
审计/outbox、passkey 和索引 migration（包含同一数字前缀下按完整文件名排序的多个文件）。
当前 runner 以 filename + SHA-256 checksum 记录，`*_notx.sql` 只允许非事务
`CREATE/DROP INDEX CONCURRENTLY`，并在已知 invalid index 情况下清理后重试。

| 检查 | 结论与证据 |
|---|---|
| 新增 migration 清单 | `backend/migrations` 中新增 28 个 SQL/迁移测试文件；完整差异留在源码历史，运行时按文件名排序。 |
| expand / backfill / contract | 大多数为新增表/nullable 或带默认列和索引（expand）；`175_default_openai_long_context_billing.sql` 含函数、触发器和 accounts backfill，属于行为改变 + 数据回填，必须单独 rehearsal；没有可直接宣称完成的 contract。 |
| 锁表与数据规模 | 未在生产执行；CREATE INDEX CONCURRENTLY 文件不持有普通表级写锁，但耗时、失败残留和长事务风险仍需脱敏生产形状 rehearsal。 |
| migration 可重试性/checksum | runner 已有 filename/checksum、`CheckMigrations`、非事务模式和 invalid-index 重试测试；生产 checksum 尚未读取/ attestation。 |
| N 与 N+1 同时运行 | 仅源码静态审查；新增字段大多旧代码可忽略，但 175 触发器、188/189 request/group live 语义和 184 outbox 需双版本 integration 才能通过。 |
| N+1 写入后 N 可读取 | 未证明。尤其 accounts.extra 触发器、group live 字段、session/cache payload 需 N/N-1 测试。 |
| 仅镜像回滚是否安全 | 否，当前结论为 `database-restore-required-or-maintenance-window`，直到完成 expand/backfill 与旧版本读写测试。 |
| 需要的快照/PITR 与恢复点 | 生产 PITR 恢复点已建立，但 v0.1.169 migration 前仍需新的 approved snapshot/PITR marker 和脱敏 rehearsal；本次不执行应用 migration。 |

### 数据风险重点

- `175_default_openai_long_context_billing.sql` 会更新 accounts.extra、创建触发器并写入 scheduler_outbox；不能和应用镜像替换绑定成一次不可回滚操作。
- `180`/`181`/`182` 会增加审计和 prompt-audit 数据面；retention、访问角色、PITR 暴露和容量预算未获批准，功能不得默认启用。
- `190..._notx.sql` 必须在独立 migration-only 窗口执行，并验证长事务、重复索引和恢复重试。

## 配置与接口逐项审查

| 领域 | 当前结论 |
|---|---|
| 应用配置/环境变量/部署模板 | upstream 改动已审查；候选额外保留 externally-managed release、process role、health/drain 和 offline pricing 配置。生产配置由私有 ops catalog 渲染，公开默认值不写入生产专属主机。 |
| API/JWT/API Key/session | API 新增能力需兼容旧 JWT/API key；OAuth state/session 改为 Redis 共享，但跨实例 start/callback 尚未做真实演练。 |
| OAuth、账号 credentials/extra | upstream 有 OAuth/账号行为变化；FZ-006 保持外部 session，账号 extra 新字段按 N/N-1 读写审查；不修改生产账号数据。 |
| Redis key/cache/queue payload | worker lease、OAuth session、cache invalidation outbox 共享 Redis；旧/新 key 和 TTL 需双版本测试，不能以 sticky session 掩盖。 |
| 模型/价格/计费/余额/订阅 | upstream pricing fallback 和本地 offline pricing 都变更；必须做离线定价、Claude/OpenAI synthetic、usage/billing/idempotency 验收。 |
| 分组/调度/固定代理 | composite route、group policy、scheduler snapshot 和固定出口绑定需保持；出口扩容是独立 infra 轴，不随应用 release 自动发生。 |
| 前端静态资源与旧后端 | Version 页面支持外部运维模式、缓存/失败 warning；旧后端 API 兼容需执行前端 build 与浏览器 smoke。 |
| 后台 cron/worker/leader lock | FZ-007 只提供 lease/fencing 门禁；所有单例任务仍需逐项标注幂等性和 fencing 证据，自动 failover 关闭。 |
| SSE/WebSocket/HTTP2/TLS | bounded drain 已实现；WebSocket 只有注册到长连接 registry 的 handler 可纳入排空，不能宣称绝对无中断；真实 ALB 测量待做。 |
| upstream URL/redirect/DNS | FZ-008 每跳重新校验 allowlist、HTTPS、端口、userinfo 和私网 DNS；未列出的 host 必须拒绝，需真实网关/出口测试。 |
| 工具链/生成文件 | Go 1.26.5 本地运行 Ent/Wire 生成检查；Node/pnpm9 lint/typecheck/targeted tests 已通过；完整 amd64 镜像、govulncheck、容器扫描仍是 release gate。 |

## Patch 处置

详见 [`PATCH_QUEUE.md`](PATCH_QUEUE.md) 的 v0.1.169 表。八项均为 carry/reimplement/recalculate，当前没有任何一项可视为已由 upstream 自动吸收。

## 已完成验证与缺口

```yaml
backend_unit: passed targeted service, server, repository, cmd/server; full ./... pending
backend_integration: not-run against a real PostgreSQL/Redis candidate
wire_ent_generation: passed with Go 1.26.5; generated diff committed
frontend_lint_typecheck: passed
frontend_targeted_tests: passed (34 tests)
frontend_full_test_build: pending build/full suite
golangci_lint: not-run
govulncheck: not-run
dependency_audit: not-run
container_scan: blocked locally by Docker socket permission; no production claim
linux_amd64_image: not-built locally; CI/ECR evidence required
migration_rehearsal: not-run; required before approval
proxy_group_billing_e2e: not-run against real gateway/egress
claude_synthetic: not-run
openai_synthetic: not-run
old_new_coexistence: not-proven
rollback_class: database-restore-required-or-maintenance-window
deployment_strategy: maintenance-window until rolling contract passes
stop_conditions: missing candidate digest, migration rehearsal, real readiness, synthetic, or reviewed ops plan
observation_window: not-started
final_decision: candidate only; no release/tag/promotion
```

## 下一步与回滚

1. 冻结并审查候选源码 SHA，完成 full tests、security scans、amd64 digest/provenance。
2. 在脱敏生产形状数据库做 migration rehearsal，记录耗时/锁/回滚类别；必要时拆出
   175 backfill 和 190 concurrent index 为独立 change。
3. 在私有 ops 暂存 immutable ECR tag/digest，做 Terraform plan、预拉取、readiness
   和低成本 synthetic；不使用 `latest` 或 upstream/main。
4. 只有所有证据齐全且获得单独批准后，才可选择维护窗口或人工主备切换。失败时先隔离
   candidate、恢复 stable app/config pointer；若 schema 已改变，按 PITR/数据库恢复流程，
   不把“换回旧镜像”误当成安全回滚。
