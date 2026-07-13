# Frenzy Runtime Patch Queue

本文件只记录当前已部署应用相对 upstream base 的运行时差异。纯文档、CI 和 ops 提交不计入 patch queue。每个差异使用稳定 Patch ID；commit SHA 会在 reimplement/cherry-pick 后变化，Patch ID 与行为不变量不变。

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
last_reviewed_against: e316ebf52838a89d57fc790981cce7520f819ac8
upstream_issue_or_pr: none
```

- Intent：修复 deployed base 当时的运行依赖漏洞并对齐 Go modules。
- Invariant：生产 candidate 不包含未接受、未到期例外覆盖的可利用高危/严重运行依赖漏洞。
- Paths：`backend/go.mod`、`backend/go.sum`。
- Tests：unit/integration、编译、govulncheck、容器 Inspector。
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
