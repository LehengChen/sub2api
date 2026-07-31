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
| 新增 migration 清单 | 相对已部署 v0.1.151 新增 25 个 SQL migration 和 3 个 migration 测试文件；相对 v0.1.163 新增 8 个 SQL migration。完整差异留在源码历史，运行时按完整文件名排序。 |
| expand / backfill / contract | 大多数为新增表/nullable 或带默认列和索引（expand）；`175_default_openai_long_context_billing.sql` 含函数、触发器和 accounts backfill，属于行为改变 + 数据回填，必须单独 rehearsal；没有可直接宣称完成的 contract。 |
| 锁表与数据规模 | 未在生产执行；CREATE INDEX CONCURRENTLY 文件不持有普通表级写锁，但耗时、失败残留和长事务风险仍需脱敏生产形状 rehearsal。 |
| migration 可重试性/checksum | runner 已有 filename/checksum、`CheckMigrations`、非事务模式和 invalid-index 重试测试；生产 checksum 尚未读取/ attestation。 |
| N 与 N+1 同时运行 | 仅源码静态审查；新增字段大多旧代码可忽略，但 175 触发器、188/189 request/group live 语义和 184 outbox 需双版本 integration 才能通过。 |
| N+1 写入后 N 可读取 | 未证明。尤其 accounts.extra 触发器、group live 字段、session/cache payload 需 N/N-1 测试。 |
| 仅镜像回滚是否安全 | 否，当前结论为 `database-restore-required-or-maintenance-window`，直到完成 expand/backfill 与旧版本读写测试。 |
| 需要的快照/PITR 与恢复点 | 当前只核实到历史生产恢复能力/恢复点；针对本次 v0.1.169 migration 的 approved pre-migration snapshot/PITR marker 尚未建立，仍需先完成并做脱敏 rehearsal；本次不执行应用 migration。 |

### v0.1.163 → v0.1.169 新增 SQL migration

以下清单来自源码差异，不代表已经在生产执行。runner 按完整 filename 排序，因此
数字前缀相同的文件也有确定顺序（例如 `172_composite...` 排在已有的
`172_video...` 之前）。

| 文件 | 静态影响判断 | 发布门禁 |
|---|---|---|
| `172_composite_model_routes.sql` | 新表、外键和 3 个部分索引；expand。 | 验证现有分组/路由读写、删除级联和旧版本忽略未知表。 |
| `186_alipay_mobile_precreate_deep_link.sql` | 仅插入默认关闭的 setting；行为由显式 opt-in 控制。 | 确认旧版本读取 setting 不失败，默认仍走 legacy WAP。 |
| `186_group_auth_cache_image_generation.sql` | 替换群组 auth-cache 失效触发器函数，新增 image-generation 变化的 outbox 失效事件。 | 双实例验证 UPDATE/DELETE、outbox 幂等、Redis 消费延迟和重复事件。 |
| `187_add_usage_log_session_id.sql` | `usage_logs`/`batch_image_jobs` 增加 nullable `session_id`；预期 metadata-only，但仍需核对表规模和锁等待。 | 生产形状 rehearsal、旧版本读取兼容、敏感值截断/留空检查。 |
| `188_allow_live_usage_request_type.sql` | 重建 request-type check constraint，将允许范围扩展到 `0..5`。 | N/N-1 读写测试，确认旧版本不会拒绝新类型或错误计费。 |
| `189_add_group_allow_live.sql` | `groups.allow_live` 为 `NOT NULL DEFAULT false`。 | 旧管理员写路径和 API DTO 未知字段兼容；确认默认关闭。 |
| `190_add_users_email_alias_dedup_index_notx.sql` | 非事务 `CREATE INDEX CONCURRENTLY` 表达式索引；可能长时间运行并留下 invalid index。 | 独立 migration-only rehearsal、长事务/取消/重试和 invalid-index 清理演练。 |
| `191_passkey_credentials.sql` | 新增 passkey handle/credential 表及索引；不应自动开启功能。 | RP origin、认证回调、Redis session 和双 Center 共享状态验收后才可 opt-in。 |

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
| 工具链/生成文件 | Go 1.26.5 本地运行 Ent/Wire 生成检查；Node/pnpm9 lint/typecheck、完整 Vitest（197 files/1356 tests）和生产构建已通过；`xlsx@0.18.5` 已替换为 `@e965/xlsx@0.20.3`，生产依赖审计结果为 high/critical 0（仍有 low 8、moderate 29）；完整 amd64 镜像与容器扫描仍是 release gate。 |

