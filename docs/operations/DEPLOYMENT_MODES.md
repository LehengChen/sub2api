# Sub2API 运行模式与部署形态

本文区分五个经常被混用的概念：应用运行模式、Backend Mode、注册/支付开关、分组计费类型和基础设施拓扑。

## 1. 应用运行模式只有两个

配置项为 `run_mode` / `RUN_MODE`，代码常量位于 `backend/internal/config/config.go`。

| 能力 | `simple` | `standard` |
|---|---|---|
| 目标场景 | 个人、单管理员、内部试用 | 管理员托管多用户、完整平台 |
| 用户/分组/订阅 UI | 大量隐藏或路由禁止 | 完整显示 |
| 管理员创建用户 | UI 隐藏 | 支持 |
| 账号分组调度隔离 | 忽略组边界，按平台选择全部可调度账号 | API Key 所属组只调度该组账号 |
| 余额/订阅/Key 额度检查 | 跳过 | 执行 |
| 使用日志 | 记录但不扣费 | 记录并按规则计费 |
| 默认组 | 启动时自动补齐平台默认组 | 由管理员维护 |
| 多应用副本 | 不推荐 | 业务能力适合多用户，但当前版本尚未通过多副本安全认证 |

Simple 不是“精简 UI 的 Standard”。它会在 API Key 认证后提前放行、跳过余额和订阅检查，并在调度时忽略 `account_groups` 边界。即便 Key 显示属于某个默认组，也不能据此认定请求只会使用那个组内的账号。

Standard 才是用户所描述的完整模式：管理员创建用户、维护公开/专属分组、给账号绑定一个或多个分组，并让每把 API Key 只使用其所属组的账号池。

Standard 不等于已经可以横向扩容。当前版本的并发槽启动清理会影响其他进程的 Redis request-prefix，部分 OAuth 临时状态仍在进程内，且若干后台任务尚未具备跨实例 fencing/幂等证明。双 Center 前必须完成 [`ROLLING_RELEASE_CONTRACT.md`](ROLLING_RELEASE_CONTRACT.md) 的多副本门禁；不能仅因为运行模式是 Standard 就直接增加实例。

非法或缺失的 `run_mode` 当前会回退为 `standard`。不要依赖这个回退；生产必须显式配置。

## 2. Backend Mode 不是第三种运行模式

`backend_mode_enabled` 是数据库中的站点设置，与 `RUN_MODE` 正交。

开启后：

- 普通用户不能登录自助界面；
- 注册和大多数自助认证入口被禁止；
- 管理员仍可登录；
- 已存在的 API Key 网关调用仍可继续。

因此：

| 目标 | Backend Mode |
|---|---|
| 管理员创建用户，用户自己登录和创建 Key | 关闭 |
| 只有管理员操作，外部系统仅持有已发放 Key | 可开启 |

受控多用户部署的推荐值是关闭 Backend Mode，并单独关闭公众注册。

## 3. 注册、支付和功能开关是另一层策略

`registration_enabled`、支付、邀请码、推广码、风控和渠道监控均不是运行模式。

推荐的第一阶段 Standard 姿态：

```text
run_mode                  = standard
backend_mode_enabled      = false
registration_enabled      = false
payment_enabled           = false
公开推广/兑换/分销入口      = false
管理员手工创建用户          = true
```

这样管理员可以创建用户，用户可以登录并使用 Key，但公众不能自行注册，也不会提前引入支付和公开 SaaS 合规面。

若未来打开公众注册，应先补齐邮箱验证、Turnstile/风控、隐私与服务条款、滥用处置、支付合规和审计，再单独上线；不要把它和 Standard 切换绑定成一次发布。

## 4. 分组的 `subscription_type` 不是运行模式

Standard 下，分组还有两种计费类型：

| 分组类型 | 访问/计费语义 |
|---|---|
| `standard` | 按用户余额扣费；公开组所有用户可选，专属组还需 `allowed_groups` 授权 |
| `subscription` | 用户必须有该组的有效订阅，并执行订阅的日/周/月限制 |

账号和分组是多对多关系；API Key 只绑定一个分组。固定代理属于账号属性，不属于分组属性。

建议业务池按平台划分：

- `claude-prod`：Anthropic 账号
- `openai-prod`：OpenAI 账号
- 可选 `*-canary`：只挂少量账号，用于版本或模型验证

