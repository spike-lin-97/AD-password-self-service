# 密码自助平台

面向微软 AD 的密码自助服务：员工用 **飞书扫码 / 一键授权 / AD 账密** 登录后，可自行修改域密码、解锁被锁定的账户。

本目录（`feishu-login/`）是当前在用的实现。页面和静态资源通过 `go:embed` 打进二进制，部署时只需 **可执行文件 + YAML 配置**。

## 功能

- 飞书扫码登录、飞书客户端一键授权、AD 用户名密码登录
- 按飞书姓名匹配 AD `displayName`，登录后展示 `sAMAccountName`、上次改密时间、密码过期时间
- 自助改密（unicodePwd）与账户解锁（`lockoutTime=0`）
- 管理后台：改飞书 / AD 参数，测试飞书与 LDAPS 连通性

## 环境要求

- Ubuntu 20.04 / 22.04 / 24.04（其它 systemd Linux 同样适用）
- Go 1.22+（仅编译时需要；运行不依赖 Go）
- 能访问 AD 域控 **LDAPS 636**
- 飞书开放平台应用（网页应用），已配置重定向 URL

开发机可直接：

```bash
cd feishu-login
make build          # 生成 bin/password-self-service
make run            # go run，读取 etc/password-self-service.yaml
```

浏览器打开 `http://<本机IP>:8080`。健康检查：`curl http://127.0.0.1:8080/health`。

## 配置

编辑 `etc/password-self-service.yaml`（安装成服务后是 `/opt/password-self-service/etc/password-self-service.yaml`）。YAML **只用空格缩进，不要用 Tab**。

主要字段：

| 段 | 说明 |
| --- | --- |
| `server.addr` | 监听地址，默认 `:8080`。生产建议 `server.mode: release` |
| `ldap.host/port/tls` | 域控地址，生产用 `port: 636` + `tls: true` |
| `ldap.username/password` | 有改密、解锁权限的绑定账号（DN） |
| `ldap.base` | 搜索起点，例如 `OU=FeishuSync,DC=example,DC=com` |
| `ldap.skip-verify` | `true` 时跳过 LDAPS 证书校验（自签/过期证书可用）。配好企业 CA 后改为 `false` 并填写 `ca-file` / `tls-server-name` |
| `feishu.app-id` / `app-secret` | 飞书应用凭证 |
| `feishu.redirect-url` | 必须与飞书开放平台「重定向 URL」**逐字一致**，例如 `http://localhost:8080/api/v1/auth/feishu/callback` |
| `session.hash-key` / `block-key` | Cookie 加解密密钥。生产必须换成随机值：`hash-key` 建议 32 字节，`block-key` 必须是 16/24/32 字节 |
| `admin.username` / `password` | `/admin.html` 后台账号，部署后请立刻改掉默认密码 |

飞书开放平台还需把该回调地址加到应用的 **安全设置 → 重定向 URL**。匹配规则：飞书用户姓名 = AD `displayName`，且在 `feishu.ad-user-base`（默认等于 `ldap.base`）下唯一。

登录页源码在仓库根目录的 `login-new/`。改完界面后先构建，再编译 Go，因为页面会嵌入二进制：

```bash
cd ../login-new
npm install
npm run build   # 写入 feishu-login/web/index.html 与 web/assets/login/
cd ../feishu-login
make build
```

---

## 封装成 Ubuntu systemd 服务

目标：本机开机后自动拉起，用 `systemctl` 管理，日志进 journald。

### 1. 安装

在 **这台要跑服务的 Ubuntu** 上进入本目录，用 root 执行：

```bash
cd /path/to/password-self-service/feishu-login
chmod +x deploy/install.sh
sudo ./deploy/install.sh
```

脚本会优先用 `/usr/local/go`，并用仓库里的 `vendor/` **离线编译**（不访问 `proxy.golang.org`）。若仍失败，可先本机编好再安装：

```bash
GOPROXY=https://goproxy.cn,direct make build
sudo ./deploy/install.sh
```

脚本会：

1. 编译生成 `bin/password-self-service`（若已安装 Go）
2. 创建系统用户 `pss`（无登录 shell）
3. 安装到 `/opt/password-self-service/`
   - 程序：`/opt/password-self-service/password-self-service`
   - 配置：`/opt/password-self-service/etc/password-self-service.yaml`（**已存在则不覆盖**）
4. 写入 `/etc/systemd/system/password-self-service.service`
5. `systemctl enable --now`：开机自启并立刻启动

若 8080 已被旧的 `go run` / 手工进程占用，先停掉再装：

```bash
# 看谁占用了 8080
sudo ss -lntp | grep 8080
# 停掉后重新执行 install.sh，或：
sudo systemctl restart password-self-service
```

### 2. 启用 / 启停（日常）

```bash
sudo systemctl enable password-self-service    # 开机自启（install.sh 已做过）
sudo systemctl start password-self-service     # 启动
sudo systemctl stop password-self-service      # 停止
sudo systemctl restart password-self-service   # 改配置或换二进制后重启
sudo systemctl status password-self-service    # 看是否 active (running)
```

### 3. 看日志

```bash
sudo journalctl -u password-self-service -f
sudo journalctl -u password-self-service -n 100 --no-pager
```

启动成功日志里会有 `Starting server on :8080`。然后：

```bash
curl http://127.0.0.1:8080/health
# 期望：{"status":"ok"}
```

防火墙若开了 ufw，放行 8080：

```bash
sudo ufw allow 8080/tcp
```

### 4. 改配置 / 升级程序

```bash
sudo nano /opt/password-self-service/etc/password-self-service.yaml
sudo systemctl restart password-self-service
```

升级（源码有更新时，在 `feishu-login` 目录再跑一次安装脚本即可；**不会覆盖已有 YAML**）：

```bash
cd /path/to/password-self-service/feishu-login
sudo ./deploy/install.sh
```

也可手动：

```bash
make build
sudo install -o pss -g pss -m 0755 bin/password-self-service /opt/password-self-service/password-self-service
sudo systemctl restart password-self-service
```

管理后台改过的配置会立刻写回 YAML。服务进程对 `etc`、`var` 目录可写（unit 里 `ReadWritePaths`），无需为改配置而手工重启；改 `server.addr` 等监听项仍需 `systemctl restart`。

操作日志写在 `var/audit.jsonl`（安装后为 `/opt/password-self-service/var/audit.jsonl`），后台「操作日志」页可查 IP、MAC、操作的 AD、变更时间、下次到期时间。MAC 只能在用户与服务器同一二层网段时从 ARP 表读取。

### 5. 卸载

```bash
sudo systemctl disable --now password-self-service
sudo rm -f /etc/systemd/system/password-self-service.service
sudo systemctl daemon-reload
sudo rm -rf /opt/password-self-service
sudo userdel pss    # 可选
```

---

## 页面入口

| 地址 | 说明 |
| --- | --- |
| `/` | 登录页 |
| `/console.html` | 自助中心（改密 / 解锁） |
| `/admin.html` | 管理后台（系统配置 / 操作日志） |
| `/api/v1/auth/feishu/callback` | 飞书 OAuth 回调（不要当页面打开） |

## 手动编译参数

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/password-self-service main.go
./bin/password-self-service --config etc/password-self-service.yaml
```

`--config` 建议用绝对路径，避免从其它工作目录启动时找不到 YAML。systemd unit 已使用绝对路径。
