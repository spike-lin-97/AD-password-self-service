# 当前服务部署说明

`feishu-login/` 是当前 AD密码自助服务，独立于仓库根目录的旧版程序。功能、目录结构、配置字段和前端构建步骤见 [项目 README](../README.md)。

## 本地运行

在本目录执行：

```bash
cp etc/password-self-service.yaml etc/password-self-service.local.yaml
# 填写 AD、飞书、随机 Session 密钥和新的后台密码
go run -mod=vendor main.go --config etc/password-self-service.local.yaml
```

默认访问 `http://localhost:8090/`，后台为 `/admin.html`。示例账号 `admin` / `change-me` 仅用于初始化，请在开放访问前修改。

配置必填项为空会导致启动失败；占位值通过非空校验也不代表可以连接 AD 或飞书。当前没有空配置初始化向导。

## 编译与安装

Linux systemd 环境，在本目录执行：

```bash
make build
sudo bash deploy/install.sh
```

安装脚本会尝试使用本机 Go 和 vendor 编译；没有 Go 时可以使用事先准备好的 `bin/password-self-service`。

安装位置：

- 程序：`/opt/password-self-service/password-self-service`
- 配置：`/opt/password-self-service/etc/password-self-service.yaml`
- 审计日志：`/opt/password-self-service/var/audit.jsonl`
- systemd 单元：`/etc/systemd/system/password-self-service.service`
- 运行用户：`pss`

**安装脚本仅在目标配置不存在时复制 `etc/password-self-service.yaml`，不会自动使用本地的 `.local.yaml`。** 首次安装会启动服务；对外开放前，请编辑安装目录内的配置，替换所有占位值、后台口令及 Session 密钥，然后重启。已有配置不会被后续安装覆盖。

```bash
sudo nano /opt/password-self-service/etc/password-self-service.yaml
sudo systemctl restart password-self-service
sudo systemctl status password-self-service
curl http://127.0.0.1:8090/health
```

以上健康检查使用示例端口 8090；如修改 `server.addr`，需同步调整命令。安装脚本末尾仍有历史 8080 提示，以实际配置为准。

## 配置与证书

AD 绑定账号需获得目标用户的密码重置和账户解锁权限。通过 LDAPS 连接域控，正式部署配置可信 CA，关闭 `ldap.skip-verify`。证书文件必须允许运行用户读取；systemd 单元启用了 `ProtectHome=true`，不要将运行时证书放在用户家目录。

飞书姓名必须在配置搜索范围内唯一匹配 AD `displayName`。忘记密码、账户锁定的用户应走飞书登录。

后台保存飞书与 AD 配置后会写回正在使用的 YAML，后续请求读取新参数。监听地址及初始化时创建的 Session 管理器相关参数变更后，需要重启服务。替换 Session 密钥后旧登录态失效。

当前程序不使用 Redis 保存会话，不需要单独部署 Redis。页面和静态资源已内嵌，前端变更后必须重新构建二进制。

## 日志与维护

```bash
sudo journalctl -u password-self-service -f
sudo journalctl -u password-self-service -n 100 --no-pager
sudo systemctl restart password-self-service
sudo systemctl stop password-self-service
sudo systemctl start password-self-service
```

审计日志包含用户信息，可从后台查看。MAC 信息依赖服务器能否从 ARP 表读取，跨网段通常不可得。备份时同时保存配置与审计日志，并限制访问权限。

升级时重新编译并执行 `sudo bash deploy/install.sh`；安装脚本保留目标配置。升级前建议备份 `/opt/password-self-service/etc/` 和 `var/`。

## CI 与旧版文件

GitHub 工作流构建并检查当前服务，不发布镜像。GitLab 部署为手动触发，需要自行配置部署变量。根目录 Dockerfile 和 `manifest/` 对应旧版服务，不能直接用作当前飞书版本的部署入口。详见 [项目 README](../README.md#ci-与容器文件)。
