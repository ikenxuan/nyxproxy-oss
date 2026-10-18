# NyxProxy-OSS Web 管理面板设计

- 日期：2026-10-18
- 状态：设计已确认，待用户审阅
- 适用平台：Linux 裸机、Docker（linux/amd64、linux/arm64）
- 技术栈：Go、Next.js、Tailwind CSS、pnpm

## 1. 背景与目标

NyxProxy-OSS 目前通过 `config.yaml` 和环境变量配置。每次修改代理、网络或轮换参数都需要手动编辑文件并重启进程，Docker 用户还需要重新创建容器。排障只能看容器日志或 `journalctl`。

本次目标是添加一个内置 Web 管理面板：

- Linux 单文件部署时仍然只有一个二进制
- Docker 部署时仍然只有一个容器，运行时不依赖 Node
- 在浏览器中查看运行状态、编辑配置、保存后自动应用
- 应用失败时自动回滚，磁盘上始终保留最后一次可用配置
- 在面板中实时查看进程日志
- 界面支持中英文切换
- 保留现有配置方式和命令行用法；未启用 Web 的旧配置行为不变

## 2. 非目标

以下内容明确不在 v1 范围内：

- Windows 支持、Windows 构建产物和 Windows 文档
- 细粒度热更新（只重建受影响的代理内部组件）
- 手动地址池操作、单 IP 使用明细、连接明细
- 多用户、RBAC、审计日志
- 外部数据库；会话只存内存
- Next.js 服务端运行时、API Routes、Server Actions
- ACME 自动签发证书；内置 TLS 只接受已有证书，或由反向代理终止 TLS
- 配置文件 watcher；手动修改文件后需要重启进程
- Playwright 端到端测试
- 访问日志查看器；日志页只展示进程日志
- 历史指标图表；仪表盘只展示实时状态
- Web 密码哈希；v1 沿用配置文件明文密码 + 0600 权限

## 3. 关键决策

| 主题 | 决策 |
| --- | --- |
| 平台范围 | 只做 Linux 和 Docker，移除 Windows 构建与文档 |
| 面板形态 | 浏览器 Web 管理面板 |
| 前端技术栈 | Next.js App Router + TypeScript + Tailwind CSS |
| 包管理器 | 全链路 pnpm，提交 `pnpm-lock.yaml`，不使用其他包管理器 |
| 前端产物 | Next.js 静态导出，`go:embed` 进 Go 二进制 |
| 访问安全 | Docker 默认 `0.0.0.0:18081`，强制登录；支持可选内置 TLS 和反向代理 |
| 配置应用 | 进程内 supervisor + 组件级重启 + 事务性回滚 |
| v1 功能 | 登录、仪表盘、配置编辑、自动应用、实时日志 |
| 界面语言 | 中文 / English 切换，轻量字典，不引入 i18n 框架 |
| 配置权威来源 | Docker 首次启动后，`/data/config.yaml` 优先于环境变量 |

## 4. 总体架构

```text
cmd/proxy/main.go
└─ internal/app.Supervisor
    ├─ internal/api.Server        Web UI + REST + SSE，配置应用时保持在线
    ├─ pkg/monitoring            现有监控端点，配置变化时重启
    ├─ proxy (socks5 | https)    配置应用时整体重启
    │   └─ network.IPv6Manager
    └─ internal/logbuf           日志环形缓冲，同时写 stderr
```

### 4.1 包职责

| 位置 | 职责 |
| --- | --- |
| `cmd/proxy/main.go` | 解析 `-config`、`-healthcheck`、`-version`，初始化日志，构建 supervisor，等待信号 |
| `internal/app` | 组件生命周期、配置应用/回滚、运行状态聚合、代理工厂 |
| `internal/config` | 配置加载、校验、原子写盘、环境变量覆盖 |
| `internal/api` | REST、登录/会话/CSRF、登录限速、SSE 日志、安全响应头 |
| `internal/webui` | `go:embed` 静态前端、SPA fallback、缓存头、未构建时的占位页 |
| `internal/logbuf` | 日志 ring buffer、订阅者管理、非阻塞广播 |
| `pkg/socks5` | SOCKS5 代理，按新生命周期接口改造 |
| `pkg/https` | HTTPS 代理，按新生命周期接口改造 |
| `pkg/monitoring` | 现有监控端点，增加 `Stop()` |
| `pkg/network` | IPv6 地址池与地址管理；地址命令通过可替换 runner 执行 |
| `web/` | Next.js + Tailwind 前端源码 |

### 4.2 代理生命周期接口

```go
type Proxy interface {
    Start() error                  // 绑定监听成功后返回
    Wait() error                   // 阻塞到服务退出；正常停止返回 nil
    Stop(ctx context.Context) error // 关闭监听、drain、清理地址池
    Status() ProxyStatus
}
```

`ProxyStatus` 至少包含：状态（`starting` / `running` / `error`）、类型、监听地址、协议、启动时间、最近一次错误。

`Stop()` 触发的 `net.ErrClosed` / `http.ErrServerClosed` 在 `Wait()` 中视为正常退出。

### 4.3 启动顺序

