# Fork 与 Upstream 同步流程

本文是 Frenzy fork 接收 upstream release 的唯一权威流程。它只负责形成可审查的应用候选；是否部署到生产，必须在私有 `infra/` 仓库中另行批准和执行。

当前 commit、tag 和差距属于可变事实，统一记录在 [`UPSTREAM_STATUS.md`](UPSTREAM_STATUS.md)。本地运行时差异记录在 [`PATCH_QUEUE.md`](PATCH_QUEUE.md)。不要把快照数字写回本流程。

## 不变量

- `origin` 是 `LehengChen/sub2api`；`upstream` 是只读的 `Wei-Shaw/sub2api`。
- `git remote get-url --push upstream` 必须为 `DISABLED`。
- 已部署版本从私有 release manifest 的 `deployed_tag`（或历史等价字段）读取，不能用分支 `HEAD`、最新 tag 或聊天记录猜测。
- 默认分支 `main` 是 fork 的维护和接手入口，不自动代表线上版本。
- upstream 同步不等于生产发布授权；应用仓库不执行 ECR、SSM、Terraform 或 rollout。
- 已发布的 `release/*` 分支和 `frenzy/app/*` tag 不 rebase、不 force-push、不移动。
- 不把 `upstream/main` 直接 merge/rebase 到 `main` 或生产 release；每次只从一个明确的 upstream 正式 tag 建立 integration。
- 不用无范围的 `git fetch --tags`。upstream tag 进入独立的 `refs/tags/upstream/*` 命名空间，避免与 fork tag 冲突。
- 每个本地运行时补丁必须有稳定 Patch ID、明确处置和定向测试。clean cherry-pick 只说明文本可应用，不说明语义正确。

## 分支、tag 与职责

```text
main                                  fork 默认控制分支；文档、CI、已批准维护状态
integration/v<upstream>-frenzy.<n>    一次 upstream 集成；冲突、补丁重放和验证
release/v<upstream>-frenzy.<n>        固定候选 SHA 后创建；不继续开发
contrib/<topic>                       可单独回馈 upstream 的通用修复
frenzy/app/v<upstream>-frenzy.<n>     指向 release SHA 的不可变 annotated tag
refs/tags/upstream/v<upstream>        本地保存的 upstream tag 对象
```

历史 release/tag 的命名可能不同，继续保留，不为了统一格式改写历史。新的 release 使用上面的规范。

## 同步状态机

### 0. 取得已部署事实

先阅读私有 ops 仓库中当前环境最新、状态为 stable 的 release manifest，手工核对并设置：

```bash
DEPLOYED_APP_TAG='frenzy/app/<verified-tag>'
OLD_BASE='<verified-upstream-base-sha>'
UPSTREAM_TAG='v<target-version>'
RELEASE_NO='<n>'
```

manifest schema 尚未统一时，不要写一个“猜字段”的自动解析器。必须同时核对 tag、完整源码 SHA、镜像 digest 和 ops revision；缺少其中任一项就停止。

### 1. Fail-closed preflight

从应用仓库根目录执行：

```bash
test -z "$(git status --porcelain)"
test "$(git remote get-url origin)" = "https://github.com/LehengChen/sub2api.git"
test "$(git remote get-url upstream)" = "https://github.com/Wei-Shaw/sub2api"
test "$(git remote get-url --push upstream)" = "DISABLED"
git rev-parse --verify "refs/tags/${DEPLOYED_APP_TAG}^{commit}"
git merge-base --is-ancestor "$OLD_BASE" "$DEPLOYED_APP_TAG"
```

工作区有用户修改时先保存或提交到正确分支，不能 reset；remote、tag 或祖先关系不符时停止并调查。

### 2. 只读取目标 upstream tag

