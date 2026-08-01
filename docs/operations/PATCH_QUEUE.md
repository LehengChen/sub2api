# Frenzy Runtime Patch Queue

本文件只记录当前已部署应用相对 upstream base 的运行时差异。纯文档、CI 和 ops 提交不计入 patch queue。每个差异使用稳定 Patch ID；commit SHA 会在 reimplement/cherry-pick 后变化，Patch ID 与行为不变量不变。

## v0.1.169 集成处置（2026-08-02，Asia/Tokyo）

以下结论针对 `refs/tags/upstream/v0.1.169`（peeled
`26d894ef4f50645a4bf1030e378ac892f17d0223`），不是生产批准。当前 integration
已固定 immutable source tag；镜像 digest 和私有 ops release manifest 仍须独立闭环，
任一缺失都禁止 production activation。`original_commit` 保留历史来源，下面同时记录本次候选中实际承载行为的
commit 和可重复计算的 stable patch-id。

| Patch ID | 处置 | applied commit / stable patch-id | 证据/理由 | 删除或重审条件 |
|---|---|---|---|---|
| FZ-001 | `reimplement` | `22fbd18ef90607facfa2782c7b40982a820549a9` / `b676c2495ff399d2ae5b1fe2b4fdf0fa01a61204` | upstream v0.1.169 没有 Frenzy 的出口 HTTPS-only probe 约束；保留 fail-closed probe 与定向测试。 | upstream 提供等价 HTTPS-only 行为并通过固定出口 synthetic 后重新计算。 |
| FZ-002 | `reimplement` | `081d44d7f8647771df6ca91ffdf860131c39f81d` / `c28d9bb835a86e352b1d04277ec7a0dc925fe2a0` | upstream pricing fallback 资源已变化，不能机械 cherry-pick；候选保留无公网定价模式和离线测试。 | upstream 提供完整、可审计且不要求 Center 直连公网的定价同步。 |
| FZ-003 | `recalculate` | `58ece1bceca43a044a7f130cb49fbde525e7d53a` / `2d7a2ca2a67b3b5fddd772a1601c7f79e2c46da0`；`2121caba55d254b82d251d7e02a6b3ea1c276a6f` / `4bb46028f20708712f77a0411c4ad76a0ebcc667`；`615a3a923c54e8c91f7fa0f73a09e3377b61a5c7` / `643fd0768cb6f2ef3b105ffe6d380018df3e273f`；`e306ebdf5466a354ee6ef28cff02e030f80d7963` / `614268256ee56463b99850e4147c2933dc9cf44c`；`dd73f1d7f85dbb9083d7342c343a13090b2b1f20` / `b16d37203c1fcaca2f96d320d6d1613376cad59e`；`f8c285dbd594101c9941a6270507c8dc328a2593` / `57e6c691bf1fd6cf76c84aac1d41765cd9ea8b7b` | v0.1.169 已更新部分依赖；候选重新计算 Go/Node 依赖，移除高危 `xlsx@0.18.5` 并替换为 `@e965/xlsx@0.20.3`，将 Inspector 报告的 `go.opentelemetry.io/otel` 中危漏洞从 v1.41.0 升级到 v1.44.0，并补齐 migration 190 concurrent index 的 invalid-index 清理/重试。候选不能把旧 lockfile、旧镜像扫描或 mock migration 测试当永久证据。 | govulncheck、依赖审计和镜像扫描绑定新的候选 digest 后，逐项 drop 或重写；migration runner 重试逻辑需保留生产形状演练。 |
| FZ-004 | `reimplement` | `e35b16c9a91f7d341132c7668494423a9c09e85d` / `707bb8053c5d4bcb43439c949c90b55138bf8163`；`3b905270147e80507f34b6e86b2b9f537fe8a1b2` / `48c693b8ffa76f1fcd29b2ae5d8365d39d5c4456` | AWS/私有 catalog 是外部运维边界；版本检查可以只读，但应用内替换/回滚在 externally managed 模式必须禁用。 | 私有 ops catalog、digest/provenance 和外部 controller 已获批并覆盖同等能力后重审。 |
| FZ-005 | `reimplement` | `281d7517f9612eb97b290ef7790c7051b45edeb5` / `36c5ccf56825d4ca38ba7dcc8cf49199e6a7b29a`；`27c5c6605fd582cff31ec871136d2856347c8f73` / `90941563989f468d721fd5d4783058bcf205ab6c`；`569771c4149c5f4efce2727a4881472e7347824a` / `627424b972656c56c69654c34877e1668de62480`；`4cfdbfce60284b619227433b3da78051a5cac8a1` / `aa68439ae75fa19a9eab66a3c41dd2ec4bd69035`；`b911db348e65134b9a59219c96fba92c7a3929f8` / `d77ecca5a18d3a33ddaa57b4503f62540fd21155` | `/livez`、真实 `/readyz`、bounded drain、hijacked WebSocket 强制关闭、usage/billing/quota 有序 flush、migration readiness 和部署层 probe 分工是多 Center 前置条件；embedded frontend 必须明确绕过 lifecycle probes，不能由 upstream 健康路径或 SPA fallback 替代。 | upstream 等价实现完成真实 ALB/SSE/WebSocket/认证 synthetic 验收后重审。 |
| FZ-006 | `reimplement` | `6b0bf99da9380cca4f3c45d9a36b29c425c72149` / `a0bedb66b091548f3d24a1573e0aff53997c2e1e` | OAuth/session 必须跨 Center 共享；候选使用受控 Redis store，兼容性仍需双实例 start/callback 演练。 | upstream 提供等价外部化 session 且 N/N-1 测试通过。 |
| FZ-007 | `reimplement` | `8ef0277d0a18cc38d20a4783385c6c7de8f6e781` / `70245a28a377d3c1aaedaf3359950f48a2c1c482`；`15218a20ccaefa4110725406858b9025324b92ed` / `928c5f9742f3a16cfac42b595cfa295db34e60b0` | 显式 process role、worker lease/fencing、migration-only 启动和 standby startup worker gate 是 Frenzy 运行边界；当前只允许人工主备，不开启 active-active。 | 所有关键写路径 fencing、共享状态和故障演练通过后才可扩大能力。 |
| FZ-008 | `reimplement` | `f37abee8b1ccb2fcd5746690b7fbd68df3ed0ee4` / `14c320693cd8eedd3267f3eca38731567e1b1eac` | 每一跳 redirect 都重新执行 scheme、host allowlist、userinfo、端口和私网 DNS 校验；安全约束不能依赖首跳。 | upstream 等价实现并完成网关转发与 DNS TOCTOU 审计。 |
| FZ-009 | `reimplement` | `af23695b003d35286d208fc8e07a3aa247779b0c` / `af8841ca825c84bfc7a302176377036745ba7708`；`c68b4c8b82d2e8005a02de8ec540589c07272ecb` / `195e1d150c6247ba2281b4db1248a1d3da501573` | 应用、GoReleaser 和开发 Dockerfile 固定 Dockerfile frontend、Node、Go、Alpine、PostgreSQL multi-architecture digest，并把根构建 pnpm 固定为 9.15.9；entrypoint mode 显式固定为 `0755`，不再受构建 context umask 影响；生产 publisher 仍需在私有 ops 显式覆盖和记录获准 digest。 | upstream 提供等价 immutable build-input policy；Alpine package snapshot、provenance statement policy 和签名仍须另行完成。 |