1. 解析参数，加载并校验配置；配置本身非法时直接退出
2. 初始化日志 broker，执行 `log.SetOutput(io.MultiWriter(os.Stderr, broker))`
3. 先启动 Web/API 服务器（如果启用）
4. 启动监控服务（如果启用）；启动失败只记录警告，不阻塞 Web/代理
5. 启动代理
   - 代理启动失败且 Web 启用：进程保持运行，代理状态为 `error`，面板可以修改配置或重试
   - Web 未启用：非零退出，交给 Docker/systemd 重启
6. 收到 `SIGTERM` / `SIGINT`：先停代理（drain + 清理地址池），再停监控、Web，最后退出

先启动 Web 的原因：代理因缺少 `ip` 命令、地址池填充失败等运行时问题起不来时，面板仍然可用，能看到错误并修复，而不是整个进程直接消失。

### 4.4 关键重构

- `socks5.Proxy` 必须持有 listener 并在 `Stop()` 中关闭；当前实现只清理地址池，热重启时端口无法重新绑定
- 移除 `socks5` / `https` 的包级 `ipv6Manager` 全局变量；状态从 supervisor 当前持有的代理实例读取
- `main.go` 中轮询全局 manager 更新 metrics 的逻辑迁移到 supervisor
- 代理构造函数不再产生全局副作用；构造/启动失败时自行清理已创建的 manager
- `https.Proxy` 使用 `net.Listen` + `http.Server.Serve`，保留 listener 以支持 `Shutdown`
- `monitoring.Server` 增加 `Stop()`，由 supervisor 管理启停
- `IPv6Manager` 增加可注入的命令 runner，测试不依赖真实 `ip` 命令
- supervisor 通过代理工厂创建实例，测试注入 fake proxy，不触碰真实网络

## 5. 配置模型与持久化

### 5.1 配置结构

```yaml
proxy:
  type: socks5
  listen_address: "0.0.0.0"
  listen_port: 1080
  username: "admin"
  password: "change_me"

network:
  interface_name: "eth0"
  ipv4_enabled: false
  ipv6_enabled: true
  ipv6_subnet: "2001:db8:1:2::/64"
  dns_source_address: ""
  rotate_ipv6: true
  ipv6_pool_size: 64
  ipv6_max_usage: 100
  ipv6_max_age: 30

monitoring:
  enabled: false
  port: 9090
  allow_remote: false

logging:
  debug_level: 1

web:
  enabled: true
  listen_address: "0.0.0.0"
  port: 18081
  username: "admin"
  password: "change_me_too"
  tls_cert_file: ""
  tls_key_file: ""
  trust_proxy_headers: false
  cookie_secure: false
```

### 5.2 默认值与兼容性

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `web.enabled` | `false` | 旧配置没有 `web` 段时视为关闭，升级后行为不变 |
| `web.listen_address` | `127.0.0.1` | 裸机安全默认值 |
| `web.port` | `18081` | Docker 模板同样使用该端口 |
| `web.username` | `admin` | 启用时不能为空 |
| `web.password` | 空 | 启用时必须非空 |
| `web.tls_cert_file` / `tls_key_file` | 空 | 两者必须同时设置或同时为空 |
| `web.trust_proxy_headers` | `false` | 仅可信反代后开启，用于 `X-Forwarded-For` / `X-Forwarded-Proto` |
| `web.cookie_secure` | `false` | 内置 TLS 时自动为 true；反代终止 TLS 时手动设为 true |
| 会话有效期 | 12 小时 | 固定值，不提供配置项 |

Docker 模板覆盖 `web.enabled: true`、`web.listen_address: "0.0.0.0"`、`web.port: 18081`。

### 5.3 配置路径与优先级

- 路径优先级：`-config` 参数 > `NYXPROXY_CONFIG` 环境变量 > `./config.yaml`
- Docker 使用 `/data/config.yaml`，挂载命名卷 `nyxproxy-data`
- entrypoint 只在 `/data/config.yaml` 不存在时，用模板和环境变量生成
- 运行时覆盖：
  - `NYXPROXY_PREFIX`：覆盖 `network.ipv6_subnet`
  - `NYXPROXY_INTERFACE`：覆盖 `network.interface_name`
- 运行时覆盖只影响内存，不写回文件；UI 将对应字段显示为“由环境变量管理”并禁止编辑

### 5.4 校验规则

- `proxy.type` 必须是 `socks5` 或 `https`
- `proxy.username` / `proxy.password` 非空
- `ipv4_enabled` 和 `ipv6_enabled` 至少一个为 true
- `proxy.listen_port`、`monitoring.port`、`web.port` 必须在 1–65535
- 启用的监听器之间不能冲突（相同端口且地址重叠视为冲突）
- `dns_source_address` 非空时必须是合法 IP、不能是 link-local，且与当前出口协议一致
- `ipv6_subnet` 非空时必须是合法 IPv6 /64；为空且启用轮换时尝试自动检测
- 启用轮换时：`ipv6_pool_size > 0`、`ipv6_max_usage > 0`、`ipv6_max_age > 0`
- 启用轮换时检查 `ip` 命令是否存在；不存在时返回明确错误
- `web.enabled` 为 true 时：用户名/密码非空、监听地址合法、端口合法
- TLS 证书和私钥必须同时设置；文件必须可读
- 配置校验只做只读检查，不修改任何状态