```bash
git fetch --no-tags --prune upstream '+refs/heads/*:refs/remotes/upstream/*'
git fetch --no-tags --prune origin '+refs/heads/*:refs/remotes/origin/*'

git ls-remote --tags upstream \
  "refs/tags/${UPSTREAM_TAG}" \
  "refs/tags/${UPSTREAM_TAG}^{}"

git fetch --no-tags upstream \
  "refs/tags/${UPSTREAM_TAG}:refs/tags/upstream/${UPSTREAM_TAG}"

TARGET_REF="refs/tags/upstream/${UPSTREAM_TAG}"
TARGET_TAG_OBJECT="$(git rev-parse "$TARGET_REF")"
TARGET_COMMIT="$(git rev-parse "${TARGET_REF}^{commit}")"
git merge-base --is-ancestor "$OLD_BASE" "$TARGET_COMMIT"
```

把 `ls-remote` 返回的 tag object 和 peeled commit 记录到升级兼容性报告。若 upstream tag 未签名，不能伪称通过了签名验证；当前可执行的控制是记录远端 object/commit、保存本地 namespaced ref，并在 fork 端保护 release tag 不被移动。

同名 `refs/tags/upstream/*` 已存在但远端 object 变化时，fetch 应失败。不要加 `--force` 掩盖异常。

### 3. 建立 integration 并审查差异

```bash
VERSION="${UPSTREAM_TAG#v}"
INTEGRATION="integration/v${VERSION}-frenzy.${RELEASE_NO}"
git switch --create "$INTEGRATION" "$TARGET_COMMIT"

git diff --stat "$OLD_BASE..$TARGET_COMMIT"
git diff --name-status "$OLD_BASE..$TARGET_COMMIT" -- \
  backend/migrations backend/go.mod backend/go.sum \
  frontend/package.json frontend/pnpm-lock.yaml deploy
```

复制 [`UPGRADE_COMPATIBILITY_TEMPLATE.md`](UPGRADE_COMPATIBILITY_TEMPLATE.md) 形成这次升级的兼容性记录。至少审查：

- SQL migration、锁表、backfill、N/N-1 兼容和数据库回滚类别；
- 配置新增、删除、默认值、secret 与部署模板；
- API、OAuth、账号 credential、模型、计费、分组和调度语义；
- Redis key/cache/payload 格式以及所有后台任务；
- SSE、WebSocket、代理、TLS/HTTP2 和长连接行为；
- Go/Node 工具链、lockfile 和生成代码。

### 4. 逐项处置 patch queue

对 [`PATCH_QUEUE.md`](PATCH_QUEUE.md) 中每个稳定 Patch ID 选择一个结论：

- `drop-upstreamed`
- `reimplement`
- `cherry-pick`
- `contribute`
- `retire`

每项都要记录目标 commit、理由、drop condition 和测试。没有完整处置表时不能形成 candidate。

遇到冲突时：

1. 记录冲突路径与相关 upstream commit；
2. `git cherry-pick --abort`；
3. 按目标版本接口重新实现语义；
4. 在新 commit trailer 中写 `Frenzy-Patch-ID: FZ-xxx`；
5. 用定向测试和 `git range-diff` 证明行为，而不是在冲突界面随意拼接。

lockfile 必须通过对应包管理器重生成；Ent/Wire 等生成文件必须通过生成器重生成，禁止手工修改生成结果。

比较旧、新 patch queue 时只使用已部署应用 tag，不使用当前分支 `HEAD`：

```bash
git range-diff \
  "$OLD_BASE..$DEPLOYED_APP_TAG" \
  "$TARGET_COMMIT..HEAD"
```

### 5. 验证 candidate

最低 release gate：

```bash
(cd backend && go test -tags=unit ./...)
(cd backend && go test -tags=integration ./...)
(cd backend && golangci-lint run ./...)

pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend run lint:check
pnpm --dir frontend run typecheck
pnpm --dir frontend run test:run
pnpm --dir frontend run build
```

还必须完成：

