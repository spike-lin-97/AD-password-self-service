#!/usr/bin/env bash
# 将本程序安装为 Ubuntu systemd 服务（开机自启）。
# 用法：在 feishu-login 目录执行  sudo ./deploy/install.sh
set -euo pipefail

PREFIX="${PREFIX:-/opt/password-self-service}"
SERVICE_NAME="password-self-service"
SYS_USER="pss"
SRC="$(cd "$(dirname "$0")/.." && pwd)"
UNIT_SRC="$SRC/deploy/${SERVICE_NAME}.service"
BIN_SRC="$SRC/bin/${SERVICE_NAME}"
CFG_NAME="password-self-service.yaml"

if [[ "${EUID}" -ne 0 ]]; then
    echo "请用 root 运行：sudo $0"
    exit 1
fi

if [[ ! -f "$UNIT_SRC" ]]; then
    echo "找不到 unit 文件：$UNIT_SRC"
    exit 1
fi

# sudo 默认 PATH 往往只有 /usr/bin/go（apt 旧版），读不了较新的 go.mod。
# 优先使用本机官方安装的 /usr/local/go，以及执行 sudo 的那个用户的 go。
find_go() {
    local candidates=()
    local from_user=""
    local home=""
    [[ -n "${GO_BIN:-}" ]] && candidates+=("${GO_BIN}")
    candidates+=("/usr/local/go/bin/go")
    if [[ -n "${SUDO_USER:-}" && "${SUDO_USER}" != "root" ]]; then
        home="$(getent passwd "${SUDO_USER}" | cut -d: -f6 || true)"
        [[ -n "$home" ]] && candidates+=("${home}/go/bin/go" "${home}/.local/go/bin/go")
        from_user="$(su -s /bin/bash - "${SUDO_USER}" -c 'command -v go' 2>/dev/null || true)"
        [[ -n "$from_user" ]] && candidates+=("$from_user")
    fi
    if command -v go >/dev/null 2>&1; then
        candidates+=("$(command -v go)")
    fi
    local g
    for g in "${candidates[@]}"; do
        if [[ -n "$g" && -x "$g" ]]; then
            printf '%s\n' "$g"
            return 0
        fi
    done
    return 1
}

GO_CMD="$(find_go || true)"
if [[ -z "$GO_CMD" && ! -x "$BIN_SRC" ]]; then
    echo "找不到可用的 go。请安装 Go，或先在本目录执行 make build。"
    echo "若 Go 装在 /usr/local/go，也可：sudo GO_BIN=/usr/local/go/bin/go ./deploy/install.sh"
    exit 1
fi

# sudo 默认 GOPROXY 是 proxy.golang.org，国内会超时。
# 有 vendor 时完全离线编译；否则走 goproxy.cn，并复用执行 sudo 的用户的模块缓存。
build_binary() {
    mkdir -p "$SRC/bin"
    local -a envcmd=(env CGO_ENABLED=0 GOTOOLCHAIN=local)
    local -a go_args=(build -ldflags="-s -w" -o "bin/${SERVICE_NAME}" main.go)

    if [[ -f "$SRC/vendor/modules.txt" ]]; then
        echo ">>> 使用 vendor 离线编译（不访问网络）"
        envcmd+=(GOPROXY=off)
        go_args=(build -mod=vendor -ldflags="-s -w" -o "bin/${SERVICE_NAME}" main.go)
    else
        echo ">>> 使用 GOPROXY=https://goproxy.cn,direct 编译"
        envcmd+=(
            GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
            GOSUMDB="${GOSUMDB:-sum.golang.google.cn}"
        )
        if [[ -n "${SUDO_USER:-}" && "${SUDO_USER}" != "root" ]]; then
            local user_home
            user_home="$(getent passwd "${SUDO_USER}" | cut -d: -f6 || true)"
            if [[ -n "$user_home" ]]; then
                envcmd+=(
                    HOME="$user_home"
                    GOPATH="${user_home}/go"
                    GOCACHE="${user_home}/.cache/go-build"
                    GOMODCACHE="${user_home}/go/pkg/mod"
                )
            fi
        fi
    fi

    (cd "$SRC" && "${envcmd[@]}" "$GO_CMD" "${go_args[@]}")
}

if [[ -n "$GO_CMD" ]]; then
    echo ">>> 使用 $($GO_CMD version) 编译 ${SERVICE_NAME}"
    if ! build_binary; then
        if [[ -x "$BIN_SRC" ]]; then
            echo ">>> 编译失败，改用已有二进制 $BIN_SRC"
        else
            echo "编译失败。可先不带 sudo 执行：GOPROXY=https://goproxy.cn,direct make build"
            echo "成功后再：sudo ./deploy/install.sh"
            exit 1
        fi
    else
        BIN_SRC="$SRC/bin/${SERVICE_NAME}"
    fi
else
    echo ">>> 未找到 go，改用已有二进制 $BIN_SRC"
fi

if [[ ! -x "$BIN_SRC" ]]; then
    echo "找不到可执行文件：$BIN_SRC"
    exit 1
fi

if ! id -u "$SYS_USER" >/dev/null 2>&1; then
    echo ">>> 创建系统用户 ${SYS_USER}"
    useradd --system --home "$PREFIX" --shell /usr/sbin/nologin "$SYS_USER"
fi

echo ">>> 安装到 ${PREFIX}"
install -d -o "$SYS_USER" -g "$SYS_USER" -m 0755 "$PREFIX" "$PREFIX/etc" "$PREFIX/var"
install -o "$SYS_USER" -g "$SYS_USER" -m 0755 "$BIN_SRC" "$PREFIX/${SERVICE_NAME}"

if [[ -f "$PREFIX/etc/${CFG_NAME}" ]]; then
    echo ">>> 已保留现有配置 $PREFIX/etc/${CFG_NAME}"
else
    install -o "$SYS_USER" -g "$SYS_USER" -m 0640 "$SRC/etc/${CFG_NAME}" "$PREFIX/etc/${CFG_NAME}"
    echo ">>> 已写入初始配置，请按需修改：$PREFIX/etc/${CFG_NAME}"
fi

install -m 0644 "$UNIT_SRC" "/etc/systemd/system/${SERVICE_NAME}.service"

# 若 8080 已被旧的 go run / 手工进程占用，提醒但不强制杀掉
if ss -lnt 2>/dev/null | grep -q ':8080 '; then
    echo "注意：本机 8080 端口当前已被占用。若旧进程不是本服务，请先停掉再启动。"
fi

systemctl daemon-reload
systemctl enable "${SERVICE_NAME}.service"
systemctl restart "${SERVICE_NAME}.service"

sleep 1
systemctl --no-pager --full status "${SERVICE_NAME}.service" || true
echo
echo "安装完成。常用命令："
echo "  sudo systemctl status ${SERVICE_NAME}"
echo "  sudo journalctl -u ${SERVICE_NAME} -f"
echo "  curl http://127.0.0.1:8080/health"