### 集成附加提交

- `615a3a923c54e8c91f7fa0f73a09e3377b61a5c7` / `643fd0768cb6f2ef3b105ffe6d380018df3e273f`：非事务并发索引失败后的 invalid-index 清理/重试（FZ-003）。
- `f8c285dbd594101c9941a6270507c8dc328a2593` / `57e6c691bf1fd6cf76c84aac1d41765cd9ea8b7b`：补齐 migration 190 email alias concurrent index 的同等清理/重试（FZ-003）。
- `569771c4149c5f4efce2727a4881472e7347824a` / `627424b972656c56c69654c34877e1668de62480`：登记所有现有 WebSocket upgrade、强制断连并按 usage/billing/quota 顺序收尾（FZ-005）。
- `4cfdbfce60284b619227433b3da78051a5cac8a1` / `aa68439ae75fa19a9eab66a3c41dd2ec4bd69035`：容器 liveness 固定使用 `/livez`，Compose/Caddy 流量健康固定使用 `/readyz`，并保留 `/health` 兼容别名（FZ-005）。
- `b911db348e65134b9a59219c96fba92c7a3929f8` / `d77ecca5a18d3a33ddaa57b4503f62540fd21155`：embedded frontend 明确放行 `/livez`、`/readyz`，防止真实 handler 被 SPA HTML 截获（FZ-005）。
- `15218a20ccaefa4110725406858b9025324b92ed` / `928c5f9742f3a16cfac42b595cfa295db34e60b0`：standby 构造路径不启动后台 worker，并扩展 WorkerFence 失租测试（FZ-007）。
- `af23695b003d35286d208fc8e07a3aa247779b0c` / `af8841ca825c84bfc7a302176377036745ba7708`：基础镜像和 pnpm 构建输入固定到已核验版本/digest，降低同一源码重建漂移；未把 APK repository 描述为 hermetic（FZ-009）。
- `c68b4c8b82d2e8005a02de8ec540589c07272ecb` / `195e1d150c6247ba2281b4db1248a1d3da501573`：两个生产 Dockerfile 将入口脚本权限确定性固定为 `0755`（FZ-009）。
- `8ef96178887753cfecb925addc6ab10d661af3fb` / `dc5a4ba48482be28e8348e530db37e16fd051397`：Wire/Ent 生成输出和回滚 API timeout 测试同步；生成器重跑已通过。
- `backend/cmd/server/VERSION` 从 upstream tag 内的 `0.1.168` 规范化为 `0.1.169`，避免控制台把正式 v0.1.169 误报为旧版本；该变更必须随候选源码 SHA 一起审查。

