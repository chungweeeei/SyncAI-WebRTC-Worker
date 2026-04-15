#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="syncai-webrtc-worker"
INSTALL_DIR="/opt/${SERVICE_NAME}"
SYSTEMD_DIR="/etc/systemd/system"

# Resolve script directory to find the tar.gz
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# ---------- Locate artifact ----------
if [ -n "${1:-}" ]; then
    TARBALL="$1"
elif [ -f "${SCRIPT_DIR}/${SERVICE_NAME}.tar.gz" ]; then
    TARBALL="${SCRIPT_DIR}/${SERVICE_NAME}.tar.gz"
else
    # Search current directory
    TARBALL="$(ls -t ${SERVICE_NAME}-*.tar.gz 2>/dev/null | head -1 || true)"
fi

if [ -z "$TARBALL" ] || [ ! -f "$TARBALL" ]; then
    echo "Usage: $0 <path-to-tarball>"
    echo "  e.g. $0 syncai-webrtc-worker-20260415-170000.tar.gz"
    exit 1
fi

echo "==> Deploying from: ${TARBALL}"

# ---------- Create install directory ----------
sudo mkdir -p "${INSTALL_DIR}"

# ---------- Stop service if running ----------
if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
    echo "==> Stopping ${SERVICE_NAME}..."
    sudo systemctl stop "${SERVICE_NAME}"
fi

# ---------- Extract artifact ----------
echo "==> Extracting to ${INSTALL_DIR}..."
sudo tar -xzf "${TARBALL}" -C "${INSTALL_DIR}"
sudo chmod +x "${INSTALL_DIR}/${SERVICE_NAME}"

# ---------- Install service file ----------
if [ -f "${INSTALL_DIR}/${SERVICE_NAME}.service" ]; then
    echo "==> Installing systemd service..."
    sudo cp "${INSTALL_DIR}/${SERVICE_NAME}.service" "${SYSTEMD_DIR}/"
    sudo systemctl daemon-reload
fi

# ---------- Enable and start ----------
echo "==> Starting ${SERVICE_NAME}..."
sudo systemctl enable "${SERVICE_NAME}"
sudo systemctl start "${SERVICE_NAME}"

# ---------- Verify ----------
sleep 2
if systemctl is-active --quiet "${SERVICE_NAME}"; then
    echo "==> ${SERVICE_NAME} is running."
    systemctl status "${SERVICE_NAME}" --no-pager
else
    echo "==> ${SERVICE_NAME} failed to start. Check logs:"
    echo "    journalctl -u ${SERVICE_NAME} -n 20 --no-pager"
    exit 1
fi
