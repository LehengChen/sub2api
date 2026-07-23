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

历史失败候选 `frenzy/candidate/0.1.163-frenzy.1` 对应完整源码
`713c4999354ab33bc01fc863ed9817cde96ea1d3`。远程 run
[`29929198055`](https://github.com/LehengChen/sub2api/actions/runs/29929198055) 于
2026-07-22 23:33:55 至 23:47:06（Asia/Tokyo）运行：gate、backend、frontend 和
security 成功，其中 backend 使用真实 PostgreSQL/Redis service 完成 unit/integration、
必需共享状态 no-skip gate、race 和 Ent/Wire clean generation；lint 与 image 失败。
lint 暴露 service 直接导入 Redis 和迁移 readiness 的 `rows.Close` 未显式处理；image
暴露带 provenance/SBOM 的 OCI 顶层 index 没有直接 platform descriptor，原 jq 校验
错误地只检查第一层。对应修复为 `d4a8273d1`、`1f2935494` 和 `3bcb8a6b8`，并已用该
失败 run 的真实 OCI archive 只读复验递归校验器。`.1` 保持失败且不可变。

本机没有可用 Docker daemon，因此没有本地伪造镜像构建或 Trivy 结果。`.1` 的 image
job 在平台 evidence 步骤停止，不能视为容器扫描成功。随后 `.2`（源码
`4378e9b2666cdcaf3020f181f923b51fd897bf93`，远程
[`29933005376`](https://github.com/LehengChen/sub2api/actions/runs/29933005376)）的 OCI
构建、递归 platform evidence 和所有非 image job 成功，但 Trivy 发现三个可修复 HIGH：
`CVE-2026-29181`（OpenTelemetry `1.37.0`，fixed `1.41.0`）以及
`CVE-2026-46602`/`CVE-2026-46604`（x/image `0.39.0`，fixed `0.43.0`）。依赖修复
已提交为 `3432dee8e`（stable patch-id
`fc1e3faf1b9269c995384c1ac161192a75a6f9b7`）。`.3` 的 image、security、lint、frontend、
backend unit/integration/race 全部成功，但 Ent/Wire clean generation 因 readonly 模式
缺少 `github.com/google/subcommands` checksum 失败；本地已补齐该显式间接依赖并验证
两次生成幂等，修复提交为 `b7c256cf0`（stable patch-id
`a251baa3cc5b992970ab182f4bbd17b91bb365da`）。不可变 `.4`（源码
`c45bbd468ba983d2306c02744a20920adfe5a109`）的远程 run
[`29936467514`](https://github.com/LehengChen/sub2api/actions/runs/29936467514) 于
2026-07-23 01:06:04 至 01:19:41（Asia/Tokyo）完成，gate、backend、security、image、
frontend、lint 和 summary 全部成功；Trivy HIGH/CRITICAL 计数为 0，真实 PostgreSQL/
Redis integration、必需测试 no-skip、目标 race 和 Ent/Wire clean generation 均通过。

`.4` evidence 明确为 `production=false`、`linux/amd64`，本地 OCI archive SHA-256 为
`d0f5cda227d69241e85c5442bc68a73ac2dd65e0071d5a54962afcd1ef7f2681`，唯一可运行
manifest digest 为 `sha256:81a8c5b8eb7be7bac64cf7e56eaf8e748a02d202b458bdf3699df502c0d9f6e1`。
registry digest 和签名仍明确为 missing；生产 ECR digest、provenance/签名、catalog、
migration rehearsal 和真实 synthetic 仍是后续批准门禁。

最新不可变候选 `frenzy/candidate/0.1.163-frenzy.5` 对应完整源码
`20f1d47e65737cc8476bed277cffc47b3ea48d30`。远程 run
[`29978188921`](https://github.com/LehengChen/sub2api/actions/runs/29978188921) 于
2026-07-23 12:54:40 至 13:08:14（Asia/Tokyo）完成，gate、backend、security、image、
frontend、lint 和 summary 全部成功。`.5` 在 `.4` 的候选构建证据上增加了
v0.1.151 legacy runner transition gate 的 fail-closed 启动测试；该 run 仍标记
`production=false`，没有生产 ECR digest、签名、approved catalog、migration rehearsal、
生产 synthetic 或部署授权。后续引用 v163 candidate 时以 `.5`/该完整 SHA 为准，历史
`.4` 只保留作审计记录。

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

## v0.1.151 Runner Transition Gate

已部署 v0.1.151 时代的 legacy runner 若不向容器传入 deployment catalog 和
`SUB2API_PROCESS_ROLE`，直接换成 v0.1.163 镜像会回落到兼容 standalone 安装的
`self_managed` + `all`。该角色会在应用初始化中执行 migration 和 bootstrap writes，
因此不能被 app-only 发布当作安全的兼容路径。

应用保持 fail-closed：`externally_managed` 与隐式 `all` 的组合拒绝启动，不自动猜测
primary 身份，也不根据 hostname、镜像 tag 或配置文件把 legacy runner 静默提升为
`active`。首次生产激活前必须先完成一个独立、可回滚的 runner-only 过渡，使旧镜像
保持不变但新 runner 能向容器显式传入：

```text
SUB2API_DEPLOYMENT_CONTROL_MODE=externally_managed
SUB2API_PROCESS_ROLE=active
SUB2API_INSTANCE_ID=center-primary
SUB2API_WORKER_LEASE_KEY=sub2api:runtime:primary-worker
SUB2API_WORKER_LEASE_TTL_SECONDS=60
SUB2API_WORKER_LEASE_RENEW_SECONDS=15
SUB2API_MULTI_API_ENABLED=false
```

该 runner-only 变化必须先用 v0.1.151 原镜像验证 image/config/RepoDigest 不变、legacy
`/health` 和认证 synthetic 通过，并保留旧 runner 回滚。v0.1.151 不识别这些新增变量，
所以“变量可透传”不等于它已经具备 external update protection 或 worker fencing；这两项
只能在 v0.1.163 激活后验收。随后仍须先执行独立 migration-only 变化，再由 app-only
激活 v0.1.163；此时 `active` 只核对 migration checksum，不执行 migration/bootstrap，
且必须取得 Redis worker lease 后 `/readyz` 才能成功。

当前应用单测固定了两条边界：legacy `all` 不能进入 external 模式；显式 `active` 可以
接流量但没有 migration/bootstrap write capability。migration readiness 也显式验证旧
migration 集合可以忽略 174-185 的额外记录。它们不替代真实 v0.1.151 二进制在脱敏
v0.1.163 schema 上的持续读写、账号 credential 保留、计费和回滚 rehearsal。

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

最终 commit、stable patch-id、`.1`/`.2`/`.3` 失败证据和 `.4`/`.5` 成功证据已回填
[`PATCH_QUEUE.md`](PATCH_QUEUE.md)。这完成 candidate CI 门禁，不授权 production
promotion。

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