### 5.5 持久化

- 保存时在配置同目录创建临时文件，权限 0600，写入规范化 YAML
- apply 成功后：`fsync` 临时文件 → `rename` 覆盖正式配置 → 对目录执行 `fsync`
- apply 失败或进程收到 `SIGTERM` 时删除临时文件，正式配置不变
- UI 保存输出规范化 YAML，并带一行管理头注释；原文件中的注释不会保留
- 启动时清理同目录下 `config.yaml.tmp-*` 形式的陈旧临时文件
- 配置文件权限 0600；容器内以 root 运行，`/data` 目录权限 0700

### 5.6 密码处理

- `GET /api/config` 不返回密码明文，`proxy.password` 和 `web.password` 返回空字符串
- `meta.passwords_set` 标记两个密码是否已设置
- `PUT /api/config` 中密码为空字符串表示“保持不变”，非空表示修改
- 登录密码使用常量时间比较
- API 访问日志、应用日志、错误信息都不记录密码或请求体

### 5.7 并发与外部修改

- 同一时间只允许一个 apply；第二个保存请求返回 `409 apply_in_progress`
- v1 不监听外部文件变更；手动修改 `config.yaml` 后需要重启进程，或重新在 UI 保存一次
- 手动编辑与 UI 保存同时发生时，以最后一次成功 apply 的内容为准

### 5.8 Docker 行为变化

- 首次启动用环境变量生成 `/data/config.yaml`；之后环境变量不再覆盖已有配置
- 前缀变化时 entrypoint 只更新 `NYXPROXY_PREFIX` 并重启进程，不改写 UI 保存的配置
- 重启期间面板会断开几秒；地址池由新进程重新填充
- 修改面板密码请使用 UI；如果忘记密码，删除卷或直接编辑 `/data/config.yaml`

## 6. 保存、应用与回滚

### 6.1 一次保存的完整流程

1. `TryLock`：已有 apply 在执行 → `409 apply_in_progress`；否则状态置为 `applying`
2. 纯校验新配置（含运行时覆盖后的有效配置）。失败 → `422 validation_failed` + 字段级错误，不写文件、不重启
3. 锁定字段检查：环境变量管理的字段，以及重启生效字段，修改则拒绝
4. 计算差异：
   - `proxy` / `network` / `logging` 变化 → 重启代理
   - `monitoring` 变化 → 重启监控
   - `web.username` / `web.password` 变化 → 不重启组件，提交后更新认证并使所有会话失效
   - 无变化且代理正在运行 → `200 {changed:false}`
   - 无变化但代理处于 `error` → 当作一次“用当前配置重试启动”
5. 预写临时文件：同目录 `CreateTemp`，0600，写入规范化 YAML；磁盘满/权限错误在此返回，代理不受影响
6. 停旧代理：关闭监听器 → 最多等 5s 让活跃连接结束 → `IPv6Manager.Stop()` 删除池地址。单地址清理失败只记警告，不阻塞。旧代理不存在则跳过
7. 启新代理：构造 manager（填充地址池）→ 绑定端口。任何一步失败都要清理已创建的 manager，然后进入回滚
8. 按需重启监控：失败同样触发回滚
9. 提交配置：临时文件 `fsync` → `rename` 覆盖 `config.yaml` → 目录 `fsync`
10. 更新内存：替换当前配置、`generation++`、记录 `last_applied_at`；Web 凭据变化则更新认证并让现有会话失效
11. 状态恢复 `running`，释放锁

apply 请求可能持续数十秒（地址池填充），服务端不设短写超时；前端显示 applying 状态，日志页可以看到实时进度。

### 6.2 失败与回滚

| 失败点 | 处理 |
| --- | --- |
| 校验失败 | 不写文件、不重启，返回 422 + `fields` |
| 临时文件写入失败 | 不重启，返回 `persist_failed` |
| 新代理/监控启动失败 | 停半成品 → 用旧配置重启旧组件 → 删除临时文件 → 返回 `apply_failed` + `rolled_back: true` |
| 旧组件也启动失败 | 代理状态 `error`，进程保持运行（Web 启用时），返回 `rolled_back: false` |
| 正式配置提交失败 | 停新组件 → 重启旧组件 → 删除临时文件 → 返回 `persist_failed` |
| apply 期间收到 SIGTERM | 取消流程、停组件、删除临时文件；旧配置仍有效 |

失败但回滚成功时，代理状态仍是 `running`，仪表盘显示“上次保存失败，仍在运行旧配置”。

### 6.3 状态模型

- `generation`：本次进程内启动时为 1，每次成功 apply +1
- `last_apply`：`{ok, error, at, generation}`
- `proxy.state`：`starting | running | error`
- `apply_step`：`validating | stopping | starting_proxy | starting_monitoring | committing`
- `POST /api/proxy/retry`：代理处于 `error` 时用当前内存配置重试启动；已经运行或正在启动时返回 `409 proxy_already_running`

