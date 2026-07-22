# Upstream 状态快照

本文件只记录可变事实。稳定策略与操作命令见 [`UPSTREAM_MAINTENANCE.md`](UPSTREAM_MAINTENANCE.md)。

## 2026-07-13 只读观察

> 2026-07-22（Asia/Tokyo）补充：精确目标 `v0.1.163` tag object 为
> `bb752ef7776dc126ffca5df9188087d0d0aed559`，peeled commit 为
> `d0bdd7e771636a8d315f542cafd39484f39bd60c`。已从该 tag 创建
> `integration/v0.1.163-frenzy.1`；tag 未签名，candidate 尚未批准、构建或部署。
> 兼容性和门禁见 [`UPGRADE_V0.1.163.md`](UPGRADE_V0.1.163.md)。下表继续保留已部署基线事实。

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

## 下一次升级已知关注点

- 三个本地 patch 尚未被 `git cherry` 判定为 upstream 等价补丁。
- upstream 已修改 `backend/go.mod`、`go.sum`、pricing/config 和部署示例；offline pricing 与依赖补丁必须重新审查，不能机械 cherry-pick。
- `v0.1.153` 含 migration 和配置变化；必须先完成 [`UPGRADE_COMPATIBILITY_TEMPLATE.md`](UPGRADE_COMPATIBILITY_TEMPLATE.md) 的实例记录。
- 本快照不是升级批准；开始工作前重新读取 upstream tag object、peeled commit、release notes 和 fork/ops manifest。

## 更新规则

- 只在只读 fetch 和私有 manifest 核对后更新。
- 记录观察日期、完整 SHA 和数据来源；短 SHA 只用于阅读。
- 新快照替换本节时保留 Git 历史，不在稳定流程文档堆积过期数字。
