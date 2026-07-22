# Upgrade Compatibility: v0.1.163

观察时间：2026-07-23（Asia/Tokyo）

状态：`integration candidate`，未批准、未构建生产镜像、未部署。

## Candidate Verification Snapshot

截至 2026-07-23（Asia/Tokyo），本工作区已完成以下只读候选验证：Go unit（含
`-tags=unit ./...`）、定向 race（runtime/server 全包、service 契约及 repository Redis
adapter）、Ent/Wire 二次生成洁净检查、`golangci-lint v2.9.0`、
`govulncheck@v1.1.4`、`linux/amd64` 编译、前端 lint/typecheck/Vitest/build、pnpm audit
现有例外门禁、workflow policy、Terraform validate/test（私有 ops 27/27）和新增
shell/controller 测试。

不可变候选 `frenzy/candidate/0.1.163-frenzy.1` 对应完整源码
`713c4999354ab33bc01fc863ed9817cde96ea1d3`。远程 run
[`29929198055`](https://github.com/LehengChen/sub2api/actions/runs/29929198055) 于
2026-07-22 23:33:55 至 23:47:06（Asia/Tokyo）运行：gate、backend、frontend 和
security 成功，其中 backend 使用真实 PostgreSQL/Redis service 完成 unit/integration、
必需共享状态 no-skip gate、race 和 Ent/Wire clean generation；lint 与 image 失败。
lint 暴露 service 直接导入 Redis 和迁移 readiness 的 `rows.Close` 未显式处理；image
暴露带 provenance/SBOM 的 OCI 顶层 index 没有直接 platform descriptor，原 jq 校验
错误地只检查第一层。对应修复为 `d4a8273d1`、`1f2935494` 和 `3bcb8a6b8`，并已用该
失败 run 的真实 OCI archive 只读复验递归校验器。`.1` 保持失败且不可变；新的 `.2`
在全部远程 job 成功前仍是待验证候选。

本机没有可用 Docker daemon，因此没有本地伪造镜像构建或 Trivy 结果。`.1` 的 image
job 在平台 evidence 步骤停止，不能视为容器扫描成功；生产 ECR digest、provenance、
签名和 catalog 仍是后续批准门禁。

## 身份闭环

| 事实 | 值 |
|---|---|
| deployed upstream base | `e316ebf52838a89d57fc790981cce7520f819ac8` |
| target upstream tag | `v0.1.163` |
| target tag object | `bb752ef7776dc126ffca5df9188087d0d0aed559` |
| target peeled commit | `d0bdd7e771636a8d315f542cafd39484f39bd60c` |
| tag timestamp | `2026-07-22T15:18:47+08:00` |
| target ancestry | deployed base is an ancestor |
| tag signature | missing; annotated but unsigned |
| exact diff | 1067 files, +134972/-6522 |

目标只从 `refs/tags/upstream/v0.1.163` 创建，不包含 `upstream/main`。缺失 upstream tag 签名必须作为 provenance 缺口进入私有 release catalog；不能把 annotated tag 等同于已验证签名。

## Migration

目标新增 17 个 SQL migration：

```text
174_add_usage_log_long_context_billing.sql
174_add_usage_logs_api_key_latest_ip_index_notx.sql
174_group_web_search_price_per_call.sql
175_add_ops_system_logs_host.sql
175_default_openai_long_context_billing.sql
175a_add_ops_system_logs_host_index_notx.sql
176_channel_monitor_grok_provider.sql
177_add_subscription_plan_currency.sql
178_channel_image_input_price.sql
179_usage_log_image_input_tokens.sql
180_audit_logs.sql
181_group_duplicate_operation_id.sql
181_prompt_audit.sql
182_prompt_audit_full_prompt.sql
183_ops_ingress_reject_aggregates.sql
184_auth_cache_invalidation_outbox.sql
185_group_reasoning_effort_policy.sql
```

高风险点包括大表 backfill/trigger、check constraint 重建、非并发 unique index、durable cache invalidation outbox，以及 prompt audit 数据保留语义。仓库内部分测试只验证 SQL 文本，不证明生产规模的锁等待和耗时。

处置：应用 candidate 已拆出 `--migrate`/`migrator`；其他显式角色只做 checksum readiness。promotion 前仍必须在脱敏恢复快照上 rehearsal，记录耗时、锁、取消、重试、磁盘增长、checksum 和 N/N-1 行为。migration、app activation 和运行模式变更不得同窗。

## Compatibility Matrix

| 领域 | v0.1.163 变化/风险 | 当前处置 | promotion gate |
|---|---|---|---|
| Config | 新增大量 gateway、audit、reasoning、Redis ACL 等设置；upstream 安全默认不等于生产期望 | 私有配置显式固定 allowlist、private/insecure、forwarded-IP 和 trusted proxies | config render 负向测试、candidate/stable diff |
| API/frontend | 139 个 route/handler 文件变化，新增 audit、prompt、billing probe、async media 和 responses alias | 使用同一 candidate 构建前后端 | API contract、旧/新前后端 skew 测试 |
| OAuth/credential | 五种 OAuth 临时会话原为进程内；旧版本管理写可能覆盖新 credential 字段 | candidate 改为共享 Redis 一次性 session | N/N+1 start/callback、credential 保留测试 |
| Models/billing | long-context、image input、web search、reasoning policy、Grok 等口径变化 | 保留离线 pricing patch | golden usage/billing replay 与低成本真实 synthetic |
| Groups/scheduler | reasoning policy、duplicate operation、snapshot/outbox 变化 | scheduler 首次 rebuild 纳入 readiness；单 active worker lease | outbox backlog、cache rebuild、failover 测试 |
| Redis/cache | API cache snapshot v14→v16，共享 key 可能在混合版本反复 miss | 禁止未经验证 active-active；只做先隔离旧主的人工切换 | N/N-1 cache hit/miss/TTL/rollback 测试 |
| Concurrency | upstream 启动会删除其他进程 prefix 和 wait counter | candidate 删除自动 startup sweep，保留 TTL 驱动的周期清理 | 双进程 in-flight slot/waiter 测试 |
| SSE/WebSocket | 长流不可跨实例续传 | readiness/drain 有界等待；超时后明确中断 | ALB deregister + HTTP/SSE/WS 实测并记录 |
| Proxy/TLS | TLS fingerprint、H2 fallback、代理协议路径变化 | redirect 每跳重新校验 allowlist/scheme/private DNS；FZ-001 HTTPS-only | HTTP/HTTPS/SOCKS、三出口、无直连旁路测试 |
| Toolchain | Go `1.26.5`，Node `24`、pnpm `9.15.9` 和前端锁文件变化 | 本地与候选 Dockerfile/CI 固定相同版本 | unit/integration/generate/lint/typecheck/audit/build/scans |
| Rollback | migration triggers/outbox/新字段可能留给旧应用；175a 并发索引中断可能留下 invalid index | runner 已为 175a 增加 invalid-index 清理后再重试；N/N-1 checksum 允许额外新 migration，但业务兼容仍未证明 | rehearsal 证明 old app read/write，或声明 forward-only；175a 重试测试必须通过 |
| Prompt audit data | migration 182 增加完整 prompt 持久化，可能进入备份/PITR | candidate 只迁移 schema，不自动启用功能；安全控制单独登记 | retention、访问审计、删除和备份暴露策略获批后才启用 |

## Patch Queue Decision

| Patch ID | 结论 | v0.1.163 证据 |
|---|---|---|
| FZ-001 | `reimplement` | upstream 默认 proxy probe 仍使用 HTTP；candidate 保留 v163 响应上限并恢复 HTTPS-only target/parser |
| FZ-002 | `reimplement` | upstream pricing 仍可远程刷新；candidate 恢复 isolated/offline 开关和 bundled fallback |
| FZ-003 | `recalculate` | 禁止重放旧 `go.sum`；必须对 v163 当前依赖运行 govulncheck、pnpm audit 和容器扫描 |
| FZ-004 | `new` external release control | AWS 不允许应用内替换/rollback/restart；只读 catalog identity |
| FZ-005 | `new` health/drain | `/livez`、真实 `/readyz`、migration/scheduler/fence checks 和 bounded drain |
| FZ-006 | `new` shared OAuth session | Claude/OpenAI/Gemini/Antigravity/Grok 使用 Redis TTL 和原子 consume |
| FZ-007 | `new` multi-center runtime | 显式角色、migration-only、worker lease 和冷 standby fail-closed |
| FZ-008 | `new` redirect revalidation | redirect 每一跳重新执行 scheme/host/private-IP policy |

最终 commit、stable patch-id 和 `.1` 失败证据已回填
[`PATCH_QUEUE.md`](PATCH_QUEUE.md)；`.2` 成功证据仍待远程运行。

## Required Evidence

- 脱敏快照 migration rehearsal 和独立生产 migration change；
- Go unit/integration、race、Wire/Ent clean generation；
- frontend lint、typecheck、Vitest、build；
- govulncheck、Go module/pnpm audit、容器扫描；
- `linux/amd64` OCI build、不可变 digest、provenance/SBOM，签名或明确缺失项；
- exact allowlist、unlisted host、HTTP downgrade、private DNS、redirect 和真实三出口 synthetic；
- N/N-1 OAuth、credential、cache、scheduler、usage/billing 和 rollback；
- ALB deregistration、HTTP/SSE/WebSocket drain、认证 synthetic、实际中断记录；
- 私有 release catalog 的 app tag、完整源码 SHA、image digest、config revision 和 ops revision。

候选 CI 对 PostgreSQL/Redis 共享状态集成测试提供临时服务，并要求关键测试明确
`pass`；缺少 DSN 或 `skip` 不再被视为成功。镜像扫描使用固定版本 Trivy，build-only
OCI evidence 仍明确记录 registry digest/signature 缺失，不伪造生产证明。

任何一项硬门禁未完成时，本 candidate 不能 promotion，也不能据此创建或注册生产 standby。
