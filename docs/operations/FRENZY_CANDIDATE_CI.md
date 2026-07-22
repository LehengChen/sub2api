# Frenzy Candidate CI

状态：workflow、策略测试和文档已加入应用 fork；截至 2026-07-22 尚未把该
workflow 的成功 run 当作生产发布证据。它是非生产 candidate gate，不会推送镜像、
写 Git ref、部署或修改 AWS/私有 ops。

## 触发边界

`.github/workflows/frenzy-candidate.yml` 只接受三类入口：

- `workflow_dispatch`，必须提供显式 `version`；
- 头分支以 `integration/` 或 `release/` 开头的 pull request；
- `frenzy/candidate/**` tag push。

GitHub Actions 没有 pull request head-ref 的原生过滤器，因此 workflow 可以被其他
PR 事件唤醒，但 `gate` job 在任何 runner 工作前 fail-closed 跳过它们。候选 PR 的
source 使用 `pull_request.head.sha`，不是 GitHub 自动生成的 merge SHA；手动和 tag
运行使用事件对应的完整 commit SHA。每个后续 job 都重新 checkout 该 SHA 并核对
`git rev-parse HEAD`。

版本、完整源码 SHA 和 UTC build date 会同时写入 job outputs、evidence JSON、OCI
labels 和 Go linker build args。UTC date 从 candidate commit 的 timestamp 确定性生成；
它不改变应用运行时应使用的 `Asia/Tokyo` 配置。

## 必需 gate

成功的 summary 只有在以下每一项都成功时才会通过：

1. Backend unit：`go test -tags=unit ./...`。
2. Backend integration harness：`CI=true go test -tags=integration ./...`，使用仓库已有的 testcontainers harness。
3. Ent/Wire：`go generate ./ent` 和 `go generate ./cmd/server` 后工作树无任何 diff 或未跟踪生成物。
4. `golangci-lint` 固定为 `v2.9`。
5. Frontend frozen install、lint、typecheck、完整 Vitest 和 build。
6. 固定版本 `govulncheck`（当前 workflow 为 `v1.1.4`）。
7. `pnpm audit --prod --audit-level=high --json` 以及现有 `.github/audit-exceptions.yml` validator；审计 JSON 只写入临时 evidence，不覆盖仓库文件。
8. Buildx `linux/amd64` OCI build，传入显式 `VERSION`、`COMMIT`、`DATE`，并请求 `provenance: mode=max` 与 `sbom: true`。

后端 race gate 目前刻意限定为新增共享状态契约：`internal/runtimecontrol`、
`internal/server` 全包，以及 `internal/service` 中 WorkerFence/OAuth session
定向测试。对整个旧 `internal/service` 包并发运行会触发既有测试对进程全局 fixture
的竞争；该清理项不能被候选 evidence 伪装成已解决，也不能阻塞本次新增契约的精确
race 证明。

后端、前端、安全和 OCI evidence 作为短期 GitHub artifact 上传，均标记为
`production: false`，保留期不代表批准期。

## Digest 与签名边界

候选 workflow 的 Buildx 输出为本地 OCI archive，`push: false`、不登录 registry，
因此它不会产生可激活的 ECR/registry image digest，也没有签名身份。报告会分别记录：

- OCI archive 的 SHA-256（用于下载校验）；
- 本地 OCI manifest/buildx metadata 中若存在的 digest（仅本地工件事实）；
- `registry_image_digest.status=missing`；
- `signature.status=missing`，并说明是 read-only PR CI 的预期缺失。

不能把 archive SHA、Buildx metadata 或 workflow 成功误称为生产镜像 digest、签名或
provenance promotion。私有 ops 必须在独立批准阶段重新构建/推送不可变 ECR tag，
读取实际 digest，完成签名/证明能力核验，再写入 release catalog。

## 权限与禁止动作

workflow 顶层权限固定为 `contents: read`，没有 `packages: write`、`id-token` 或其他
写权限。源码、tag、镜像和默认分支均不写；没有 `git push`、`docker push`、registry
login、Terraform、AWS、kubectl、SSH 或 rollout 调用。artifact service 只保存非生产
证据，不是部署通道。

本地策略回归测试：

```bash
bash tools/test_frenzy_candidate_workflow.sh
shellcheck tools/test_frenzy_candidate_workflow.sh
```

workflow 通过后仍不能跳过 migration 副本演练、代理/分组/计费 synthetic、私有
release manifest、镜像激活审批或主备切换门禁。
