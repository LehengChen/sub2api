# 应用滚动发布兼容契约

本文定义 Sub2API 要实现低中断、多副本发布时必须满足的应用层条件。AWS 目标架构与实施阶段位于私有运维仓库 `infra/docs/LOW_DOWNTIME_ARCHITECTURE.md`。

## 当前事实

截至 2026-07-22（Asia/Tokyo），应用源码已加入 `/livez`、真实 `/readyz`、可配置
readiness/shutdown timeout，以及 SIGTERM 先标记 not-ready 再进行有限排空的生命周期
实现。该源码变更尚未自动证明已经部署到生产；生产事实仍须从私有 release manifest
和运行工件核对。普通 HTTP/SSE 会由应用排空计数和 `http.Server.Shutdown` 管理；
hijacked WebSocket 只有在 handler 使用长连接 registry 后才会纳入排空等待，当前不能
据此宣称所有 WebSocket 已经无中断。

截至 2026-07-13，当前部署版本存在以下边界：

- `/health` 固定返回 200，只证明 HTTP 进程可以响应，不检查 PostgreSQL、Redis、migration 或账号调度。
- 没有真正的 `/readyz`；未知路由可能落入前端 SPA，不能被当作 readiness。
- 收到 SIGTERM 后，HTTP server 的 graceful shutdown 上限只有 5 秒。
- 新主机第一次启动时，bootstrap init container 会执行 SQL migrations；同一主机上的普通应用重启不会再次进入该 setup 路径。
- migration runner 使用 PostgreSQL advisory lock，可避免多个新实例同时执行同一 migration，但这不自动保证新旧应用版本兼容。
- 多个周期任务已经使用 Redis leader lock 或 PostgreSQL advisory lock；仍需对所有后台 worker 做多副本认证，不能从部分实现推断全局安全。
- `ConcurrencyService` 启动时把非本进程 request-prefix 的 Redis account/user slot 当作 stale 清理，并删除共享等待计数；第二个健康 Center 启动会破坏第一个 Center 的在途并发状态。修复前禁止双活或滚动重叠。
- xAI 与 Antigravity OAuth `SessionStore` 仍是进程内 map；回调落到另一实例时不能读取原 session。

因此，当前单 Center 可以做维护窗口发布，但不满足“无计划中断”的发布契约。

上面的并发清理是多副本 P0 硬阻断，不是“观察后可接受”的风险。目标实现必须让 slot 归属具有可验证的实例 lease/过期语义，不能把“不是我的 prefix”直接等同于“已死进程”；同时要保留崩溃实例残留的有界回收能力。

## 目标端点

应用应区分：

| 端点 | 用途 | 失败条件 |
|---|---|---|
| `/livez` | 进程是否存活；供容器自愈 | 进程卡死或无法服务 HTTP |
| `/readyz` | 是否可以接收真实流量；供 ALB | DB/Redis 必需连接失败、migration 未完成、关键初始化未完成、正在 drain |
| authenticated synthetic | 发布验收，不作为高频 ALB probe | 登录、读写数据库、分组调度或低成本网关链路失败 |

readiness 必须快速、有超时、无副作用，不能在每个 probe 中做真实模型请求。Redis 在 Standard 模式承担额度、锁和调度状态，应作为生产 readiness 的必需依赖；如果未来设计降级模式，必须另写契约和测试。

## 启动契约

新实例只有在以下步骤全部成功后才能进入 ALB：

1. 拉取明确 digest 的镜像；
2. 生成并校验运行配置；
3. 数据库 migration 已完成且 checksum 一致；
4. PostgreSQL、Redis 和必要本地资源可用；
5. 后台服务完成初始化；
6. `/readyz` 连续通过健康阈值。

启动失败的实例不得替换健康容量。镜像 tag 只能用于可读性，部署控制面必须固定 digest 或不可变的 Launch Template 版本。

## 停止与 drain 契约

正确顺序是：

