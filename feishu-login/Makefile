BINARY := password-self-service
CONFIG := etc/password-self-service.yaml

.PHONY: run build ui tidy swag clean install

# 重新构建登录页（源码在 ../login-new）后再编译
ui:
	cd ../login-new && npm install && npm run build

# 本地开发启动
run:
	@echo ">>> $(BINARY) are running"
	go run main.go --config $(CONFIG)

# 编译二进制（有 vendor 时离线）
build:
	CGO_ENABLED=0 go build -mod=vendor -ldflags="-s -w" -o bin/$(BINARY) main.go

# 安装为 systemd 服务（需 root）
install: build
	@sudo ./deploy/install.sh

# 拉取/整理依赖
tidy:
	go mod tidy

# 生成 swagger 文档（swag 需在 PATH 中，或软链接到 /bin/swag）
swag:
	rm -rf docs
	swag init

clean:
	rm -rf bin docs

# 完整初始化：拉依赖 + 生成文档
init: tidy swag