- `govulncheck`、前端依赖审计和容器扫描；扫描结论绑定具体 commit/digest；
- 所有保留 patch 的定向测试；
- migration 副本演练；
- `linux/amd64` 镜像构建；
- API Key → 分组 → 调度 → 固定代理，以及低成本 Claude/OpenAI synthetic；
- [`ROLLING_RELEASE_CONTRACT.md`](ROLLING_RELEASE_CONTRACT.md) 的兼容性结论。

GitHub checks 为零、workflow 未运行或例外已过期都不算通过。当前 fork CI 与治理实况见 [`GITHUB_GOVERNANCE.md`](GITHUB_GOVERNANCE.md)。

### 6. PR、固定 SHA 与 promotion

```bash
git push --set-upstream origin "$INTEGRATION"
gh pr create --base main --head "$INTEGRATION" --fill
```

PR review 和 required checks 完成后，先记录 PR head 的不可变 `CANDIDATE_SHA`。release tag 保持目标 upstream + Frenzy patch 的线性历史，不依赖可移动分支名：

```bash
CANDIDATE_SHA='<approved-full-sha>'
RELEASE_BRANCH="release/v${VERSION}-frenzy.${RELEASE_NO}"
APP_TAG="frenzy/app/v${VERSION}-frenzy.${RELEASE_NO}"

test "$(git rev-parse HEAD)" = "$CANDIDATE_SHA"
test -z "$(git status --porcelain)"
git branch "$RELEASE_BRANCH" "$CANDIDATE_SHA"
git tag -a "$APP_TAG" "$CANDIDATE_SHA" -m "Frenzy application release $APP_TAG"
git push origin "refs/heads/${RELEASE_BRANCH}:refs/heads/${RELEASE_BRANCH}"
git push origin "refs/tags/${APP_TAG}:refs/tags/${APP_TAG}"

git ls-remote origin \
  "refs/heads/${RELEASE_BRANCH}" \
  "refs/tags/${APP_TAG}" \
  "refs/tags/${APP_TAG}^{}"
```

远端 branch SHA、tag object 和 peeled commit 必须与本地记录一致。release/tag 创建后，不再向 release 分支追加文档；维护文档通过 PR 进入 `main`，避免 branch HEAD 被误当成部署源码。

tag 回读一致后，将已批准 PR 以 merge commit 合入 `main`，使默认分支得到最新维护状态并保留 release commit 的祖先关系；不要 squash/rebase 掉稳定 Patch ID 的实现历史。`main` 的 merge commit 不是应用部署引用，生产仍只认上面的 app tag。若 merge 解决过程改变了 candidate tree，停止并把变化作为新的 candidate 重新测试，不能让 main 和 release 静默分叉。

### 7. 交给私有 ops 发布

应用仓库只交付：

- upstream tag object/peeled commit；
- upstream base、candidate SHA、release branch、annotated app tag；
- Patch ID 处置表；
- 测试、安全扫描、migration/回滚兼容结论；
- clean `linux/amd64` 镜像构建输入。

私有 ops 仓库负责 ECR digest、SSM/Terraform 激活、rollout、smoke、观察和 release manifest。upstream 同步完成本身不授权生产写操作。

## 停止条件

出现任一情况必须停止 promotion 或部署：

- 已部署 tag/完整 SHA/镜像 digest/ops revision 无法闭环；
- upstream remote 可 push，或目标 tag object 与先前记录不一致；
- 目标 tag 不是旧 upstream base 的后代；
- patch queue 有未处置项目；
- migration 或配置变更没有 N/N-1 与回滚结论；
- 测试、扫描、amd64 build 或生产链路 synthetic 缺失；
- candidate 工作区不 clean，或构建输入不是 candidate SHA；
- release/tag 远端回读不一致。

## 节奏

- 每周：只读 fetch heads，检查正式 release/security 变化，不自动集成或部署。
- 每次 upstream release：更新 [`UPSTREAM_STATUS.md`](UPSTREAM_STATUS.md)，建立独立 compatibility report。
- 每个 candidate：重新计算 patch queue，不沿用上次结论。
- 每季度：审查本地 patch 是否可以贡献、删除或改成配置，并核对 GitHub 治理状态。
