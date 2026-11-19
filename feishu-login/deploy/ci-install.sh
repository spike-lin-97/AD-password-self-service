#!/bin/bash
# 在目标机上以 root 执行。二进制和示例配置需事先放到 /tmp。
# 已有 YAML 不会被覆盖。
set -euo pipefail

PREFIX=/opt/password-self-service
NAME=password-self-service

if ! id -u pss >/dev/null 2>&1; then
  useradd --system --home "$PREFIX" --shell /usr/sbin/nologin pss
fi

install -d -o pss -g pss -m 0755 "$PREFIX" "$PREFIX/etc" "$PREFIX/var"

if systemctl is-active --quiet "$NAME"; then
  systemctl stop "$NAME"
fi

install -o pss -g pss -m 0755 "/tmp/${NAME}.new" "$PREFIX/$NAME"
install -m 0644 "/tmp/${NAME}.service" "/etc/systemd/system/${NAME}.service"

if [[ ! -f "$PREFIX/etc/${NAME}.yaml" ]]; then
  install -o pss -g pss -m 0640 "/tmp/${NAME}.yaml.example" "$PREFIX/etc/${NAME}.yaml"
  echo "已写入示例配置：$PREFIX/etc/${NAME}.yaml"
  echo "请把域控和飞书参数改成真实值后再使用登录功能。"
else
  echo "保留已有配置 $PREFIX/etc/${NAME}.yaml"
fi

rm -f "/tmp/${NAME}.new" "/tmp/${NAME}.service" "/tmp/${NAME}.yaml.example"

systemctl daemon-reload
systemctl enable "$NAME"
systemctl restart "$NAME"
sleep 1
curl -fsS http://127.0.0.1:8080/health
echo
systemctl --no-pager --full status "$NAME" || true