不要按 Osaka-01/02/03 拆业务组。否则客户端会失去统一调度，而固定出口已经由账号到代理的绑定保证。

## 5. 部署方法不是应用运行模式

仓库提供的包装方式包括：

| 形态 | 内容 |
|---|---|
| Docker Compose | 应用 + PostgreSQL + Redis 一体化，适合单机 |
| Standalone Compose | 只运行应用，外接 PostgreSQL/Redis |
| Binary + systemd | 应用二进制作为系统服务 |
| 自定义 AWS | 私网应用节点 + RDS + ElastiCache + ALB + 固定出口节点 |

以上任一种都可以配置 Simple 或 Standard。`server.mode=debug|release` 只是 Web 服务器/Gin 的调试或发布模式，也不是产品业务模式。

同样，`pilot` 与 `production` 是基础设施成熟度：RDS 是否 Multi-AZ、Redis 是否有副本、应用是否多副本、是否有演练和告警。把 `RUN_MODE` 改为 Standard 不会自动获得高可用。

## 推荐目标：受控 Standard

适合当前需求的目标是：

```text
应用：Standard
Backend Mode：关闭
公众注册：关闭
支付/推广：首阶段关闭
账号池：按平台分组
用户：管理员手工创建
计费：先选择并验证余额制或订阅制中的一种
出口：账号固定绑定独立代理/EIP
```

## Simple → Standard 的迁移门槛

切换前必须逐项检查：

1. 每个账号已经显式加入正确分组；不能只检查默认组是否存在。
2. 每把现有 Key 都有正确且 active 的分组。
3. Standard 组用户有足够余额，或 Subscription 组用户有有效订阅。
4. 审计 Key 的 `quota`、过期时间和各时间窗口限额；Simple 下被跳过的限制会在 Standard 立即生效。
5. 审计用户并发。Simple 会把初始管理员并发一次性提升到 30，切回 Standard 不会自动降回。
6. 验证所有允许模型都有可用价格；离线定价部署尤其需要检查覆盖率。
7. 保持 `registration_enabled=false`，确认 Backend Mode 和支付开关符合目标。
8. 备份数据库并建立可验证的恢复点。

推荐分阶段：

1. 在现有版本上只准备数据和设置。
2. 只切换 `run_mode`，不要同时升级 upstream 镜像或数据库 schema。
3. 完成管理员登录、用户、余额/订阅、两平台 API Key 和真实模型请求验收。
4. 稳定观察后再做 RDS/Redis HA 和应用多副本。
5. 最后才评估公开注册、支付和其他 SaaS 功能。

模式切换本身共用同一套数据模型，通常可以回切 Simple；但在 AWS 等把 `run_mode` 写入不可变 UserData 的部署中，切换可能替换无状态应用节点并产生维护窗口。数据库迁移或 upstream 升级若与切换混在一起，会显著破坏这个回滚边界。

## 最小验收

- 管理员可登录并看到用户、分组、订阅相关菜单。
- 新建普通用户后可登录，但未授权用户不能进入专属组。
- Anthropic Key 只调度 Anthropic 生产组账号。
- OpenAI Key 只调度 OpenAI 生产组账号。
- 余额不足或订阅失效时按预期拒绝。
- 使用日志、余额/订阅扣减和账号用量一致。
- 公众注册仍不可用。
- 账号固定代理和出口 EIP 未发生变化。

## 已知文档漂移

当前 README 提到生产 Simple 需要 `SIMPLE_MODE_CONFIRM=true`，但当前代码没有读取该变量。不要把它当成启动保护。真正的保护必须来自显式配置、部署校验和测试。

## 代码入口

- `backend/internal/config/config.go`：`RunModeStandard`、`RunModeSimple`
- `backend/internal/server/middleware/api_key_auth.go`：Simple 提前放行和 Standard 计费检查
- `backend/internal/service/gateway_scheduling.go`：分组调度差异
- `backend/internal/repository/simple_mode_default_groups.go`：Simple 默认组
- `backend/internal/server/middleware/backend_mode_guard.go`：Backend Mode
- `frontend/src/components/layout/AppSidebar.vue`：模式对应的 UI 能力
- `frontend/src/router/index.ts`：Simple/Backend 路由限制