### 6.4 drain 细节

- SOCKS5：监听器包一层计数 listener；`Stop()` 先关闭监听，等活跃连接归零或 5s 超时，再清理地址池
- HTTPS：`http.Server.Shutdown(ctx)` 使用 5s 超时，再清理地址池
- 超过 5s 的连接会被断开
- Web 面板和 API 不受代理重启影响

### 6.5 为什么先写临时文件

apply 失败时磁盘必须停留在最后一次可用配置；临时文件在成功前不会变成正式配置。磁盘满/权限问题会提前暴露，避免“先停代理再发现写不进去”。

## 7. HTTP API

### 7.1 端点

| 方法 | 路径 | 认证 | 用途 |
| --- | --- | --- | --- |
| `GET` | `/healthz` | 无 | 存活检查，Docker healthcheck 用 |
| `POST` | `/api/auth/login` | 无 | 登录 |
| `GET` | `/api/auth/session` | 会话 | 当前用户、CSRF token、过期时间 |
| `POST` | `/api/auth/logout` | 会话 + CSRF | 退出 |
| `GET` | `/api/status` | 会话 | 运行状态 |
| `GET` | `/api/config` | 会话 | 有效配置 + 元数据 |
| `PUT` | `/api/config` | 会话 + CSRF | 校验、保存、应用 |
| `POST` | `/api/proxy/retry` | 会话 + CSRF | 代理 error 时重试启动 |
| `GET` | `/api/network/interfaces` | 会话 | 网卡名、全局 IPv6 地址、建议 /64 |
| `GET` | `/api/logs/stream` | 会话 | SSE 日志流 |

静态页面和 `/_next/*` 资源不需要登录（登录页必须能加载）；认证中间件只包 `/api/*`。

### 7.2 会话与 CSRF

- 会话 ID：32 字节随机数，服务端内存 map
- 绝对过期 12 小时；最多 100 个会话，超出淘汰最旧会话；每 10 分钟清理过期会话
- Cookie：`nyxproxy_session`，`HttpOnly`、`SameSite=Strict`、`Path=/`；TLS 下自动 `Secure`
- CSRF：每个会话一个随机 token；登录和 `GET /api/auth/session` 返回
- 所有 `POST` / `PUT` / `DELETE` 必须带 `X-CSRF-Token`，服务端常量时间比较
- 变更请求校验 `Origin`：存在且与请求 host 不一致则拒绝
- 登录失败：按 IP 限速 5 次 / 5 分钟；超限 `429` + `Retry-After`；失败响应延迟 200ms
- 密码修改后清空所有会话；前端收到 `reauth_required: true` 后跳转登录页
- 反代场景：
  - `web.trust_proxy_headers: true` 时使用 `X-Forwarded-For` / `X-Forwarded-Proto`
  - 只有确认应用不直接暴露时才开启；文档强调风险
  - `web.trust_proxy_headers: true` 且 `X-Forwarded-Proto: https` 时自动视为 Secure；`web.cookie_secure: true` 作为无转发协议时的显式开关
  - 反代必须保留 `Host` 头，否则 `Origin` 校验会失败

### 7.3 配置接口

`GET /api/config` 返回：

```json
{
  "config": {
    "proxy": { "type": "socks5", "listen_address": "0.0.0.0", "listen_port": 1080, "username": "admin", "password": "" },
    "network": { "interface_name": "eth0", "ipv4_enabled": false, "ipv6_enabled": true, "ipv6_subnet": "2001:db8:1:2::/64", "dns_source_address": "", "rotate_ipv6": true, "ipv6_pool_size": 64, "ipv6_max_usage": 100, "ipv6_max_age": 30 },
    "monitoring": { "enabled": false, "port": 9090, "allow_remote": false },
    "logging": { "debug_level": 1 },
    "web": { "enabled": true, "listen_address": "0.0.0.0", "port": 18081, "username": "admin", "password": "", "tls_cert_file": "", "tls_key_file": "", "trust_proxy_headers": false, "cookie_secure": false }
  },
  "meta": {
    "path": "/data/config.yaml",
    "generation": 3,
    "locked": {
      "network.ipv6_subnet": "env",
      "network.interface_name": "env",
      "web.enabled": "restart_required",
      "web.listen_address": "restart_required",
      "web.port": "restart_required",
      "web.tls_cert_file": "restart_required",
      "web.tls_key_file": "restart_required",
      "web.trust_proxy_headers": "restart_required",
      "web.cookie_secure": "restart_required"
    },
    "passwords_set": { "proxy.password": true, "web.password": true }
  }
}
```

`PUT /api/config` 的响应：

```json
{
  "changed": true,
  "generation": 4,
  "restarted": { "proxy": true, "monitoring": false },
  "reauth_required": false
}
```

约束：

- 请求体上限 1 MiB
- `Content-Type` 必须是 `application/json`
- JSON 解码禁止未知字段
- 锁定字段如果传回的值与当前有效值相同则忽略并保持文件原值，不同则 `422 field_locked`

### 7.4 状态接口

