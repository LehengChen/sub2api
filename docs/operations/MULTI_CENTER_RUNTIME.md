# Multi-Center Runtime Contract

截至 2026-07-22（Asia/Tokyo），本文件描述应用源码中的多 Center 运行契约。它不表示生产已经有第二台 Center，也不表示 RDS、Redis 或应用已经达到高可用。生产拓扑、slot 身份和切换证据只记录在私有运维仓库。

## 进程角色

`SUB2API_PROCESS_ROLE` 是部署控制面设置的不可变启动参数：

| 角色 | HTTP | 单例后台任务 | 普通启动迁移 | 可通过 `/readyz` 接流量 |
|---|---:|---:|---:|---:|
| `all` | 是 | 是 | 是 | 是，仅兼容单机安装 |
| `active` | 是 | 是，必须持有 worker lease | 否 | 是 |
| `api` | 是 | 否 | 否 | 默认否；未来能力开关开启后才允许 |
| `worker` | 否 | 是，必须持有 worker lease | 否 | 否 |
| `standby` | 是 | 否 | 否 | 否 |
| `migrator` | 否 | 否 | 是，完成后退出 | 否 |

外部运维控制模式禁止使用隐式 `all`。生产必须明确选择角色，避免应用启动时同时发生 schema 迁移、bootstrap 写入、后台任务启动和接流量。

`api` 的未来多实例能力默认由 `SUB2API_MULTI_API_ENABLED=false` 封闭。当前主备方案只使用一个 `active` 和一个未注册 ALB 的 `standby`；不能用 `api` 绕过尚未完成的 active-active 验证。

## Migration-Only

普通 `active`、`api`、`worker` 和 `standby` 启动只读取 `schema_migrations` 并核对所有已嵌入 migration 的 filename/checksum。缺 migration、checksum 不一致或 schema 表不可读时启动失败。

只有以下入口会执行 SQL migration：

```bash
sub2api --migrate
SUB2API_PROCESS_ROLE=migrator sub2api
```

Migrator 不启动 HTTP、OAuth refresh、scheduler、清理任务或其他 worker，也不执行 setup、创建管理员或补种业务分组。migration rehearsal、生产 migration、应用激活必须是三个独立的变更和回滚点。

## Worker Lease

`active` 与 `worker` 启动前必须取得 Redis lease。lease value 绑定非秘密 instance label 和单调递增 fencing token；第二个进程在 lease 存活时启动会失败。默认参数：

```text
SUB2API_WORKER_LEASE_KEY=sub2api:runtime:primary-worker
SUB2API_WORKER_LEASE_TTL_SECONDS=30
SUB2API_WORKER_LEASE_RENEW_SECONDS=10
```

续租失败、lease 被替换或 Redis 不可达时，进程立即变为 not-ready，并进入有限排空与清理。清理期间继续保有 lease，正常停止最后才释放。

该 token 当前是防止双 worker 启动的门禁和审计身份，并没有被所有 PostgreSQL/Redis 关键写路径逐项验证。因此它还不是完整的分布式 fencing。自动故障切换和 active-active 必须继续关闭；人工切换必须先隔离或停止旧主，不能只等待 TTL 猜测旧主已经失效。

## Readiness

`/readyz` 至少要求：

- 应用初始化完成且未进入 drain；
- 当前角色允许接流量；
- PostgreSQL 与 Redis 可用；
- migration filename/checksum 完整；
- `active` 的 worker lease 仍有效；
- active worker 的 scheduler 首次快照 rebuild 已成功。

`standby` 的 `/livez` 可以为 200，但 `/readyz` 必须为 503。这能防止误把冷 standby 注册到 ALB。提升 standby 时应先以 `active` 配置重启、取得新 fencing token、连续通过 readiness 和认证 synthetic，再注册流量。

## 人工切换顺序

1. 冻结 app、config、egress 和 Terraform 变更，核对 stable/candidate manifest、恢复点和两个 slot 的工件/config digest。
2. 从 ALB 注销旧 active，等待负载均衡器和应用内 HTTP/SSE/WebSocket 排空上限。
3. 停止或网络隔离旧 active，确认它不再接流量和执行 worker；不能只依赖健康检查失败。
4. 用相同不可变应用工件、受控配置、JWT/TOTP 和共享 PostgreSQL/Redis 启动新 slot 的 `active` 角色。
5. 核对新 fencing token、连续 `/readyz`、认证 synthetic、固定出口绑定和 usage/billing 写入，再注册 ALB。
6. 任一检查失败时保持新 slot 隔离；只有旧 slot 的工件、配置和 schema 仍兼容时才恢复旧主。

实际中断时间必须从 deregistration、最后旧请求完成、首次新请求成功和长连接断开记录计算。没有演练数据前不得宣称零中断。

## 尚未开放

- 自动故障切换和 active-active；
- 所有关键写路径校验 fencing token；
- 混合版本账号 credential 写入、cache snapshot 和 scheduler/outbox 的完整 N/N-1 证明；
- Redis replica/automatic failover、RDS Multi-AZ、告警动作和恢复演练带来的基础设施 HA；
- 不可恢复的 SSE/WebSocket 跨实例续传。

这些能力未完成时，第二个 Center 只是受控冷备，不是高可用声明。
