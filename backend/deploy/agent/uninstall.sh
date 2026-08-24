#!/usr/bin/env bash
# Remove o agente de estacao do vd_stats (servico, unit e binario).
#
# A config /etc/vd-agent.env so e apagada com confirmacao: ela carrega o token
# e o codigo da unidade, que valem para uma reinstalacao futura.
set -euo pipefail

BIN_DEST="/usr/local/bin/vd-agent"
UNIT_DEST="/etc/systemd/system/vd-agent.service"
ENV_DEST="/etc/vd-agent.env"

if [[ $EUID -ne 0 ]]; then
    echo "erro: rode como root (sudo $0)" >&2
    exit 1
fi

if systemctl list-unit-files vd-agent.service >/dev/null 2>&1; then
    systemctl disable --now vd-agent >/dev/null 2>&1 || true
fi

rm -f "$UNIT_DEST" "$BIN_DEST"
systemctl daemon-reload
echo "servico e binario removidos"

if [[ -f "$ENV_DEST" ]]; then
    read -r -p "apagar tambem a config $ENV_DEST? [s/N] " resposta
    if [[ "${resposta,,}" == "s" ]]; then
        rm -f "$ENV_DEST"
        echo "config removida"
    else
        echo "config preservada em $ENV_DEST"
    fi
fi
