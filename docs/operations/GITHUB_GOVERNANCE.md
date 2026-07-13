# Fork GitHub 治理

本文同时记录期望状态与实际状态。GitHub 设置属于外部可变事实，必须通过 `gh`/API 回读，不能因为文档写了要求就假定已经生效。

## 期望状态

- 默认分支 `main` 包含 `AGENTS.md`、运维入口和 fork CI，是新 clone 的接手入口。
- `main` 与 `release/**` 禁止 force-push 和删除；合并必须通过约定 checks。
- `frenzy/app/**` tag 禁止更新和删除。
- release tag 只能指向通过 review/checks 的固定 candidate SHA。
- fork workflow 默认 `contents: read`；不得反写源码或默认分支。
- upstream 专用 workflow 必须有 `github.repository == 'Wei-Shaw/sub2api'` 身份 guard。
- workflow/action 版本、安全例外和 required checks 定期复核。

## 2026-07-13 实际观察

- GitHub 默认分支为 `main`；本次整理后已包含 `AGENTS.md`、运维入口、CI 安全门禁和维护模板，新 clone 可以直接接手。
- Actions 权限为 enabled；本次整理 push 后，API 已回读到 4 个 active workflows。历史 deployed application tag 的 check-run 仍为 0，因此不能追溯性地声称该版本经过 fork CI；新的 candidate 必须引用实际成功 run。
- `main`、当前 release 没有 branch protection，也没有 repository ruleset。
- upstream `release.yml` 仍有 `workflow_dispatch`、`contents: write` 和发布逻辑。本次维护变更已给所有 job（包括实际 push 默认分支的 job）加入 upstream repository 身份 guard，并已进入 fork `main`。Frenzy 仍需要独立 build-only workflow。
- 本次维护变更删除了三个已过期且当前 audit 不再命中的例外，把剩余 xlsx 例外 owner 改为仓库责任人，并让 validator 全局拒绝过期/占位 owner。两个 xlsx 高危例外仍有效至 2026-10-06，必须在到期前升级、移除或重新获得有依据的审批。

这些是风险登记，不是本轮文档提交自动修复的外部状态。

## 只读核验

```bash
gh repo view LehengChen/sub2api --json defaultBranchRef
gh workflow list --repo LehengChen/sub2api --all
gh run list --repo LehengChen/sub2api --limit 20
gh api repos/LehengChen/sub2api/rulesets
gh api repos/LehengChen/sub2api/branches/main/protection
git show origin/main:AGENTS.md >/dev/null
```

保护接口返回 404/403、workflow 列表为空或没有 checks 时，应报告真实缺口。不能把“仓库计划不支持”解释为“已经保护”；应记录补偿控制，例如只允许 PR、annotated immutable tag、远端回读和双人 review。

## Fork workflow 合同

候选流水线至少应覆盖：

1. backend unit/integration 和 golangci-lint；
2. frontend lint、typecheck、完整 Vitest 和 build；
3. 生成代码无差异；
4. 固定版本的 govulncheck/gosec、依赖和容器扫描；
5. clean candidate SHA 的 `linux/amd64` build；
6. SBOM、镜像 digest 和 provenance/attestation；
7. 只读权限，不 push branch、不修改 `VERSION` 后写回默认分支。

fork CI 是可重复证据，不替代 migration 副本演练、固定代理/分组/计费 E2E 或生产 canary。

## 启用/调整后的验收

- fresh clone 默认能看到 `AGENTS.md`。
- 测试 PR 产生所有预期 checks，且失败会阻止 promotion。
- ruleset/protection 可以通过 API 回读；force-push/delete/tag update 被拒绝。
- fork release workflow 没有默认分支写能力。
- 至少保存一次成功 candidate run；release manifest 记录 run URL、candidate SHA 与 artifact digest。