```text
停止接收新流量
  -> readiness 失败 / 从 target group deregister
  -> 等待 ALB connection drain
  -> SIGTERM
  -> 等待活动 HTTP/SSE/WebSocket 和后台 flush
  -> 超时后才强制退出
```

要求：

- shutdown timeout 可配置，不能写死为 5 秒；
- systemd、容器、ASG lifecycle hook 和 ALB deregistration timeout 要相互一致；
- 新请求在 drain 开始后不能再进入旧实例；
- SSE/WebSocket 客户端必须支持断线重连；长连接无法仅靠负载均衡器获得绝对无感迁移；
- 计费写入、用量日志和幂等状态必须在退出前 flush 或通过数据库幂等恢复。

## 新旧版本并存契约

滚动窗口内至少会同时存在版本 N 和 N+1。候选必须证明：

- JWT、API Key、Redis key 格式和 session 数据可双向读取；
- N 写入的数据 N+1 能读，N+1 写入的数据 N 在回滚窗口内也能读；
- 配置新增有安全默认值，删除配置至少跨一个 release；
- 队列 payload、缓存编码和后台任务锁不会因版本差异重复执行；
- 前端静态资源与后端 API 在浏览器缓存窗口内兼容；
- 账号 credentials/extra 的新字段不会被旧版本更新操作意外抹除。

任何一项不成立时，不能使用普通 rolling；必须改用维护窗口、双写迁移或受控 blue/green。

## Migration 契约

### 允许在 rolling 前执行

- 新增 nullable 字段或带安全默认值的字段；
- 新增表；
- 使用并发方式创建索引；
- 旧代码可以忽略的新数据结构。

### 必须拆阶段

- rename/drop 字段或表；
- 收紧非空、唯一性或外键约束；
- 大规模回填；
- 改变枚举/状态语义；
- 新旧代码不能同时理解的 payload 或缓存格式。

目标流程：

```text
Release A: expand，旧代码继续工作
Job:       backfill/verify，可暂停和重试
Release B: 新代码切换读取，仍保留旧结构
Release C: contract，确认回滚窗口结束后清理
```

长期应提供独立、可重复、只执行 migration 的命令或 job。不能依赖“新 EC2 恰好没有 `.installed` 文件”来定义 migration 时机。

## 多副本后台任务门禁

进入双 Center 前，逐项列出所有启动 goroutine/cron/queue worker，并确认：

- leader lock key 全局唯一；
- lock TTL 大于任务最坏运行时间；
- 锁释放使用 owner token compare-and-delete；
- Redis 故障时的 DB advisory lock fallback 不会双跑；
- worker crash 后任务可重试且幂等；
- 两个实例同时启动不会执行重复外部调用。

代码中已有的 leader-lock helper 是基础，不是完成证明。验收需要真正的双进程 integration test 与生产 canary 观察。

进入双 Center 前还必须完成：

- 两个实例同时持有 account/user slots 时，任一实例启动或退出都不会删除另一个实例的活动 slot/waiter；
- crash 后的 slot 能在明确 TTL/lease 后回收，且并发限额不会永久泄漏；
- OAuth state/session 外部化，或由签名无状态数据实现，并验证跨实例 start/callback；
- 备份、账号定时测试、渠道监控、token refresh、cleanup、aggregation 等所有启动任务逐项分类为“每实例幂等”或“带 fencing 的单例”；
- 不依赖负载均衡 sticky session 掩盖状态不共享问题。

## 发布兼容清单

每个应用 release 必须给出：

```text
schema change: none / expand / backfill / contract
old+new coexistence tested: yes/no
rollback without DB restore: yes/no
cache/payload format changed: yes/no
background workers multi-replica safe: yes/no/not touched
readiness contract satisfied: yes/no
long-lived connection behavior tested: yes/no/not touched
```

只要其中一个必要答案为 `no`，私有运维仓库就必须选择维护窗口或更保守的流量切换方案，而不能把 release 标记为低中断。