## 已部署基线

观察日期：2026-07-13

| 项目 | 值 |
|---|---|
| upstream base | `e316ebf52838a89d57fc790981cce7520f819ac8` |
| deployed source | `3c39b35c81b8e4664d9110b9e68939c66b263817` |
| deployed tag | `frenzy/app/v0.1.151-e316ebf5.1` |
| runtime patch count | 3 |

release 分支在 deployed source 之后还有运维文档提交。它们没有进入当前运行镜像，也不计入本表。

## FZ-001：HTTPS-only exit probe

```yaml
id: FZ-001
order: 10
status: carry-review
original_commit: 1f2caaba7dd392c4862bab84d1fa754c52bb3c13
stable_patch_id: e51f285970d28c1af4f28753082d9f1b7d0265df
last_reviewed_against: e316ebf52838a89d57fc790981cce7520f819ac8
upstream_issue_or_pr: none
```

- Intent：代理出口探测只访问 HTTPS，符合出口节点只允许 TCP/443 的 fail-closed 策略。
- Invariant：任何 quality/connectivity probe 都不能要求出口放开明文 HTTP。
- Paths：`proxy_probe_service.go` 及测试。
- Tests：代理基础连通；OpenAI/Anthropic/Gemini quality probes；非 HTTPS 目标拒绝；真实固定代理 synthetic。
- Drop condition：upstream 提供经过相同测试的等价 HTTPS-only 行为，或生产出口策略不再依赖该约束。
- Next sync：优先贡献 upstream；未吸收时按目标结构审查后 carry/reimplement。

## FZ-002：isolated offline pricing

