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

## 2026-08-03 实际观察（Asia/Tokyo）

- 仓库为 public fork，默认分支为 `main`；远端 `main` 可读取 `AGENTS.md`，新 clone 有接手入口。
- API 回读到 5 个 active workflows：CI、CLA Assistant、Frenzy Candidate、Release 和
  Security Scan。workflow 默认权限为 `read`，且不允许 workflow 批准 pull request review。
- `main` 没有 branch protection，仓库也没有 repository ruleset；因此 force-push、删除、
  required checks 和 tag immutability 仍未由 GitHub 服务端强制执行。
- Frenzy Candidate 已提供独立候选流水线；每次 release 仍必须引用与目标 SHA 对应的实际
  run/check 证据，不能由 workflow 文件存在反推某个历史 tag 已通过。
- `.github/audit-exceptions.yml` 当前为空；`xlsx` 已由 `@e965/xlsx` 替代，2026-08-03
  重新执行的 production dependency audit 为 high/critical 0。该结果只属于本次源码和
  lockfile，后续每个候选仍须重新审计，不能把空例外清单当作永久安全证明。

这些是 2026-08-03 的只读外部事实和风险登记，不是本轮文档提交自动修复的 GitHub 设置。

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