```json
{
  "version": "2c7a64e",
  "uptime_seconds": 123,
  "proxy": {
    "state": "running",
    "type": "socks5",
    "listen": "0.0.0.0:1080",
    "protocol": 6,
    "error": ""
  },
  "monitoring": {
    "enabled": false,
    "state": "stopped",
    "error": ""
  },
  "pool": {
    "enabled": true,
    "size": 64,
    "prefix": "2001:db8::/64",
    "rotations": 12,
    "max_usage": 100,
    "max_age_minutes": 30
  },
  "metrics": {
    "active_connections": 3,
    "total_connections": 100,
    "total_requests": 230,
    "bytes_sent": 1234,
    "bytes_received": 5678
  },
  "config": {
    "path": "/data/config.yaml",
    "generation": 3,
    "last_applied_at": "2026-10-18T12:00:00Z",
    "last_apply": { "ok": true, "error": "", "at": "2026-10-18T12:00:00Z", "generation": 3 }
  },
  "platform": {
    "os": "linux",
    "ip_command": true,
    "warnings": []
  },
  "apply": {
    "in_progress": false,
    "step": ""
  }
}
```

`monitoring.state` 取值为 `stopped | running | error`；启动失败只记录警告，不阻塞代理。

`GET /api/network/interfaces` 返回：

```json
[
  {
    "name": "eth0",
    "addresses": ["2001:db8:1:2::1/64"],
    "suggested_prefix": "2001:db8:1:2::/64"
  }
]
```

### 7.5 错误模型

```json
{
  "error": {
    "code": "validation_failed",
    "message": "configuration is invalid",
    "fields": {
      "proxy.listen_port": "must be between 1 and 65535"
    }
  }
}
```

| code | HTTP | 场景 |
| --- | --- | --- |
| `invalid_credentials` | 401 | 用户名或密码错误 |
| `unauthenticated` | 401 | 无会话或会话过期 |
| `csrf_failed` | 403 | token 缺失或不匹配 |
| `origin_mismatch` | 403 | Origin 与 host 不一致 |
| `rate_limited` | 429 | 登录失败次数超限 |
| `validation_failed` | 422 | 配置校验失败，带 `fields` |
| `field_locked` | 422 | 修改了环境变量管理或重启生效字段 |
| `restart_required` | 422 | 修改了 Web 监听/TLS 配置 |
| `apply_in_progress` | 409 | 并发保存 |
| `apply_failed` | 500 | 应用失败，带 `rolled_back` |
| `persist_failed` | 500 | 配置文件写入/替换失败 |
| `proxy_already_running` | 409 | retry 时代理已经运行或正在启动 |
| `payload_too_large` | 413 | 请求体超过 1 MiB |
| `unsupported_media_type` | 415 | 非 JSON 请求 |
| `method_not_allowed` | 405 | 路径存在但方法不允许 |
| `not_found` | 404 | 未知路径 |
| `internal_error` | 500 | 未分类错误 |

前端按 `code` 查 i18n 字典，`fields` 映射到表单字段；未知 code 显示服务端 `message`。

### 7.6 安全响应头

- CSP：`default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Referrer-Policy: no-referrer`
- TLS 启用时加 HSTS；TLS 最低 1.2
- HTML 响应 `Cache-Control: no-store`；`/_next/static/*` 长缓存
- API 访问日志不写请求体；访问日志不进入 SSE 日志流

## 8. SSE 日志流

### 8.1 日志捕获

- 启动时执行 `log.SetOutput(io.MultiWriter(os.Stderr, broker))`
- Broker 实现 `io.Writer`，按 `\n` 拆行，保留跨 `Write` 调用的半行缓冲区
- 每条记录：`{seq uint64, ts time.Time, line string}`；`seq` 进程内单调递增
- 环形缓冲 2000 行，超出覆盖最旧记录
- 代理、监控、Web、TLS 握手错误都会进入日志流
- 现有 `fmt.Printf` 形式的运行日志改为 `log.Printf`，否则不会进入 SSE
- HTTP 访问日志单独写 stderr，不进入 broker

### 8.2 订阅模型

- 每个 SSE 连接一个 256 容量的缓冲 channel
- broker 写入时非阻塞发送；channel 满说明客户端太慢，直接移除该订阅者并关闭 channel，绝不阻塞日志写入
- 客户端断开后 EventSource 自动带 `Last-Event-ID` 重连，broker 从环形缓冲补发缺失行
- 最多 8 个并发日志流，超出返回 503
- 密码修改/会话失效后，心跳时检测会话不存在则关闭流

### 8.3 协议

```text
event: log
id: 42
data: {"seq":42,"ts":"2026-10-18T12:00:00Z","line":"2026/10/18 12:00:00 starting socks5 proxy on 0.0.0.0:1080 (Protocol: IPv6)"}
```

