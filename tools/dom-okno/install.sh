#!/usr/bin/env bash
# Ставит «окно дома» как службу systemd --user.
# Запуск: bash install.sh   (из каталога tools/dom-okno)
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="$HOME/.local/lib/dom-okno"
UNIT_DIR="$HOME/.config/systemd/user"

mkdir -p "$LIB" "$UNIT_DIR"
install -m 644 "$HERE/okno.py" "$LIB/okno.py"

# Юнит собирается здесь, чтобы путь был верный для любого пользователя.
cat > "$UNIT_DIR/dom-okno.service" <<EOF
[Unit]
Description=Окно дома — пульт проекта ПОТОК и ноутбука
After=openobserve.service

[Service]
Type=simple
ExecStart=/usr/bin/python3 $LIB/okno.py
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
EOF

systemctl --user daemon-reload
systemctl --user enable --now dom-okno.service
sleep 1
systemctl --user restart dom-okno.service

echo "окно дома: http://127.0.0.1:${OKNO_PORT:-8799}/"
echo "состояние: systemctl --user status dom-okno --no-pager"
