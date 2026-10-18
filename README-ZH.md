# NyxProxy-OSS

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go)](https://go.dev/)
[![CI](https://github.com/ikenxuan/nyxproxy-oss/actions/workflows/ci.yml/badge.svg)](https://github.com/ikenxuan/nyxproxy-oss/actions/workflows/ci.yml)
[![Latest Release](https://img.shields.io/github/v/release/ikenxuan/nyxproxy-oss)](https://github.com/ikenxuan/nyxproxy-oss/releases)

**高性能代理服务器，支持自动 IPv6 轮换** —— 从你自己的 /64 子网中为每个请求获取一个新的 IPv6 地址！

[English](README.md) | 中文

---

## 📋 目录

- [为什么选择 NyxProxy？](#-为什么选择-nyxproxy)
- [核心特性](#-核心特性)
- [IPv6 轮换是如何工作的](#-ipv6-轮换是如何工作的)
- [快速开始](#-快速开始)
  - [Docker（推荐）](#docker推荐)
  - [一键安装（Debian/Ubuntu）](#一键安装debianubuntu)
  - [卸载（Debian/Ubuntu）](#卸载debianubuntu)
  - [手动安装](#手动安装)
- [配置](#-配置)
- [以守护进程运行（Systemd）](#-以守护进程运行systemd)
- [Docker 部署](#-docker-部署)
- [Web 管理面板](#-web-管理面板)
- [使用示例](#-使用示例)
- [IPv6 轮换详解](#-ipv6-轮换详解)
- [环境要求](#-环境要求)
- [故障排查](#-故障排查)
- [许可证](#-许可证)

---

## 🎯 为什么选择 NyxProxy？

NyxProxy 是一个**为每个请求分配不同 IPv6 地址**的代理服务器 —— 全自动完成！

### 问题

大多数代理只使用一个 IP 地址，这会让你：
- ❌ 容易被跨请求追踪
- ❌ 容易受到限流
- ❌ 容易被指纹识别
- ❌ 请求过多后被封禁

### 解决方案：IPv6 轮换

NyxProxy 利用服务商提供的 **IPv6 /64 子网**（约 1800 万亿个地址！）自动轮换：
- ✅ **每个请求使用不同 IP** —— 看起来像不同用户
- ✅ **不受限流影响** —— 每个 IP 都是"新鲜"且未使用过的
- ✅ **抗指纹识别** —— 无法跨请求追踪
- ✅ **智能轮换** —— IP 在使用 100 次或 30 分钟后被替换
- ✅ **快速** —— 预填充 200 个 IP 的地址池，随时可用

### 实际使用场景

- **网页抓取**：通过表现为不同用户来避免限流
- **API 测试**：从不同 IP 模拟多个"用户"进行测试
- **隐私保护**：每个请求看起来都来自不同来源
- **负载分散**：将负载分散到多个 IP

---

## ✨ 核心特性

### 🔄 自动 IPv6 轮换（核心功能！）

- 轮换池中包含 **200+ 个 IPv6 地址**
- **智能轮换**：使用 100 次或 30 分钟后替换 IP
- **完全可配置**：可调整池大小、轮换频率和存活时间
- **后台刷新**：旧 IP 会被自动替换
- **无需人工干预**：设置一次，之后无需管理

### 🚀 性能

- **每个请求 < 100ms** —— 预填充 IP 池
- **并发连接** —— 可处理数千个连接
- **高效轮换** —— 无延迟、不阻塞

### 🔧 灵活配置

- **SOCKS5 或 HTTPS 代理** —— 可自行选择协议
- **支持 IPv4 + IPv6** —— 双栈能力
- **网卡选择** —— 自动检测或手动指定
- **认证** —— 用户名/密码保护
- **监控端点** —— 内置健康检查

### 🐧 Linux 原生

- **Debian/Ubuntu** —— 一键安装
- **Systemd 服务** —— 以守护进程运行
- **开机自启** —— 可靠部署
- **支持 ARM64** —— 兼容树莓派

### 🐳 容器就绪

- **多架构镜像** —— 发布到 GHCR，支持 `linux/amd64` 和 `linux/arm64`
- **自动处理前缀** —— 等待 IPv6 `/64`，前缀变化时自动重启
- **干净退出** —— 收到 `SIGTERM` 时删除池地址
- **内置 Compose 文件** —— host 网络、`NET_ADMIN`、只读根文件系统

### 🖥️ 内置 Web 管理面板
- **浏览器管理** —— 仪表盘、配置编辑器和实时日志
- **事务性应用** —— 非法配置在停止任何组件前被拒绝；启动失败会自动回滚
- **单二进制、单容器** —— Next.js 界面已嵌入，生产环境不依赖 Node
- **默认安全** —— 密码登录、CSRF 防护、登录限流、可选内置 TLS

---

## 🔍 IPv6 轮换是如何工作的

### 理解 IPv6 /64 子网

当你租用一台带有已路由 IPv6 地址块的服务器时，通常会得到：
- 1 个主 IPv6 地址（例如 `2a05:f480:1800:25db::1`）
- 1 个完整的 **/64 子网**（例如 `2a05:f480:1800:25db::/64`）

**/64 意味着什么？**
- 前 64 位是你的子网前缀
- 后 64 位可以是**任意值**
- 也就是 `2^64 = 18,446,744,073,709,551,616` 个可能的 IPv6 地址！

### NyxProxy 的智能轮换

```
你的 /64 子网: 2a05:f480:1800:25db::/64
               └─────────┬──────────┘└─可以是任意值─┘
                   前缀（固定）           你的 IP（1800 万亿个！）

NyxProxy 生成随机 IP：
├─ 2a05:f480:1800:25db:1a2b:3c4d:5e6f:7890  ← 请求 1
├─ 2a05:f480:1800:25db:9988:7766:5544:3322  ← 请求 2
├─ 2a05:f480:1800:25db:aaaa:bbbb:cccc:dddd  ← 请求 3
└─ ...（池中 200 个 IP，自动轮换）
```

### 三个阶段

**阶段 1：启动（20-30 秒）**
```
1. 从你的 /64 子网生成 200 个随机 IP
2. 将每个 IP 添加到网络接口
3. 完成！所有 IP 可立即使用
```

**阶段 2：请求处理（< 100ms）**
```
1. 新的代理请求到达
2. 从池中选择下一个 IP（轮询）
3. 将出站连接绑定到该 IP
4. 记录使用情况（count++，lastUsed = now）
```

**阶段 3：后台轮换（每 5 分钟）**
```
检查每个 IP：
  - 已使用 100+ 次？ → 替换为新 IP
  - 存活超过 30 分钟？ → 替换为新 IP
  - 否则：保留
```

---

## 🚀 快速开始

### Docker（推荐）

```bash
cp .env.docker.example .env.docker
# 编辑 .env.docker：设置 IFACE、PROXY_PASS 和 WEB_PASS

docker compose pull
docker compose up -d
docker compose logs -f nyxproxy
```

打开 `http://你的服务器:18081`，使用 `.env.docker` 中的 `WEB_USER` 和
`WEB_PASS` 登录。

compose 文件默认拉取 `ghcr.io/ikenxuan/nyxproxy-oss:latest`。详细说明见
[Docker 部署](#-docker-部署)。

### 一键安装（Debian/Ubuntu）

最简单的上手方式 —— 直接把安装脚本交给 `bash` 运行：

```bash
curl -fsSL https://raw.githubusercontent.com/ikenxuan/nyxproxy-oss/main/scripts/quick-setup.sh | sudo bash
```

脚本直接在当前会话中运行，提示信息会从终端读取，不会在当前目录留下文件。二进制
会先下载到脚本内部的临时目录，在 `sha256sums.txt` 可用时校验通过后才安装到
`/usr/local/bin/nyxproxy`。

**就这样！** 脚本会：
1. 自动检测网络接口和 IPv6 子网
2. 安装并配置 ndppd（NDP 代理守护进程）
3. 设置 IPv6 路由所需的内核参数
4. 生成 `/etc/nyxproxy/config.yaml`（权限 0600）
5. **从 GitHub Releases 下载最新的 NyxProxy 二进制文件**
6. **可选：安装为 systemd 服务**（守护进程模式）
7. **可选：立即启动代理**

安装的文件：

| 路径 | 用途 |
| --- | --- |
| `/usr/local/bin/nyxproxy` | 二进制文件 |
| `/etc/nyxproxy/config.yaml` | 配置文件 |
| `/etc/systemd/system/nyxproxy.service` | systemd 服务（可选） |
| `/etc/sysctl.d/99-nyxproxy-ipv6.conf` | 内核参数 |
| `/etc/ndppd.conf` | ndppd 配置（原文件备份到 `/etc/ndppd.conf.nyxproxy.bak`） |
| `/var/lib/nyxproxy/install-state` | `--uninstall` 使用的回滚信息 |

**安装过程中会询问：**
- 代理用户名（默认：admin）
- 代理密码（必填）
- IP 池大小（默认：200）
- 每个 IP 的最大使用次数（默认：100）
- IP 存活时间上限（分钟，默认：30）
- 是否启用 Web 面板？(y/N)
- Web 面板用户名、密码和端口（默认：18081）
- 是否安装为 systemd 服务？(y/N)
- 是否立即启动 NyxProxy？(Y/n)

**预期输出：**
```
✓ IPv6 rotation mode: IP Pool with dynamic rotation
  Interface: enp1s0
  Subnet: 2a05:f480:1800:25db::/64
  Pool size: 200 IPs
  Rotation: Every 100 uses or 30m0s
  Initializing IP pool...
  Progress: 50/200 IPs added
  Progress: 100/200 IPs added
  Progress: 150/200 IPs added
  Progress: 200/200 IPs added
✓ IP pool ready with 200 addresses
✓ Background IP rotation started

Starting https proxy on 0.0.0.0:8080 (Protocol: IPv6)
```

### 卸载（Debian/Ubuntu）

同一个脚本可以删除它安装的全部内容。在 `-s --` 后面传入 `--uninstall`：

```bash
curl -fsSL https://raw.githubusercontent.com/ikenxuan/nyxproxy-oss/main/scripts/quick-setup.sh | sudo bash -s -- --uninstall
```

卸载程序会：

- 停止并删除 `nyxproxy` 服务以及正在运行的 `nyxproxy` 进程
- 删除 `/usr/local/bin/nyxproxy`、`/etc/nyxproxy` 和 `/var/lib/nyxproxy`
- 删除 `/etc/sysctl.d/99-nyxproxy-ipv6.conf`，并恢复安装前的内核参数
- 从 `/etc/ndppd.conf.nyxproxy.bak` 恢复原来的 `/etc/ndppd.conf`，没有备份则删除生成的配置
- 仅当 ndppd 是由本脚本安装时才卸载 ndppd

使用 `--keep-config` 保留 `/etc/nyxproxy/config.yaml`，使用 `--yes` 跳过确认：

```bash
curl -fsSL https://raw.githubusercontent.com/ikenxuan/nyxproxy-oss/main/scripts/quick-setup.sh | sudo bash -s -- --uninstall --keep-config --yes
```

如果检测到旧版在当前目录安装的文件，脚本只会打印旧二进制和工作目录的位置，
不会删除不属于它的文件。

---

### 手动安装

<details>
<summary>点击展开手动安装步骤</summary>

#### 1. 下载 NyxProxy

```bash
wget https://github.com/ikenxuan/nyxproxy-oss/releases/latest/download/nyxproxy-linux-amd64 -O nyxproxy
chmod +x nyxproxy
```

#### 2. 配置 IPv6 轮换

```bash
# 安装 ndppd
sudo apt update && sudo apt install -y ndppd

# 创建 ndppd 配置
sudo tee /etc/ndppd.conf <<EOF
route-ttl 30000

proxy enp1s0 {
    router no
    timeout 500
    ttl 30000

    rule 2a05:f480:1800:25db::/64 {
        auto
    }
}
EOF

# 设置内核参数
sudo sysctl -w net.ipv6.conf.all.proxy_ndp=1
sudo sysctl -w net.ipv6.conf.enp1s0.proxy_ndp=1
sudo sysctl -w net.ipv6.ip_nonlocal_bind=1

# 启动 ndppd
sudo systemctl enable ndppd
sudo systemctl start ndppd
```

#### 3. 创建 config.yaml

```yaml
proxy:
  type: https
  listen_address: "0.0.0.0"
  listen_port: 8080
  username: "admin"
  password: "your-secure-password"

network:
  interface_name: "enp1s0"                        # 你的网卡
  ipv4_enabled: false
  ipv6_enabled: true
  rotate_ipv6: true
  ipv6_subnet: "2a05:f480:1800:25db::/64"        # 你的 /64 子网

  # 轮换设置
  ipv6_pool_size: 200     # 池中 IP 数量
  ipv6_max_usage: 100     # 使用 100 次后替换
  ipv6_max_age: 30        # 30 分钟后替换

monitoring:
  enabled: true
  port: 9090
  allow_remote: false

logging:
  debug_level: 0
```

#### 4. 启动 NyxProxy

```bash
./nyxproxy
```

</details>

---

## ⚙️ 配置

### 基础配置

```yaml
proxy:
  type: https              # 或 "socks5"
  listen_address: "0.0.0.0"
  listen_port: 8080
  username: "admin"
  password: "secure-password"
```

### Web 管理面板

```yaml
web:
  enabled: true
  listen_address: "127.0.0.1"   # 仅在防火墙/反向代理之后使用 0.0.0.0
  port: 18081
  username: "admin"
  password: "change_me_too"
  tls_cert_file: ""
  tls_key_file: ""
  trust_proxy_headers: false
  cookie_secure: false
```

### IPv6 轮换设置

```yaml
network:
  rotate_ipv6: true
  ipv6_subnet: "2a05:f480:1800:25db::/64"

  # 地址池配置
  ipv6_pool_size: 200     # IP 数量（默认：200）
  ipv6_max_usage: 100     # 每个 IP 的最大使用次数（默认：100）
  ipv6_max_age: 30        # 最大存活时间，分钟（默认：30）
```

### 轮换策略

#### 激进（抗指纹识别）

```yaml
ipv6_pool_size: 500      # 大量 IP
ipv6_max_usage: 25       # 快速替换
ipv6_max_age: 10         # 较短存活时间
```

→ 最大隐私，IP 频繁变化

#### 中等（推荐）

```yaml
ipv6_pool_size: 200      # 良好平衡
ipv6_max_usage: 100      # 标准
ipv6_max_age: 30         # 30 分钟
```

→ 性能与隐私的最佳平衡

#### 保守（性能优先）

```yaml
ipv6_pool_size: 100      # 更少 IP，启动更快
ipv6_max_usage: 500      # IP 使用更久
ipv6_max_age: 120        # 2 小时
```

→ 开销更低，性能更好

---

## 🔧 以守护进程运行（Systemd）

### 创建 Systemd 服务

```bash
sudo mkdir -p /etc/nyxproxy
sudo chmod 700 /etc/nyxproxy
sudo cp config.yaml /etc/nyxproxy/config.yaml
sudo chmod 600 /etc/nyxproxy/config.yaml
```

```bash
sudo tee /etc/systemd/system/nyxproxy.service <<EOF
[Unit]
Description=NyxProxy IPv6 Rotating Proxy Server
After=network.target ndppd.service
Requires=ndppd.service

[Service]
Type=simple
User=root
WorkingDirectory=/etc/nyxproxy
ExecStart=/root/nyxproxy -config /etc/nyxproxy/config.yaml
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal

# 安全加固（可选）
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF
```

### 管理服务

```bash
# 开机自启
sudo systemctl enable nyxproxy

# 启动服务
sudo systemctl start nyxproxy

# 查看状态
sudo systemctl status nyxproxy

# 查看日志
sudo journalctl -u nyxproxy -f

# 重启服务
sudo systemctl restart nyxproxy

# 停止服务
sudo systemctl stop nyxproxy
```

---

## 🐳 Docker 部署

仓库提供了一个 Docker 镜像：启动时等待 IPv6 前缀，自动跟随前缀变化，并在收到
`SIGTERM` 时清理池地址。

### 要求

- 一个已路由且 NDP 正常工作的 IPv6 `/64`（见 [环境要求](#-环境要求)）。
- **host 网络**（`network_mode: host`）—— bridge 网络无法向宿主机网卡添加地址。
- `NET_ADMIN` 权限。
- 镜像已包含 `iproute2`（NyxProxy 使用的 `ip` 命令）。

### 使用已发布的镜像（推荐）

仓库会将多架构镜像发布到 GitHub Container Registry：

```text
ghcr.io/ikenxuan/nyxproxy-oss:latest
```

```bash
cp .env.docker.example .env.docker
# 编辑 .env.docker：设置 IFACE、PROXY_PASS 和 WEB_PASS

docker compose pull
docker compose up -d
docker compose logs -f nyxproxy
```

`docker-compose.yml` 默认使用 host 网络、`NET_ADMIN`、只读根文件系统，以及
GHCR 上发布的镜像。

### 从源码构建

```bash
cp .env.docker.example .env.docker
# 编辑 .env.docker：设置 IFACE、PROXY_PASS 和 WEB_PASS

docker compose -f docker/docker-compose.example.yml up -d --build
```

或者手动构建并运行：

```bash
docker build -t nyxproxy-oss .
docker run -d \
  --name nyxproxy \
  --network host \
  --cap-drop ALL --cap-add NET_ADMIN \
  --security-opt no-new-privileges:true \
  --read-only --tmpfs /run \
  -v nyxproxy-data:/data \
  -e IFACE=eth0 \
  -e LISTEN_PORT=18080 \
  -e POOL_SIZE=64 \
  -e PROXY_USER=admin \
  -e PROXY_PASS=change_me \
  -e WEB_ENABLED=true \
  -e WEB_PASS=change_me_too \
  nyxproxy-oss
```

### 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `IFACE` | 默认路由网卡 | 承载 IPv6 `/64` 的网卡 |
| `PREFIX` | 自动检测 | 手动覆盖检测结果，例如 `2001:db8:1:2::/64` |
| `LISTEN_PORT` | `1080` | SOCKS5 监听端口 |
| `POOL_SIZE` | `64` | 轮换地址数量 |
| `MAX_USAGE` | `100` | 地址使用多少次后轮换 |
| `MAX_AGE` | `30` | 地址使用多少分钟后轮换 |
| `PROXY_USER` | `admin` | SOCKS5 用户名 |
| `PROXY_PASS` | *(必填)* | SOCKS5 密码 |
| `DEBUG_LEVEL` | `1` | NyxProxy 调试级别 |
| `DNS_SOURCE_ADDRESS` | 自动 | 可选的 DNS 查询固定源地址 |
| `WEB_ENABLED` | `true` | 是否启用 Web 管理面板 |
| `WEB_LISTEN` | `0.0.0.0` | 面板监听地址（host 网络） |
| `WEB_PORT` | `18081` | 面板端口 |
| `WEB_USER` | `admin` | 面板用户名 |
| `WEB_PASS` | 启用时必填 | 面板密码；仅在首次生成配置时使用 |
| `WEB_TLS_CERT_FILE` | 空 | 容器内 TLS 证书路径（只读挂载） |
| `WEB_TLS_KEY_FILE` | 空 | 容器内 TLS 私钥路径 |

首次启动会用环境变量生成 `/data/config.yaml`。之后该文件是唯一权威来源：
修改 `.env.docker` 不会改变已有配置。请用 Web 面板修改配置，或删除
`nyxproxy-data` 卷后重新生成。`PREFIX` 和 `IFACE` 仍会通过
`NYXPROXY_PREFIX` / `NYXPROXY_INTERFACE` 在每次启动时生效。

entrypoint 会优先从 `proto ra` 路由推导前缀；如果找不到，则回退到 `IFACE` 上
任意 `/64` 路由（适用于 DHCPv6-PD/静态配置）。前缀变化时会重启 NyxProxy。

### 发布

`.github/workflows/docker.yml` 会构建 `linux/amd64` 和 `linux/arm64` 镜像，并在
推送到 `main`、推送 `v*` 标签或手动执行 `workflow_dispatch` 时推送到 GHCR。
`main` 和标签构建都会发布 `latest`；`v1.2.3` 这样的版本标签也会一并发布。

```text
ghcr.io/ikenxuan/nyxproxy-oss:latest
```

英文文档见 [README.md](README.md)。

---

## 🖥️ Web 管理面板

管理面板与代理运行在同一个二进制和容器中，提供：

- 实时仪表盘（代理状态、地址池、流量计数、上次应用结果）
- 完整的配置编辑器和服务端校验
- 自动应用；组件启动失败时自动回滚
- 基于 SSE 的实时日志

### Docker

在 `.env.docker` 中设置 `WEB_PASS`，启动后打开
`http://你的服务器:18081`。host 网络下端口直接暴露在宿主机；请用防火墙或
反向代理限制访问。

内置 TLS 时，把证书只读挂载进容器，并设置：

```yaml
web:
  tls_cert_file: /certs/tls.crt
  tls_key_file: /certs/tls.key
```

```bash
docker run ... -v "$PWD/certs:/certs:ro" ...
```

### 裸机

在 `config.yaml` 中启用：

```yaml
web:
  enabled: true
  listen_address: "127.0.0.1"
  port: 18081
  username: "admin"
  password: "change_me_too"
  tls_cert_file: ""
  tls_key_file: ""
  trust_proxy_headers: false
  cookie_secure: false
```

裸机默认只监听回环地址。使用 SSH 隧道
（`ssh -L 18081:127.0.0.1:18081 user@server`）或反向代理。若使用 nginx /
Caddy，请保留 `Host` 头，对 `/api/logs/stream` 关闭代理缓冲，并且只在面板
不直接暴露时设置 `trust_proxy_headers: true`。

保存配置会重启代理（监控配置变化时也会重启监控），现有连接最多等待 5 秒
完成 drain。非法配置会在停止任何组件之前被拒绝；新配置启动失败时会恢复旧
配置，磁盘文件保持不变。

---

## 💻 使用示例

### cURL

```bash
# 单个请求
curl --proxy http://admin:password@your-server:8080 https://api6.ipify.org

# 10 个请求 —— 每个使用不同的 IPv6
for i in {1..10}; do
  echo "请求 $i:"
  curl --proxy http://admin:password@your-server:8080 https://api6.ipify.org
done
```

### Python

```python
import requests

proxies = {
    'http': 'http://admin:password@your-server:8080',
    'https': 'http://admin:password@your-server:8080'
}

# 每个请求使用不同的 IPv6
for i in range(10):
    response = requests.get('https://api6.ipify.org', proxies=proxies)
    print(f"Request {i+1}: {response.text}")
```

### Node.js

```javascript
const axios = require('axios');

const proxy = {
  host: 'your-server',
  port: 8080,
  auth: {
    username: 'admin',
    password: 'password'
  }
};

// 每个请求使用不同的 IPv6
for (let i = 0; i < 10; i++) {
  axios.get('https://api6.ipify.org', { proxy })
    .then(res => console.log(`Request ${i+1}: ${res.data}`));
}
```

---

## 📚 IPv6 轮换详解

### 为什么池里是 200 个 IP？

**启动时间与多样性的权衡：**
- IP 越多 = 启动越慢（但只需一次！）
- IP 越多 = 多样性越高，重复越少
- 对大多数场景，200 个 IP 是最佳平衡点

**计算：**
```
200 个 IP × 100 次使用 = 首次轮换前可处理 20,000 个请求
20,000 个请求 ÷ 100 请求/分钟 = 200 分钟的独立 IP 使用时间
```

### 为什么使用 100 次后替换？

**限流防护：**
- 大多数 API 按 IP 限流
- 100 次请求通常低于大多数阈值
- 使用 100 次后，该 IP 已被"消耗" → 替换它

### 为什么 30 分钟后替换？

**基于时间的追踪：**
- 某些服务会按时间追踪 IP
- 新 IP 被标记的概率更低
- 30 分钟是一个不错的平衡

### 后台轮换

每 5 分钟，NyxProxy 会检查所有 IP：
```
对池中的每个 IP：
  如果（使用次数 >= 100）或（存活时间 > 30 分钟）：
    1. 从网卡删除旧 IP
    2. 生成新的随机 IP
    3. 将新 IP 添加到网卡
    4. 更新地址池
```

**示例日志：**
```
14:35:00 - ✓ Rotated 12 IPs (used 100+ times)
15:05:00 - ✓ Rotated 8 IPs (age 30m+)
15:35:00 - ✓ Rotated 5 IPs (used 100+ times)
```

---

## 📋 环境要求

### 服务器要求
- **操作系统**：Linux（Debian、Ubuntu、CentOS 等）
- **架构**：amd64 或 arm64
- **内存**：>= 512 MB
- **磁盘**：>= 100 MB
- **Docker**：Docker Engine 20.10+ 与 Compose v2（容器部署）
- **Go**：从源码构建时需要 1.27+
- **Node**：构建前端时需要 Node 24 LTS 和 pnpm 12.14.0

### 网络要求
- **IPv6 /64 子网**确实路由到你的服务器
- 服务商将整个 `/64` 路由给你，并允许你在网卡上添加地址
- `/124` 或更小的委派对于可用的轮换池来说太小
- Docker 部署：host 网络（`network_mode: host`）和 `NET_ADMIN`

### 检查环境

```bash
# 检查是否有 /64 子网
ip -6 addr show | grep "scope global"

# 应显示类似内容：
# inet6 2a05:f480:1800:25db::1/64 scope global

# 测试 IPv6 连通性
ping6 google.com

# 检查是否安装了 ndppd
which ndppd
```

---

## 🔧 故障排查

### 问题："Cannot find device ''"

**原因：** 未配置网络接口

**解决方案：**
```bash
# 查找你的网卡
ip link show

# 在 config.yaml 中设置
network:
  interface_name: "enp1s0"  # 你的网卡名称
```

### 问题："ndppd service is not running"

**原因：** ndppd 未安装或未配置

**解决方案：**
```bash
# 安装 ndppd
sudo apt install -y ndppd

# 或重新运行安装脚本
curl -fsSL https://raw.githubusercontent.com/ikenxuan/nyxproxy-oss/main/scripts/quick-setup.sh | sudo bash
```

### 问题：请求超时

**原因：** IP 未正确路由

**解决方案：**
```bash
# 检查子网是否路由到你
curl --interface 2a05:f480:1800:25db::9999 -6 https://api6.ipify.org

# 如果成功，说明子网路由正确
# 如果失败，请联系你的服务商
```

### 问题：启动缓慢

**原因：** IP 池过大

**解决方案：**
```yaml
# 减小池大小以加快启动
network:
  ipv6_pool_size: 100  # 而不是 200
```

### 查看日志

```bash
# NyxProxy 日志（如果以服务方式运行）
sudo journalctl -u nyxproxy -f

# ndppd 日志
sudo journalctl -u ndppd -f

# 查看系统日志
dmesg | grep -i ipv6
```

---

## 🌟 高级用法

### 多代理实例

默认读取 `./config.yaml`；使用 `-config` 或 `NYXPROXY_CONFIG` 可以指定其他文件。

使用不同目录（每个目录有自己的 `config.yaml`）即可在不同端口运行多个代理：

```bash
mkdir -p proxy-8080 proxy-8081
cp nyxproxy proxy-8080/nyxproxy
cp nyxproxy proxy-8081/nyxproxy

# 创建并编辑各自的配置
cp config.example.yaml proxy-8080/config.yaml
cp config.example.yaml proxy-8081/config.yaml

(cd proxy-8080 && sudo ./nyxproxy -config /root/proxy-8080/config.yaml) &
(cd proxy-8081 && sudo ./nyxproxy -config /root/proxy-8081/config.yaml) &
```

显式指定 `-config` 后，配置路径与工作目录无关；也可以通过
`NYXPROXY_CONFIG` 环境变量覆盖。

### 监控

```bash
# 查看代理统计
curl http://localhost:9090/stats

# 输出：
# {
#   "uptime": "5h23m15s",
#   "active_connections": 0,
#   "total_connections": 0,
#   "total_requests": 0,
#   "bytes_sent": 0,
#   "bytes_received": 0,
#   "interface": "eth0",
#   "ip_pool_size": 200,
#   "ips_rotated": 38
# }
```

---

## 📝 许可证

MIT 许可证 —— 详见 [LICENSE](LICENSE) 文件。

---

## 🤝 贡献

欢迎贡献！请随时提交 Pull Request。

---

## 🙏 致谢

- 使用 Go 构建
- 使用 ndppd 实现 NDP 代理功能
- 灵感来自对更好的代理轮换的需求

---

## 📞 支持

- **问题反馈**：[GitHub Issues](https://github.com/ikenxuan/nyxproxy-oss/issues)
- **文档**：见 `docs/` 目录
- **德语文档**：见 `SETUP_DE.md`

---

**用 ❤️ 为注重隐私和网页抓取社区打造**