- 初次连接无 `Last-Event-ID`：先补发整个环形缓冲，再推新日志
- `Last-Event-ID` 仍在缓冲范围内：只补发 `seq > last` 的记录
- 已经覆盖：先发 `event: gap`，`data: {"dropped":37}`，再补发现有缓冲
- 每 15s 发一次注释心跳 `: ping`
- 单连接最长 30 分钟，到时主动关闭让浏览器重连
- 响应头：`Content-Type: text/event-stream`、`Cache-Control: no-cache, no-transform`、`X-Accel-Buffering: no`
- 反向代理文档注明 nginx 需要 `proxy_buffering off` 和足够的 `proxy_read_timeout`；Caddy 默认可用

## 9. 前端

### 9.1 技术形态

- Next.js App Router + TypeScript + Tailwind CSS，版本在 `package.json` / `pnpm-lock.yaml` 锁定
- Node 24 LTS；`package.json` 中 `packageManager: "pnpm@12.9.1"`
- 交互页面是客户端组件；`output: 'export'` 静态导出，由 Go 同源提供
- 开发时 `next dev` 不使用 export 模式，通过 `rewrites` 把 `/api` 代理到本机 Go 进程；生产构建通过环境变量切换为 export，避免静态导出不支持 rewrites 的问题
- 不引入 shadcn/Radix、Redux、Zustand、SWR；使用少量自建 UI 组件 + `lucide-react` 图标

### 9.2 路由与布局

```text
web/app/
├─ (auth)/login/page.tsx
├─ (app)/layout.tsx        # 会话检查 + 侧边栏 + 顶栏
├─ (app)/page.tsx          # 仪表盘
├─ (app)/config/page.tsx
└─ (app)/logs/page.tsx
```

桌面左侧栏 + 顶栏；移动端折叠为顶部菜单。顶栏包含页面标题、连接状态、语言切换、主题切换、退出。

### 9.3 仪表盘

- 代理卡片：状态徽章、类型、监听地址、协议、uptime；`error` 时显示错误详情和“重试启动”
- 地址池卡片：是否启用、池大小、当前前缀、已轮换次数、max usage / max age
- 流量卡片：活跃连接、累计连接、请求数、收发字节
- 配置卡片：generation、上次应用时间、配置文件路径
- 警告横幅：`ip` 命令缺失、上次保存失败、代理未运行
- 5s 轮询 `/api/status`；页面隐藏时暂停，恢复可见时立即刷新

### 9.4 配置页

- 分区：Proxy、Network、Rotation、Monitoring、Logging、Web Console、Advanced
- 顶部状态条：未保存修改 / applying + 当前步骤 / 上次应用结果
- 锁定字段显示徽章并禁用：`由环境变量管理`、`重启生效`
- 密码输入框：已设置时 placeholder 为“已设置，留空保持不变”；保存成功后清空
- 网卡字段用下拉框，数据来自 `GET /api/network/interfaces`
- 客户端先做必填/端口/前缀格式校验，服务端 `fields` 错误回填到对应输入框
- 保存时如果 diff 涉及 `proxy` / `network` / `logging`，弹确认框提示“会重启代理并中断现有连接”
- 保存后按钮进入 applying，轮询状态显示步骤；成功 toast + generation 更新；失败保留表单内容并展示错误
- “重新加载当前配置”丢弃本地修改，重新 GET
- 有未保存修改时：`beforeunload` 警告 + 内部导航确认
- 修改 Web 用户名/密码成功后，响应带 `reauth_required`，跳转登录页

### 9.5 日志页

- SSE 日志流，暂停自动滚动、清空视图、文本过滤、连接状态
- 页面最多保留 2000 条 DOM 节点，超出从顶部丢弃
- 暂停时只停止自动滚动并显示“有 N 条新日志”，不停止接收
- `onerror` 时先查 `/api/auth/session`，401 就关闭连接并跳转登录

### 9.6 i18n

- `web/lib/i18n/zh.ts`、`en.ts`
- `MessageKey = keyof typeof zh`，`en` 用 `Record<MessageKey, string>` 保证不漏词条
- Context 提供 `t(key, vars?)`
- 语言存 `localStorage`，首次跟随 `navigator.language`
- 切换时同步 `<html lang>`
- 日期/数字使用 `Intl`
- 后端错误按 `code` 映射：`t('errors.' + code)`，未知 code 显示服务端 `message`

### 9.7 主题与组件

- 浅色/深色主题，默认跟随系统，可手动切换并记忆
- `components/ui/`：Button、Input、Select、Switch、Card、Badge、Banner、Toast、Dialog、Skeleton
- 统一 focus ring、键盘操作、`aria-live` toast、对比度达标
- 表单状态用 `useState` / `useReducer`；认证、i18n、主题用 Context

### 9.8 API 客户端

- `lib/api.ts` 统一处理 JSON、错误 envelope、401 跳转登录
- CSRF token 只放内存（AuthContext），页面刷新后通过 `/api/auth/session` 重新获取
- apply 请求不设客户端超时，由服务端在流程结束后返回

### 9.9 前端测试

- Vitest + React Testing Library
- 覆盖：i18n 词条完整性、密码 sentinel、dirty diff、API 错误映射、登录表单、配置表单锁定字段、日志 store 上限/gap
- CI 执行 `pnpm lint`、`pnpm test`、`pnpm build`
- v1 不引入 Playwright；完整流程由 Go 的 httptest 集成测试覆盖

