# Sub2API 运维文档入口

本目录保存可进入公开应用 fork 的通用运行与维护知识。AWS 账号、生产资源和发布清单属于私有运维仓库，不在这里复制。

## 文档地图

| 文档 | 说明 |
|---|---|
| [`../../AGENTS.md`](../../AGENTS.md) | 新 Codex 会话的总入口、边界和验证规则 |
| [`DEPLOYMENT_MODES.md`](DEPLOYMENT_MODES.md) | Simple、Standard、Backend Mode、分组类型和部署拓扑的区别 |
| [`UPSTREAM_MAINTENANCE.md`](UPSTREAM_MAINTENANCE.md) | fork/upstream 同步、patch queue、测试、发布和回滚规范 |
| [`../../DEV_GUIDE.md`](../../DEV_GUIDE.md) | 应用开发环境和常见工程问题；其中版本信息需以代码为准 |

若工作区同时存在私有 `infra/` 仓库，再阅读：

- `infra/AGENTS.md`
- `infra/docs/README.md`
- `infra/releases/` 中当前环境的 release manifest

## 信息分层

从高到低采用以下事实优先级：

1. 实时只读检查：运行中的应用、AWS API、数据库/API 的脱敏状态
2. 私有运维仓库中的 release manifest 和 Terraform state 所描述的资源
3. 当前应用 release tag、镜像 digest 和配置 revision
4. 代码、迁移和测试
5. 文档快照与历史对话

文档与代码冲突时，以代码和实时状态为准，并在本次工作中修正文档。不能用文档替代生产检查。

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
- 生产部署后更新私有 baseline 和 release manifest，不在公开文档复制生产秘密。
- 文件中出现的 commit 数和版本均须标明观察日期；它们是快照，不是永久事实。
