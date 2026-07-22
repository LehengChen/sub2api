# 外部发布控制与只读版本目录

本文定义由外部运维系统管理应用工件时，Sub2API 管理面板与后端必须遵守的通用契约。生产身份、实际镜像地址、AWS 资源和 release manifest 只能记录在私有运维仓库。

## 为什么需要外部控制模式

应用内原有更新路径会查询 `Wei-Shaw/sub2api` 的 GitHub release、下载二进制并在本机原子替换；在线回退也依赖本地 `.backup` 或重新下载旧 release。该方式适合明确选择 self-managed 的单机二进制部署，但不适用于不可变容器：容器重建后，可写层中的替换会消失，且它不能证明 Frenzy app tag、完整源码 SHA、镜像 digest 和私有 ops revision 已经过批准。

外部控制模式把职责分开：

- 应用只读取并显示非敏感 release identity；
- 私有发布控制器构建、扫描、签名、暂存、激活和回退不可变镜像；
- 管理面板不能下载、替换、回退二进制或触发服务重启；
- catalog 不可用、字段不完整或结果来自缓存时，界面必须显示 warning，不能显示“已是最新版本”。

这不是新的业务 `run_mode`。`simple` / `standard`、Backend Mode 和 `deployment control mode` 相互独立。

## 运行配置

默认值保持 `self_managed`，以兼容现有本地和 standalone 安装。外部运维部署必须显式设置：

```text
SUB2API_DEPLOYMENT_CONTROL_MODE=externally_managed
```

允许值只有：

| 值 | 行为 |
|---|---|
| `self_managed` | 保留原有 GitHub 检查、应用内更新、回退和重启能力 |
| `externally_managed` | 仅允许只读版本检查；update、rollback、restart 由后端返回 `403 EXTERNAL_DEPLOYMENT_MANAGED` |

未知值会阻止服务启动，不能静默回退为 `self_managed`。该开关从进程环境读取，不进入数据库设置，管理员不能通过面板解除生产部署保护。

## 批准版本目录

外部部署通过以下非敏感环境变量传入批准候选。它们必须来自同一份不可变 release manifest：

| 环境变量 | 含义 | 校验 |
|---|---|---|
| `SUB2API_RELEASE_CATALOG_SOURCE` | catalog 的逻辑来源 | 受限 token，不能放 secret 或带凭证 URL |
| `SUB2API_RELEASE_CATALOG_REVISION` | catalog/manifest revision | 受限 token |
| `SUB2API_RELEASE_CATALOG_VERSION` | 批准的应用版本 | 受限 token，不代表 upstream `latest` |
| `SUB2API_RELEASE_APP_TAG` | 不可变 `frenzy/app/*` tag | 受限 token |
| `SUB2API_RELEASE_SOURCE_REPOSITORY` | 源码仓库身份 | 受限 token |
| `SUB2API_RELEASE_SOURCE_REVISION` | 完整源码 commit SHA | 40 到 64 位十六进制，禁止短 SHA |
| `SUB2API_RELEASE_IMAGE_TAG` | 人类可读的不可变镜像 tag | 受限 token，生产仍以 digest 固定 |
| `SUB2API_RELEASE_IMAGE_DIGEST` | 镜像内容身份 | 精确 `sha256:` 加 64 位十六进制 |
| `SUB2API_RELEASE_OPS_REVISION` | 私有运维仓库 revision | 受限 token |

外部模式允许先以“不完整 catalog、所有写能力关闭”的状态部署控制代码，便于分阶段迁移；此时 API 返回 `catalog_status=incomplete`、`check_status=unconfigured` 和 warning。缺失 metadata 永远不能恢复 update/rollback/restart 能力。已提供但格式非法的完整 SHA、digest 或 token 会阻止启动。

这些变量都不是秘密。不得把 token、Cookie、密码、API Key、SecretString、Terraform state 或带认证参数的 URL 放入 catalog。

## 管理 API 契约

`GET /api/v1/admin/system/check-updates` 继续作为统一只读入口。响应新增：

```json
{
  "deployment_mode": "externally_managed",
  "managed_externally": true,
  "check_status": "managed",
  "catalog_status": "valid",
  "capabilities": {
    "check_updates": true,
    "update": false,
    "rollback": false,
    "restart": false
  },
  "catalog": {
    "source": "release-catalog",
    "version": "x.y.z",
    "app_tag": "frenzy/app/...",
    "source_revision": "full-commit-sha",
    "image_digest": "sha256:...",
    "ops_revision": "ops-revision"
  }
}
```

外部模式不会请求 upstream GitHub，也不会把 upstream latest 当作批准版本。下列写路径在获取系统操作锁、下载文件或启动后台重启前即被拒绝，并且 service 层重复执行相同保护：

- `POST /api/v1/admin/system/update`
- `POST /api/v1/admin/system/rollback`
- `POST /api/v1/admin/system/restart`
- `GET /api/v1/admin/system/rollback-versions`，因为旧实现的数据源是 upstream release，而不是批准 catalog

`cached=true` 或 `check_status=error|unconfigured` 时，调用方必须把结果视为“未核验”，不能从 `has_update=false` 推导“已是最新”。

## 发布与回退边界

启用外部模式不等于发布已经自动化，也不授权部署。正式应用升级仍必须：

1. 从私有 stable manifest 恢复当前 app tag、完整 SHA、镜像 digest 和 ops revision；
2. 从明确 upstream 正式 tag 创建 `integration/*`，逐项处置 `PATCH_QUEUE.md`；
3. 完成 migration、N/N-1、API/OAuth、计费、调度、Redis、长连接、代理/TLS 和安全验证；
4. 创建不可变 `release/*` 与 annotated `frenzy/app/*` tag；
5. 构建并固定 `linux/amd64` 镜像 digest；
6. 在私有 ops 中写入批准 catalog/manifest，预拉取候选、执行 readiness 和 synthetic 后才切换流量；
7. 回退通过前一个批准 digest 和兼容数据库契约完成，不调用应用内 rollback。

外部版本控制、应用镜像升级、数据库 migration、数据层 HA 和多 Center 切换必须继续作为独立变更轴审批和回滚。

## 最小测试

- 缺失变量：默认仍为 `self_managed`；显式 external 时能力全部 fail-closed。
- 非法 mode、短源码 SHA、错误 digest：启动配置校验失败。
- external + 完整 catalog：只读检查不访问 GitHub，返回完整 identity 和只读 capabilities。
- external + 不完整 catalog：显示 warning，不显示“已是最新”。
- update、rollback、restart、rollback-version listing：后端均返回 403，且未执行下载、锁或重启。
- cached 或检查失败：store 保留 warning/status，版本组件显示未核验状态。
- self-managed：现有单机行为保持兼容。