## 10. Docker、构建与部署

### 10.1 构建链路

- `web/`：`pnpm install --frozen-lockfile` → `pnpm build` → `web/out`
- `scripts/build-web.sh`：清空并复制到 `internal/webui/dist`
- `internal/webui/dist/.gitkeep` 提交，其余产物 gitignore
- 没有前端产物时 Go 仍可编译；`/` 返回内置的 “UI not built” 页面
- Makefile 增加 `web`、`build`、`test`、`lint`、`docker` 目标，全部使用 pnpm
- Go 1.27（`go.mod`、Dockerfile、CI 一致）

### 10.2 Dockerfile 三阶段

```dockerfile
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /app/web
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /app/web/out ./internal/webui/dist
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w" -o /out/nyxproxy ./cmd/proxy

FROM alpine:3.22
RUN apk add --no-cache iproute2 ca-certificates tzdata
COPY --from=builder /out/nyxproxy /usr/local/bin/nyxproxy
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh
COPY docker/config.template.yaml /etc/nyxproxy/config.template.yaml
HEALTHCHECK CMD ["/usr/local/bin/nyxproxy", "-healthcheck", "-config", "/data/config.yaml"]
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
```

- 两个构建阶段都运行在 `$BUILDPLATFORM`，多架构时前端构建和 Go 交叉编译都不走 QEMU
- 版本注入沿用现有 ldflags 方式，在实现时补齐具体的 `-X` 参数
- `.dockerignore` 排除 `web/node_modules`、`web/.next`、`web/out`、`internal/webui/dist`
- 运行时以 root 运行（需要 `NET_ADMIN` 修改地址）
- `read_only` + `/run` tmpfs；`/data` 是唯一可写卷
- `-healthcheck` 对回环地址跳过证书校验，便于内置 TLS 场景
- 如果 pnpm 拦截 Tailwind 等原生依赖的 build script，在 `package.json` 的 `pnpm.onlyBuiltDependencies` 显式允许

### 10.3 entrypoint

- `CONFIG_PATH` 默认 `/data/config.yaml`
- 文件不存在时用模板 + 环境变量渲染（模板增加 `web` 段）；存在时直接使用
- 继续做前缀检测，导出 `NYXPROXY_PREFIX` / `NYXPROXY_INTERFACE` 后启动 `nyxproxy -config "$CONFIG_PATH"`
- 前缀变化时重启子进程
- 子进程异常退出按 2s → 10s 退避重启
- `SIGTERM` / `SIGINT` 转发给子进程，等待它清理地址池、关闭 Web 会话后再退出

### 10.4 compose 与环境变量

- 新增命名卷 `nyxproxy-data:/data`
- 新增环境变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `WEB_ENABLED` | `true` | 是否启用面板 |
| `WEB_LISTEN` | `0.0.0.0` | 面板监听地址 |
| `WEB_PORT` | `18081` | 面板端口 |
| `WEB_USER` | `admin` | 面板用户名 |
| `WEB_PASS` | 无 | 面板密码，`WEB_ENABLED=true` 时必填 |
| `WEB_TLS_CERT_FILE` | 空 | 容器内证书路径 |
| `WEB_TLS_KEY_FILE` | 空 | 容器内私钥路径 |

- host 网络下 `WEB_PORT` 直接暴露在宿主机；文档写清防火墙/反代要求
- `read_only`、`cap_drop ALL` + `NET_ADMIN`、`no-new-privileges`、`tmpfs /run` 保持
- TLS 证书通过只读 bind mount 挂进容器，例如 `./certs:/certs:ro`
- 首次启动后修改 `.env.docker` 不会修改已有配置；改面板密码用 UI，忘记密码只能删卷或直接编辑 `/data/config.yaml`

### 10.5 裸机与 systemd

- 配置放 `/etc/nyxproxy/config.yaml`，目录 0700、文件 0600
- `ExecStart=/usr/local/bin/nyxproxy -config /etc/nyxproxy/config.yaml`
- 裸机默认建议 `web.listen_address: 127.0.0.1`，远程使用 SSH 隧道或反向代理
- `quick-setup.sh` 可选启用面板，并提示设置密码
- 配置非法时进程直接退出；systemd 会反复重启，面板也起不来，需要修文件或删掉配置文件让安装脚本重新生成

### 10.6 CI、发布与文档

- `ci.yml`：移除 Windows 构建；增加 web job（`pnpm lint/test/build`）和 Go job（`go test -race`、`go vet`、构建）；路径过滤加入 `web/**`；Go 构建 job 依赖 web job 的产物，或在同一 job 内串行执行 `scripts/build-web.sh`
- `docker.yml`：多架构构建使用 `$BUILDPLATFORM` 优化，保留 GHA cache；路径过滤加入 `web/**`
- `release.yml`：先执行前端构建并复制到 `internal/webui/dist`，再构建 Go 二进制；只发布 `linux-amd64` / `linux-arm64`，移除 Windows 产物及校验文件中的对应行
- `README.md` / `README-ZH.md`：增加 Web Console、Docker 卷与端口、环境变量、安全说明；删除 Windows 段落
- `docs/CONFIG.md`：补充 `web` 段字段
- `quick-setup.sh`：增加可选 Web 配置
- 全仓搜索 `windows` / `*.exe` 引用并清理残留

