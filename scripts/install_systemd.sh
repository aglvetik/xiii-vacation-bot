#!/usr/bin/env bash
set -euo pipefail

APP_NAME="xiii-vacation-bot"
UNIT_SOURCE="systemd/${APP_NAME}.service"
UNIT_TARGET="/etc/systemd/system/${APP_NAME}.service"

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run this script as root: sudo ./scripts/install_systemd.sh"
  exit 1
fi

if [[ ! -f "${UNIT_SOURCE}" ]]; then
  echo "Systemd unit not found: ${UNIT_SOURCE}"
  exit 1
fi

echo "Building ${APP_NAME}..."
go mod tidy
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "${APP_NAME}" ./cmd/bot

echo "Installing systemd unit..."
install -m 0644 "${UNIT_SOURCE}" "${UNIT_TARGET}"

echo "Reloading systemd..."
systemctl daemon-reload
systemctl enable "${APP_NAME}"
systemctl restart "${APP_NAME}"

echo
systemctl --no-pager status "${APP_NAME}" || true
echo
echo "Logs: journalctl -u ${APP_NAME} -f"
