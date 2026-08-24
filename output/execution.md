# gb-538 NoiseTrace 噪声源贡献归因台执行验证

- 验证时间：2026-08-22（Asia/Tokyo）
- 实际端口：前端 `18538`，后端 `19538`，PostgreSQL `57538`，runtime smoke `20538`
- 项目名称：NoiseTrace 噪声源贡献归因台（`industrial-noise-source-attribution`）
- 实现提交：`619beae`（`feat: implement and verify industrial-noise-source-attribution`）

## 构建、测试与规模

| 检查 | 实际结果 |
| --- | --- |
| `go work sync` / `go build ./backend/...` / `go vet ./backend/...` | 通过 |
| `go test ./backend/...` / `go test -race ./backend/...` | 通过；覆盖算法、枚举、RBAC 与归因服务测试 |
| `npm --prefix frontend ci` / `typecheck` / `test -- --run` / `build` | 通过；Vitest 2 文件、4 用例通过 |
| runtime smoke | 通过，真实 Gin + SQLite 在 `20538/healthz` 返回 HTTP 200 后进程退出 |
| `project_scale.py .` | 通过：Go 功能代码 3486 行 / 42 个 `.go` 文件 |
| `docker compose config --quiet` | 通过 |

前端生产构建提示两个大于 800 kB 的代码块（ECharts/格式化依赖），不阻断运行；后续可以按路由或库拆分改善首次加载。

## Compose 与 API

`docker compose up -d --build` 后，PostgreSQL、backend 和 frontend 均显示 `healthy`。健康检查 `GET /healthz` 返回 HTTP 200。

| 验证 | 实际结果 |
| --- | --- |
| `POST /api/v1/auth/login`（engineer） | 200，取得 JWT |
| 未认证 `GET /api/v1/monitoring-points` | 401 |
| 监测点 / 测量 / 声源谱 / 归因运行列表 | 200；启动数据分别为 3 / 3 / 3 / 1 |
| `GET /api/v1/noise-measurements/1` | 200，状态为 `ready` |
| 非法测量迁移 `ready -> captured` | 409 |
| 发起人复核自身归因运行 | 403 |
| reviewer 对已复核种子运行复核 | 409，状态机正确拒绝重复复核 |

## 内置 Browser 验证

仅使用 Codex 内置 Browser（IAB），未使用外部 Chrome。

1. 打开 `http://127.0.0.1:18538`，以 `engineer / engineer123` 登录，跳转至 `/points`。
2. 检查 `/points`、`/measurements`、`/sources`、`/attribution`、`/audit`：均渲染真实 `/api/v1` 数据，不依赖静态 mock。
3. 在“监测点”页点击“新建监测点”，填写并提交 `MP-AUDIT-538 / Audit point 538`；成功提示出现，列表与统计从 3 刷新为 4。
4. 在 390 x 844 视口打开“贡献归因”页：`clientWidth=390`、`scrollWidth=390`，无横向溢出或文字遮挡。
5. Browser console 日志为空；页面交互和网络请求没有阻断错误。

截图：

- `output/browser-desktop.png`：桌面端贡献归因页
- `output/browser-mobile-390.png`：390px 移动端贡献归因页

## 提示词覆盖与修复结论

- 四个核心实体均有后端分层、PostgreSQL 数据表、Vue type/API/Pinia store/页面消费链路。
- 固定八个倍频程、能量域背景扣除、Gonum NNLS、输入冻结、算法证据、状态机、条件版本更新、JWT/RBAC、独立复核、限流、幂等与写入审计均经代码和上述运行链路检查。
- README 已包含项目边界、账号、页面工作流、架构、API、枚举位置、算法、安全边界、运行、测试和停止方式。
- 本轮未发现阻断功能、接口流程或响应式布局的问题；保留前端大包提示作为非阻断性能优化项。

## 清理

已执行 `docker compose down -v --remove-orphans`。随后分别查询容器、网络和命名卷：没有任何 `industrial-noise-source-attribution-*` 容器、`industrial-noise-source-attribution_*` 网络或同前缀命名卷残留；runtime smoke 进程亦已退出。