## 11. 测试策略与验收

### 11.1 Go 测试

- 按 TDD：每个新包先写失败测试
- supervisor 使用可注入的 proxy factory / fake address manager，不依赖真实 `ip` 命令
- `IPv6Manager` 的地址增删通过可替换的 command runner 测试，同时保留真实实现
- 覆盖重点：
  - config：路径/优先级、校验矩阵、锁定字段、密码脱敏、原子写
  - supervisor：diff 分类、成功 apply、失败回滚、回滚失败、并发 409、error 状态重试
  - API：会话生命周期、CSRF、Origin、登录限速、配置 GET/PUT、status、retry、interfaces、安全头
  - logbuf/SSE：顺序、环形覆盖、Last-Event-ID、gap、慢客户端、心跳
  - proxy：SOCKS5 listener 关闭与 drain、HTTPS Shutdown、失败时地址池清理、全局变量移除
- 命令：`go test -race ./...`、`go vet ./...`、`gofmt`

### 11.2 前端测试

- `pnpm lint`
- `pnpm test`（Vitest + React Testing Library）
- `pnpm build`
- i18n 词条完整性作为测试
- 覆盖：密码 sentinel、dirty diff、API 错误映射、登录表单、配置表单锁定字段、日志 store 上限/gap

### 11.3 集成与部署测试

- Go httptest 覆盖 API + supervisor + fake proxy 的完整保存/回滚流程
- 可选 CI smoke：ubuntu runner 上创建 dummy0，运行容器（pool=1），验证 `/healthz`、登录、`/api/status`、退出后地址被清理
- entrypoint 运行 `shellcheck`
- Docker 不可用的本地环境用 YAML 解析和单元测试替代；CI 负责真实镜像构建

### 11.4 验收标准

1. 全链路 pnpm，CI 与 Docker 均不依赖其他包管理器
2. `go test -race ./...`、`go vet ./...`、`pnpm lint`、`pnpm test`、`pnpm build` 全绿
3. 多架构镜像可构建，前端嵌入，运行时不包含 Node
4. 登录、CSRF、限速、配置保存、自动应用、失败回滚、SSE 日志均有测试覆盖
5. 旧配置不启用 web 时行为与现在一致
6. Windows 构建与文档清理完成，README 中英文同步

## 12. 迁移、兼容与风险

### 12.1 迁移

- 旧配置没有 `web` 段：面板默认关闭，行为不变
- Docker 首次启动用现有环境变量生成 `/data/config.yaml`，老的环境变量用法继续有效
- 裸机默认仍是 `./config.yaml`；自定义路径用 `-config`
- Windows 产物和文档段落删除；release 只保留 linux/amd64 + arm64
- UI 首次保存会重写 YAML，原注释不保留

### 12.2 已知限制

- 会话只存内存，进程重启后需要重新登录
- 配置文件中的密码是明文；依赖 0600 权限和主机访问控制
- 静态导出的 CSP 需要 `'unsafe-inline'`
- 保存配置会重建地址池，可能造成数秒到数十秒的代理中断
- 不监听配置文件变化
- 不提供历史指标和单 IP 明细
- 不提供 ACME 自动证书

### 12.3 风险与缓解

| 风险 | 缓解 |
| --- | --- |
| 当前 SOCKS5 库没有 Shutdown API | 计数 listener + 5s drain；如果行为不理想，后续换库 |
| 静态导出 CSP 需要 `'unsafe-inline'` | 管理面板无第三方内容，文档说明 |
| 地址池重建耗时 | 前端显示 applying，日志页显示进度；接口不设短超时 |
| Docker smoke 依赖 runner 的 NET_ADMIN/dummy 接口 | 不稳定时降级为 nightly/manual |
| 反代下 `trust_proxy_headers` 配错 | 默认关闭，文档强调只在可信反代后开启 |
| pnpm 拦截原生依赖 build script | `pnpm.onlyBuiltDependencies` 显式允许 |

### 12.4 后续可选事项（不属于 v1）

- 把 Go module 路径统一为当前仓库路径
- 用 bcrypt 存储 Web 密码
- ACME 自动证书
- Playwright E2E
- 手动地址池操作和单 IP 明细
- 历史指标图表

## 13. 实施顺序（高层）

1. 代理生命周期重构：listener 所有权、`Start/Wait/Stop/Status`、移除全局 manager、monitoring `Stop()`
2. 配置模型：`web` 段、路径/覆盖、校验、原子写、密码脱敏
3. supervisor：apply/rollback、diff 分类、状态聚合、retry
4. 日志 broker 与 SSE
5. API + 认证 + 安全头
6. 前端：pnpm/Next/Tailwind 脚手架、页面、i18n、API 客户端
7. 构建链路：静态导出 + embed + Makefile
8. Docker：entrypoint、模板、compose、healthcheck
9. CI/release/文档、移除 Windows
10. 全量测试与端到端验证
