# 密码自助平台

面向微软 Active Directory 的密码自助服务。员工可以用飞书扫码、飞书一键授权或 AD 账密登录，然后自行修改域密码、解锁被锁定的账户。

线上（production）：<http://192.168.9.7:8090>，部署在发布机 `192.168.9.7`，由 GitLab CI/CD 自动发布。

仓库里有两套程序：

- `feishu-login/`：当前使用的服务。页面打进二进制，部署时只需要可执行文件和 YAML。
- 仓库根目录：更早的短信 / 邮件验证码方案，保留作参考。

登录页的源码在 `login-new/`（带动画角色的 React 页面）。构建后写入 `feishu-login/web/`，由 Go 服务嵌入。

## 本地运行

```bash
cd feishu-login
# 编辑 etc/password-self-service.yaml，填入自己的域控和飞书应用
make run
```

浏览器打开 `http://localhost:8090`。健康检查：`curl http://127.0.0.1:8090/health`。

改登录页之后需要重新构建前端，再编译 Go：

```bash
cd login-new
npm install
npm run build
cd ../feishu-login
make build
```

配置说明、systemd 安装和页面入口见 [feishu-login/README.md](feishu-login/README.md)。

## 配置注意

`etc/password-self-service.yaml` 里的域名、账号和密钥都是占位符。部署前请换成自己的值，尤其是：

- LDAP 绑定账号
- 飞书 App ID / App Secret 和重定向 URL
- `session.hash-key` / `session.block-key`（生产必须用随机值）
- 管理后台密码：`admin.password` 默认是 `change-me`，**部署后必须立刻修改**，否则任何人都能登录 `/admin.html`

不要把真实口令提交到仓库。

## CI/CD 自动部署

推送到 `main` 分支会自动跑流水线（`build` → `deploy`），其他分支不触发。

- **Runner**：shell 执行器，跑在 `192.168.9.10` 的 `gitlab-runner` 容器里，作业用户 `gitlab-runner` 对 `/usr/local` 无写权限。
- **build**：CI 脚本把 Go 1.22.12 下载解压到 `$CI_PROJECT_DIR/.go` 并加入 `PATH`（不写 `/usr/local`），用仓库里的 `vendor/` 离线编译，产物 `feishu-login/bin/password-self-service`。
- **deploy**：依赖 CI/CD 变量 `DEPLOY_PASSWORD`（发布机 `skyland` 的登录密码，同时用于 sudo），Runner 容器需预装 `sshpass`。脚本把二进制、systemd unit、YAML 示例、`ci-install.sh` scp 到发布机，再 sudo 执行安装脚本。

发布机 `192.168.9.7` 上：

- 程序与配置装在 `/opt/password-self-service/`，systemd 服务名 `password-self-service`，监听 **8090**。
- 安装脚本 `ci-install.sh` **不会覆盖已存在的 YAML**：首次部署写入示例配置，之后改配置要直接改发布机上的 `/opt/password-self-service/etc/password-self-service.yaml` 再 `systemctl restart password-self-service`，只改仓库重新部署不生效。
- 该配置文件属主 `pss`、权限 0640，查看需 `sudo`。

常用运维命令：

```bash
sudo systemctl status password-self-service
sudo journalctl -u password-self-service -n 80 --no-pager
curl http://127.0.0.1:8090/health   # 期望 {"status":"ok"}
```

流水线历史见 GitLab 项目的「构建 → 流水线」。

## 许可

GPL-3.0。根目录的短信 / 邮件方案来自 [kubeop/password-self-service](https://github.com/kubeop/password-self-service)。
