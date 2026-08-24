#!/usr/bin/env bash
# Instala (ou atualiza) o agente de estacao do vd_stats como servico systemd.
#
# Uso:
#   sudo ./install.sh [caminho-do-binario]
#
# Sem argumento, usa dist/agent-linux-amd64 relativo a raiz do backend
# (gerado por `make agent-linux-amd64`). E idempotente: rodar de novo troca o
# binario e reinicia o servico; o /etc/vd-agent.env existente nunca e
# sobrescrito.
set -euo pipefail

BIN_DEST="/usr/local/bin/vd-agent"
UNIT_DEST="/etc/systemd/system/vd-agent.service"
ENV_DEST="/etc/vd-agent.env"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_SRC="${1:-$SCRIPT_DIR/../../dist/agent-linux-amd64}"

if [[ $EUID -ne 0 ]]; then
    echo "erro: rode como root (sudo $0)" >&2
    exit 1
fi

if [[ ! -f "$BIN_SRC" ]]; then
    echo "erro: binario nao encontrado em $BIN_SRC" >&2
    echo "gere com: make agent-linux-amd64 (na raiz do backend)" >&2
    exit 1
fi

# install atomico: escreve com nome temporario e faz rename, para nunca deixar
# um binario pela metade se o disco encher no meio da copia.
install -m 0755 "$BIN_SRC" "${BIN_DEST}.tmp"
mv -f "${BIN_DEST}.tmp" "$BIN_DEST"
echo "binario instalado em $BIN_DEST"

install -m 0644 "$SCRIPT_DIR/vd-agent.service" "$UNIT_DEST"

if [[ -f "$ENV_DEST" ]]; then
    echo "config existente preservada: $ENV_DEST"
else
    install -m 0600 "$SCRIPT_DIR/agent.env" "$ENV_DEST"
    echo "config modelo criada em $ENV_DEST — EDITE o token antes de usar:"
    echo "  sudoedit $ENV_DEST"
fi

systemctl daemon-reload
systemctl enable vd-agent >/dev/null 2>&1
systemctl restart vd-agent

sleep 1
systemctl --no-pager --lines=3 status vd-agent || true
echo
echo "logs: journalctl -u vd-agent -f"
