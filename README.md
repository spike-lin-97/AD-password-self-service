# AD密码自助

面向 Microsoft Active Directory 的密码自助服务，支持飞书扫码、飞书一键授权和 AD 账号密码登录。员工验证身份后，可以重置自己的域密码、解锁账户，并查看密码过期时间。

## 功能

- **多种登录方式**：AD 用户名密码、飞书扫码、飞书一键授权。
- **密码自助**：重置当前用户的域密码、解锁被锁定的账户。
- **账户信息**：展示 AD 登录名、上次改密时间和密码过期时间。
- **管理后台**：维护飞书应用和 AD 连接配置，测试连通性，查看操作日志。
- **独立部署**：Go 服务内嵌页面资源，运行时无需 Node.js。

忘记密码或账户锁定时，需使用飞书验证身份；AD 账密登录仍依赖有效的账号密码。

## 目录说明

```text
.
├── feishu-login/             当前服务，独立 Go module
│   ├── api/                  登录、自助操作和管理 API
│   ├── internal/             配置、AD、会话和审计日志
│   ├── pkg/                  飞书客户端和公共响应
│   ├── etc/                  当前服务的示例配置
│   ├── web/                  内嵌页面和前端构建产物
│   └── deploy/               Linux systemd 安装脚本及服务文件
├── login-new/                React 登录页源码，构建到 feishu-login/web/
├── api/、internal/、pkg/      旧版短信 / 邮件验证码服务
├── etc/、views/、docs/        旧版配置、页面和 API 文档
├── manifest/                 旧版 Docker Compose / Kubernetes 部署文件
├── Dockerfile、Makefile      根目录旧版服务的构建入口
└── LICENSE                   GPL-3.0 许可证
```

**开发和部署当前版本，请进入 `feishu-login/`。** 根目录的 Go 程序、Dockerfile、Swagger 文档及容器部署文件不等同于当前飞书版本。

## 运行条件

- Go：`feishu-login/go.mod` 声明最低 1.21；现有 GitLab 构建脚本使用 1.22.12。
- AD 域控：可访问的 LDAPS 服务，示例端口为 636；绑定账号需有目标用户的密码重置及解锁权限。
- 飞书应用：可用的应用凭证、授权回调地址和用户信息访问权限。
- 修改登录页时需要 Node.js、npm；直接构建已提交的页面资源无需 Node.js。
- 当前服务使用加密 Cookie 保存会话，不依赖 Redis；示例中的 Redis 字段为保留配置。

## 快速开始

以下命令从仓库根目录执行：

```bash
cd feishu-login
cp etc/password-self-service.yaml etc/password-self-service.local.yaml
# 编辑 etc/password-self-service.local.yaml，填写自己的配置

go run -mod=vendor main.go --config etc/password-self-service.local.yaml
```

示例默认监听 `:8090`。启动后访问：

- 登录页：`http://localhost:8090/`
- 自助中心：`http://localhost:8090/console.html`
- 管理后台：`http://localhost:8090/admin.html`
- 健康检查：`http://localhost:8090/health`

健康检查返回 `{"status":"ok"}` 仅表示 HTTP 服务正常，不代表飞书和 AD 已连通；可在后台分别测试。

### 首次配置

当前示例配置包含占位值，**没有真实的飞书应用和 AD 域控凭证，也不是空配置**。当前程序会校验必填项，不支持把所有字段清空后启动配置向导。

编辑 `feishu-login/etc/password-self-service.local.yaml`：

- `ldap.host`、`port`、`tls`：自己的 AD 域控地址及 LDAPS 参数。
- `ldap.username`、`password`、`base`：绑定账号、口令和用户搜索范围。
- `ldap.skip-verify`：示例为 `true`，会跳过证书校验；正式部署应配置可信 CA，并设为 `false`，按需填写 `ca-file` / `tls-server-name`。
- `feishu.app-id`、`app-secret`、`redirect-url`：自己的飞书应用凭证与完整回调 URL。
- `feishu.ad-user-base`：飞书匹配用户的搜索范围，留空时复用 `ldap.base`。
- `session.hash-key`、`block-key`：替换为随机密钥。`block-key` 按实际字符串字节数必须为 16、24 或 32。
- `admin.username`、`password`：后台示例账号为 `admin` / `change-me`，对外开放前必须修改。

可以用 Python 生成两行密钥，分别填入 `hash-key` 和 `block-key`：

```bash
python3 -c "import secrets; print(secrets.token_hex(32)); print(secrets.token_hex(16))"
```

飞书回调路径是 `/api/v1/auth/feishu/callback`。配置中的完整 URL 必须与飞书应用的重定向 URL 一致。当前身份匹配规则是 **飞书姓名与 AD `displayName` 相同，且搜索范围内唯一**；未匹配或存在重名时会拒绝登录。

本地配置 `*.local.yaml` 已被 Git 忽略，程序不会自动合并该文件，需要通过 `--config` 明确指定。后台保存配置会写回该路径，因此进程需要对配置所在目录具有写权限。

## 构建

仅构建 Go 服务（使用仓库内的 vendor）：

```bash
cd feishu-login
go build -mod=vendor -o bin/password-self-service main.go
./bin/password-self-service --config etc/password-self-service.local.yaml
```

Windows 下可输出 `bin/password-self-service.exe` 并运行该文件。Linux 环境也可以使用 `make build`。

修改 React 登录页后，需要先更新内嵌资源，再编译 Go：

```bash
cd login-new
npm ci
npm run build
cd ../feishu-login
make build
```

Vite 将资源写入 `feishu-login/web/`。前端源码和构建产物需要一起提交；只更新源码不会改变 Go 二进制中的页面。

## Linux 服务部署

systemd 安装、日志位置、升级及故障排查见 [部署说明](feishu-login/README.md)。服务默认安装到 `/opt/password-self-service/`，安装脚本保留已有配置。

正式使用前应替换所有示例口令和会话密钥，通过 HTTPS 提供网页访问，并限制后台的可访问范围。飞书姓名匹配依赖组织内姓名唯一性，部署前应核实目标用户数据。

## CI 与容器文件

- GitHub 工作流会构建 React 登录页，并对 `feishu-login/` 执行 Go 测试、静态检查和编译，提供 Linux 二进制下载，不登录或推送任何镜像仓库。
- `.gitlab-ci.yml` 保留可选的 GitLab 构建与手动部署。部署需配置 `DEPLOY_HOST`、`DEPLOY_USER`、`DEPLOY_PASSWORD`，可选 `DEPLOY_URL`；仓库中不预设内部服务器。
- 根目录 Dockerfile 和 `manifest/` 对应旧版服务。当前飞书版本推荐编译二进制或安装为 systemd 服务，现有容器文件需要适配后使用。

## 贡献

提交问题时请附运行系统、构建方式、复现步骤和脱敏日志。请勿提交真实域账号口令、飞书 App Secret、会话密钥、私钥或用户审计记录。

## 许可证与来源

仓库根目录使用 [GPL-3.0](LICENSE)。旧版短信 / 邮件方案源自 [kubeop/password-self-service](https://github.com/kubeop/password-self-service)。登录页目录保留了其 [MIT 许可证](login-new/LICENSE)，动画页面参考来源见 [前端说明](login-new/README.md)。再分发时请保留相应许可证和来源说明。
