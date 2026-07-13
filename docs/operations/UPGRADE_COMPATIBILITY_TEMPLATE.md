# Upstream 升级兼容性记录模板

复制本文件，为每次 upstream 集成建立一份独立记录。未填写项不得用“待定”穿过 promotion gate。

## 身份

```yaml
observed_at_utc:
environment_manifest:
old_app_tag:
old_app_sha:
old_upstream_base:
target_upstream_tag:
target_tag_object:
target_peeled_commit:
candidate_sha:
integration_branch:
release_branch:
patch_decisions: []
reviewer:
```

## Migration 与数据

| 检查 | 结论与证据 |
|---|---|
| 新增 migration 清单 | |
| expand / backfill / contract 分类 | |
| 锁表与数据规模 | |
| migration 可重试性/checksum | |
| N 与 N+1 同时运行 | |
| N+1 写入后 N 可读取 | |
| 仅镜像回滚是否安全 | |
| 需要的快照/PITR 与恢复点 | |

任何 rename/drop、收紧约束或大规模回填必须拆阶段。若只恢复旧镜像不安全，明确选择维护窗口或数据库恢复流程。

## 配置与接口

逐项记录新增、删除、默认值和向后兼容：

- 应用配置、环境变量和部署示例：
- API/JWT/API Key/session：
- OAuth、账号 credentials/extra：
- Redis key、cache 和 queue payload：
- 模型、价格、计费、余额/订阅：
- 分组、调度和固定代理：
- 前端静态资源与旧后端：
- 后台 cron/worker 和 leader lock：
- SSE/WebSocket/HTTP2/TLS：

## Patch 处置

| Patch ID | 旧 commit/patch-id | 结论 | 新 commit/upstream commit | 定向测试 | drop condition |
|---|---|---|---|---|---|
| FZ-001 | | | | | |
| FZ-002 | | | | | |
| FZ-003 | | | | | |

## 验证与发布策略

```yaml
backend_unit:
backend_integration:
golangci_lint:
frontend_lint_typecheck_test_build:
security_scans:
linux_amd64_image:
migration_rehearsal:
proxy_group_billing_e2e:
claude_synthetic:
openai_synthetic:
old_new_coexistence:
rollback_class: image-only | dual-version-compatible | database-restore-required
deployment_strategy: maintenance-window | rolling | blue-green
stop_conditions: []
observation_window:
final_decision:
```

`rolling` 或 `blue-green` 只有在 [`ROLLING_RELEASE_CONTRACT.md`](ROLLING_RELEASE_CONTRACT.md) 全部满足时才能选择。