```yaml
id: FZ-002
order: 20
status: carry-review
original_commit: c9be8c6e2bfe9e650b46db96b8e19326d5d0ebf6
stable_patch_id: 3cec252950d3b632eb5dc98996e5de0c516dcfe3
last_reviewed_against: e316ebf52838a89d57fc790981cce7520f819ac8
upstream_issue_or_pr: none
```

- Intent：Center 无直接公网出口时使用镜像内定价文件，并关闭后台远程更新。
- Invariant：应用可在无直连公网环境启动和计费，不因价格刷新绕过固定出口边界。
- Paths：pricing config/service/tests 与配置示例。
- Tests：offline 启动；模型价格覆盖；真实 Claude/OpenAI 计费；确认无 Center 直连公网。
- Drop condition：upstream 原生支持完整、可测试的离线 pricing，或部署架构提供获准且受控的价格同步通道。
- Next sync：upstream pricing 结构变化时重新实现，禁止盲目 cherry-pick。

## FZ-003：runtime dependency security

```yaml
id: FZ-003
order: 30
status: recalculate
original_commit: 3c39b35c81b8e4664d9110b9e68939c66b263817
stable_patch_id: 832b2eb8f0baacd9be76a430d62fa9ee45b917dd
last_reviewed_against: f632528563cf57ec7fdbefb7221d1a770ab88cc9
upstream_issue_or_pr: none
```

- Intent：修复 deployed base 当时的运行依赖漏洞并对齐 Go modules。
- Invariant：生产 candidate 不包含未接受、未到期例外覆盖的可利用高危/严重运行依赖漏洞。
- Paths：`backend/go.mod`、`backend/go.sum`。
- Tests：unit/integration、编译、govulncheck、容器 Inspector。
- 2026-08-01 Inspector 对候选 amd64 工件发现 `CVE-2026-41178`（MEDIUM，`go.opentelemetry.io/otel` v1.41.0，修复版本 v1.44.0）；本次候选已升级 core/metric/sdk/trace 到 v1.44.0，并需重建、重新扫描和重新记录 digest。
- Drop condition：目标 upstream 依赖树已包含等价或更高修复，且重新扫描通过。
- Next sync：对目标 upstream 重新计算依赖修复，不永久重放这个 lockfile patch。

## 同步时的允许结论

| 结论 | 含义 | 必须证据 |
|---|---|---|
| `drop-upstreamed` | upstream 已有等价或更完整实现 | upstream commit、行为 diff、定向测试 |
| `reimplement` | 需求仍存在，目标结构已变化 | 新 commit、range-diff、定向测试 |
| `cherry-pick` | 原 patch 在目标 tag 上语义完全适用 | clean apply、代码审查、定向测试 |
| `contribute` | 通用修复正在回馈 upstream | contrib branch/PR，同时说明本 release 如何 carry |
| `retire` | 生产约束已不存在 | 架构/配置证据与风险复核 |

“无冲突”或“能编译”都不是完整结论。没有三项 patch 的明确处置和证据，candidate 不得 promotion。

## 新增或重写规则

- 新 patch 取得下一个稳定 `FZ-xxx`，一个 patch 只表达一个行为意图。
- commit message 添加 `Frenzy-Patch-ID: FZ-xxx` trailer；重写时保留 ID 并记录原 commit。
- 使用 `git show <commit> --pretty=email --patch | git patch-id --stable` 记录稳定 patch-id。
- 填写 intent、invariant、paths、dependencies、tests、upstream issue/PR、drop condition 和 `last_reviewed_against`。
- patch queue 顺序是依赖顺序；新 release manifest 记录实际 applied commit 与处置结论。
- upstream 吸收 patch 后，只在新的已验证 release 中删除本地实现，不移动旧 tag 来整理历史。
- 依赖/lockfile patch 每个 upstream release 重新计算；稳定 Patch ID 记录风险意图，不强制保留旧 diff。

新 commit 描述模板：

```text
目的：
行为不变量：
为什么不能只用配置解决：
受影响路径/依赖：
定向测试：
upstream issue/PR：
删除条件：

Frenzy-Patch-ID: FZ-xxx
```
