# Sub2API 运维文档入口

本目录保存可进入公开应用 fork 的通用运行与维护知识。AWS 账号、生产资源和发布清单属于私有运维仓库，不在这里复制。

## 文档地图

| 文档 | 说明 |
|---|---|
| [`../../AGENTS.md`](../../AGENTS.md) | 新 Codex 会话的总入口、边界和验证规则 |
| [`PROJECT_MAINTENANCE.md`](PROJECT_MAINTENANCE.md) | 双仓库职责、事实轴、变更分类和内部维护生命周期 |
| [`DEPLOYMENT_MODES.md`](DEPLOYMENT_MODES.md) | Simple、Standard、Backend Mode、分组类型和部署拓扑的区别 |
| [`UPSTREAM_MAINTENANCE.md`](UPSTREAM_MAINTENANCE.md) | fork/upstream 同步状态机、候选 promotion 和停止条件 |
| [`UPSTREAM_STATUS.md`](UPSTREAM_STATUS.md) | 当前 deployed/upstream 差距的带日期快照 |
| [`PATCH_QUEUE.md`](PATCH_QUEUE.md) | Frenzy runtime patch 的稳定 ID、行为不变量和删除条件 |
| [`UPGRADE_COMPATIBILITY_TEMPLATE.md`](UPGRADE_COMPATIBILITY_TEMPLATE.md) | 每次 upstream 升级的 migration/config/回滚审查模板 |
| [`ROLLING_RELEASE_CONTRACT.md`](ROLLING_RELEASE_CONTRACT.md) | readiness、drain、N/N-1、migration 和多副本应用契约 |
| [`HEALTH_AND_DRAIN.md`](HEALTH_AND_DRAIN.md) | 应用 `/livez`、`/readyz`、SIGTERM 排空和长连接注册契约 |
| [`EXTERNAL_RELEASE_CONTROL.md`](EXTERNAL_RELEASE_CONTROL.md) | 外部运维控制、只读 release catalog、版本 API capability 与 fail-closed 契约 |
| [`GITHUB_GOVERNANCE.md`](GITHUB_GOVERNANCE.md) | fork CI、分支/tag 保护的期望与实际状态 |
| [`../../DEV_GUIDE.md`](../../DEV_GUIDE.md) | 本地开发示例；Git/upstream 和生产规则不在该文件定义 |

若工作区同时存在私有 `infra/` 仓库，再阅读：

- `infra/AGENTS.md`
- `infra/docs/README.md`
- `infra/releases/` 中当前环境的 release manifest

## 四条事实轴

不要用一条线性优先级让某类事实覆盖另一类：

1. Desired：Git 中应用、Terraform 和版本化非敏感配置说明“应该是什么”。
2. Actual：实时应用/AWS API 与 remote state 说明“实际是什么”。
3. Artifact：app tag/SHA、镜像 digest、AMI/CA/SSM version 说明“运行的是什么”。
4. Approved/history：change、release manifest、tag 和测试证据说明“谁批准、发生过什么”。

四条轴必须闭环；不一致就是 drift。实时状态不能自动覆盖 Git 中的批准期望，文档也不能替代生产检查。

## 两个仓库的职责

公开应用 fork只保存：

- 上游应用代码和最小本地补丁
- 应用测试
- 不含生产身份的模式说明和 upstream 维护规范

私有运维仓库保存：

- Terraform、出口代理镜像和部署脚本
- AWS 账号/Region/域名等环境事实
- release manifest、发布/回滚 runbook
- Standard 模式迁移的环境检查记录

## 更新约定

- 模式语义变化时更新 `DEPLOYMENT_MODES.md`。
- remote、branch、patch queue 或发布流程变化时更新 `UPSTREAM_MAINTENANCE.md`。
- upstream commit/tag 的可变快照只更新 `UPSTREAM_STATUS.md`，不污染稳定流程。
- 生产部署后更新私有 baseline 和 release manifest，不在公开文档复制生产秘密。
- 文件中出现的 commit 数和版本均须标明观察日期；它们是快照，不是永久事实。
