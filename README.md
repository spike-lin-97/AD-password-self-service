# 密码自助平台

面向微软 Active Directory 的密码自助服务。员工可以用飞书扫码、飞书一键授权或 AD 账密登录，然后自行修改域密码、解锁被锁定的账户。

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

浏览器打开 `http://localhost:8080`。健康检查：`curl http://127.0.0.1:8080/health`。

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
- 管理后台密码

不要把真实口令提交到仓库。

## 许可

GPL-3.0。根目录的短信 / 邮件方案来自 [kubeop/password-self-service](https://github.com/kubeop/password-self-service)。