### 不能从当前静态实现推导的能力

- `WorkerFence.Token()` 当前只在测试/接口层暴露，没有接入关键生产写入的条件更新或数据库约束；`startSingletonWorker` 只在启动时检查 lease。lease 丢失后会触发进程 drain，但不能据此证明所有已启动 worker 已立即停止写入。因此 FZ-007 目前只是启动门禁和失租通知，不是完整 write fencing；只能保留审计的手动 failover，禁止 active-active 或自动故障切换。
- `ValidateResolvedIP` 在 transport 实际拨号前单独做 DNS 查询；当前 HTTP transport 未证明把已校验 IP 固定到 socket，存在 DNS-to-connect TOCTOU。每跳 redirect 校验不能单独宣称已消除代理侧 DNS rebinding，必须用真实网关/出口路径完成审计。

## Patch 处置

详见 [`PATCH_QUEUE.md`](PATCH_QUEUE.md) 的 v0.1.169 表。八项均为 carry/reimplement/recalculate，当前没有任何一项可视为已由 upstream 自动吸收。

## 已完成验证与缺口

```yaml
backend_unit: passed `GOMAXPROCS=2 go test -tags=unit ./...`
backend_integration: not-run against a real PostgreSQL/Redis candidate
wire_ent_generation: passed with Go 1.26.5; generated diff committed
frontend_lint_typecheck: passed
frontend_targeted_tests: passed (34 tests plus capability mock regressions)
frontend_full_test_build: passed (197 files / 1356 tests; production build)
golangci_lint: passed with v2.9.0; 0 issues
govulncheck: passed (0 vulnerabilities in reachable code/imports; 3 required-but-not-called modules remain)
dependency_audit: high/critical 0; low 8, moderate 29; pnpm audit exits non-zero for remaining advisories
container_scan: previous amd64 candidate Inspector scan found one active MEDIUM
  CVE-2026-41178 in go.opentelemetry.io/otel v1.41.0 (fixed in v1.44.0);
  dependency was upgraded in the follow-up security patch and the image must be
  rebuilt and rescanned before promotion
linux_amd64_image: passed for immutable build-only candidate frenzy/candidate/0.1.169-frenzy.1
ci: passed for candidate source f632528563cf57ec7fdbefb7221d1a770ab88cc9
sbom_provenance: BuildKit SBOM and mode=max provenance generated; private registry identity is recorded only in ops
migration_rehearsal: not-run; required before approval
proxy_group_billing_e2e: not-run against real gateway/egress
claude_synthetic: not-run
openai_synthetic: not-run
old_new_coexistence: not-proven
rollback_class: database-restore-required-or-maintenance-window
deployment_strategy: maintenance-window until rolling contract passes
stop_conditions: missing candidate digest, migration rehearsal, real readiness, synthetic, or reviewed ops plan

production_image_contract: root Dockerfile OCI metadata must identify the
  LehengChen fork, full source SHA, v0.1.169 version and UTC build timestamp;
  the production linux/amd64 build overrides POSTGRES_IMAGE to
  postgres:16.14-alpine so migration/backup clients match the production RDS
  major version. This does not change the upstream PostgreSQL 18 development
  compose baseline.
observation_window: not-started
final_decision: candidate superseded by the OpenTelemetry security rebuild; no
  release/tag/promotion until the new immutable digest, scan and migration gates
  are recorded
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
