# Upstream 状态快照

本文件只记录可变事实。稳定策略与操作命令见 [`UPSTREAM_MAINTENANCE.md`](UPSTREAM_MAINTENANCE.md)。

## 2026-08-03 当前发布观察（Asia/Tokyo）

以下源码身份可由公开仓库中的 annotated tag 闭环核对。私有 release manifest 与 running
artifact 核验已确认该源码在受控的 externally managed 部署中激活；镜像 digest、配置
revision、ops revision、云资源身份和运行拓扑仍只保存在私有运维仓库，不在本文件复制。

| 项目 | 值 |
|---|---|
| published application tag | `frenzy/app/v0.1.169-c68b4c8b.2` |
| published source | `c68b4c8b82d2e8005a02de8ec540589c07272ecb` |
| release branch | `release/c68b4c8b-frenzy.2` |
| upstream release tag | `v0.1.169` |
| upstream tag object | `830b5f507396b858874b171feae1cbcfce1caded` |
| upstream peeled commit | `26d894ef4f50645a4bf1030e378ac892f17d0223` |
| application version | `0.1.169` |
| runtime patch queue | 9 个行为 Patch ID，见 [`PATCH_QUEUE.md`](PATCH_QUEUE.md) |
| deployment conclusion | 已由私有四轴证据确认激活；公开仓库不承载环境专属 artifact/ops 身份 |
| multi-Center conclusion | stop-first 人工冷备切换路径已在受控环境演练；不等于 active-active、自动故障切换或零中断 |

该结论只把公开 app tag/source 与私有部署证据分工写清楚。它不把 release branch `HEAD`、
`upstream/main`、`latest` 或版本字符串单独当作 running artifact 证明。

## 历史快照：2026-07-13 只读观察

本节描述当时的 v0.1.151 基线，已被上面的 v0.1.169 当前发布观察取代；保留它只用于
追溯升级输入。

| 项目 | 值 |
|---|---|
| deployed application tag | `frenzy/app/v0.1.151-e316ebf5.1` |
| deployed source | `3c39b35c81b8e4664d9110b9e68939c66b263817` |
| upstream base | `e316ebf52838a89d57fc790981cce7520f819ac8` |
| current release branch | `release/e316ebf5-frenzy.1` |
| upstream latest formal tag | `v0.1.153` |
| upstream `v0.1.153` tag object | `53717a125583e3916b751c2a5340901c4bfa2bb3` |
| upstream `v0.1.153` peeled commit | `a2bc1337474b68b62391116835e5698ebb5526bd` |
| upstream `main` | `7d239d62e8f1c6aea79164f88903f4158cbf2f98`（观察快照，不作为升级目标） |
| deployed runtime patches | 3，见 [`PATCH_QUEUE.md`](PATCH_QUEUE.md) |

当前 release branch 在 deployed source 后还有纯文档提交；它们没有进入运行镜像。因此所有 range-diff、patch count 和升级基线必须从 deployed application tag 计算，不能从 branch `HEAD` 计算。

## 历史快照：2026-07-31 目标集成只读观察（Asia/Tokyo）

本节记录当时 v0.1.169 集成的输入，不是当前发布或运行事实。其“integration only”结论
已被 2026-08-03 的发布观察取代。

| 项目 | 值 |
|---|---|
| deployed application tag（观察当时的 stable） | `frenzy/app/v0.1.151-e316ebf5.1` |
| deployed source（观察当时的 stable） | `3c39b35c81b8e4664d9110b9e68939c66b263817` |
| old upstream base | `e316ebf52838a89d57fc790981cce7520f819ac8` |
| target upstream tag | `v0.1.169` |
| target tag object | `830b5f507396b858874b171feae1cbcfce1caded` |
| target peeled commit | `26d894ef4f50645a4bf1030e378ac892f17d0223` |
| integration branch | `integration/v0.1.169-frenzy.1` |
| candidate state | 观察当时为 integration only；尚未冻结 release SHA、镜像 digest 或 ops revision |
| version file | 本地候选规范化为 `0.1.169`；upstream tag 内仍为 `0.1.168`，差异已在兼容性报告记录 |

目标 tag 通过 `refs/tags/upstream/v0.1.169` 保存，未对 upstream 开启 push。上述 SHA 在当时
只描述源码输入，不代表当时的 ECR 工件、配置 revision、数据库 schema 或 running artifact
已经更新。

## 下一次升级已知关注点

- 从 `frenzy/app/v0.1.169-c68b4c8b.2` 和对应私有 stable manifest 恢复当前四轴事实，不能从分支 `HEAD` 猜测部署基线。
- FZ-001--FZ-009 必须逐项对下一个明确 upstream 正式 tag 做 `drop-upstreamed`、`reimplement`、`cherry-pick`、`contribute` 或 `retire` 处置，不能机械重放旧 diff。
- migration、配置、OAuth/session、账号 credential、模型/计费、缓存/队列和 N/N-1 兼容性仍须用 [`UPGRADE_COMPATIBILITY_TEMPLATE.md`](UPGRADE_COMPATIBILITY_TEMPLATE.md) 建立新的实例记录。
- 本快照不是下一次升级批准；开始工作前重新读取目标 tag object、peeled commit、release notes 和 fork/ops manifest。

## 更新规则

- 只在只读 fetch 和私有 manifest 核对后更新。
- 记录观察日期、完整 SHA 和数据来源；短 SHA 只用于阅读。
- 新快照置顶；仍有追溯价值的旧数字必须明确标注为历史快照，不能继续冒充当前状态。
