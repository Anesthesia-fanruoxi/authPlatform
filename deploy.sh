#!/usr/bin/env bash
# 一键部署：编译 linux 二进制 → 创建 /data/authPlatform → 生成 systemd service 并启动。
# 用法：将代码拷贝到服务器后执行 sudo bash deploy.sh（服务器需 Go 1.25+）
# 端口：默认 8080，通过 service 环境变量注入（yaml 不指定），自定义：sudo bash deploy.sh 9000
set -euo pipefail

APP=authPlatform
SRC_DIR=$(cd "$(dirname "$0")" && pwd)
INSTALL_DIR=/data/authPlatform
SERVICE_FILE=/etc/systemd/system/${APP}.service
PORT=${1:-8080}
case "$PORT" in ''|*[!0-9]*) echo "端口参数必须为数字，用法: sudo bash deploy.sh [端口]"; exit 1;; esac

[ "$(id -u)" -eq 0 ] || { echo "请用 root/sudo 执行"; exit 1; }
command -v go >/dev/null 2>&1 || { echo "未检测到 Go 环境，请先安装 Go 1.25+"; exit 1; }

# 1. 编译 linux 版本
echo ">> 编译 ${APP} ..."
cd "$SRC_DIR"
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$APP" .

# 2. 创建服务目录并拷贝产物（已有 config.yaml 一并带上，否则首次启动走 Web 初始化页）
echo ">> 创建 ${INSTALL_DIR} ..."
mkdir -p "$INSTALL_DIR"
cp -f "$APP" "$INSTALL_DIR/"
[ -f config.yaml ] && cp -f config.yaml "$INSTALL_DIR/"

# 3. 生成 systemd service 文件
echo ">> 生成 ${SERVICE_FILE} ..."
cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=authPlatform unified auth center
After=network.target

[Service]
Type=simple
Environment=APP_ADDR=:${PORT}
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/${APP}
Restart=on-failure
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

# 4. 端口占用检查：本服务占用则 restart 会释放；被其他进程占用则中止
if ! systemctl is-active --quiet "$APP" && command -v ss >/dev/null 2>&1; then
  if ss -ltn | awk '{print $4}' | grep -qE "[:.]${PORT}$"; then
    echo "端口 ${PORT} 已被其他进程占用，请先释放或换端口重试：sudo bash deploy.sh <新端口>"
    exit 1
  fi
fi

# 5. 启用并启动
systemctl daemon-reload
systemctl enable "$APP" >/dev/null
systemctl restart "$APP"
sleep 1
systemctl --no-pager status "$APP" | head -5
echo ">> 部署完成：${INSTALL_DIR}，管理命令 systemctl {status|restart|stop} ${APP}"
