#!/usr/bin/env bash
set -euo pipefail

RAIZ="${DOCKKEEPER_ROOT:-}"
NOME="dockkeeper-agent"
ANTIGO="vd-agent"

BIN_DEST="$RAIZ/usr/local/bin/$NOME"
UNIT_DEST="$RAIZ/etc/systemd/system/$NOME.service"
ENV_DEST="$RAIZ/etc/$NOME.env"
ESTADO_DEST="$RAIZ/var/lib/private/$NOME"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_SRC="${1:-$SCRIPT_DIR/../../dist/agent-linux-amd64}"

if [[ -z "$RAIZ" && $EUID -ne 0 ]]; then
    echo "erro: rode como root (sudo $0)" >&2
    exit 1
fi

if [[ ! -f "$BIN_SRC" ]]; then
    echo "erro: binario nao encontrado em $BIN_SRC" >&2
    echo "gere com: make agent-linux-amd64 (na raiz do backend)" >&2
    exit 1
fi

mover_sem_sobrescrever() {
    local origem="$1" destino="$2"
    if [[ ! -e "$destino" ]]; then
        mv "$origem" "$destino"
        echo "  migrado: ${origem#"$RAIZ"} -> ${destino#"$RAIZ"}"
    else
        mv -f "$origem" "$destino.anterior"
        echo "  AVISO: ${destino#"$RAIZ"} ja existia e foi mantido; a versao antiga ficou em ${destino#"$RAIZ"}.anterior"
    fi
}

migrar_instalacao_antiga() {
    local bin_antigo="$RAIZ/usr/local/bin/$ANTIGO"
    local unit_antiga="$RAIZ/etc/systemd/system/$ANTIGO.service"
    local env_antigo="$RAIZ/etc/$ANTIGO.env"
    local estado_privado="$RAIZ/var/lib/private/$ANTIGO"
    local estado_link="$RAIZ/var/lib/$ANTIGO"

    if [[ ! -e "$bin_antigo" && ! -e "$unit_antiga" && ! -e "$env_antigo" \
        && ! -e "$estado_privado" && ! -e "$estado_link" && ! -L "$estado_link" ]]; then
        return 0
    fi

    echo "instalacao antiga ($ANTIGO) encontrada; migrando para $NOME"
    systemctl disable --now "$ANTIGO.service" >/dev/null 2>&1 || true

    if [[ -f "$env_antigo" ]]; then
        local destino_env="$ENV_DEST"
        [[ -e "$ENV_DEST" ]] && destino_env="$ENV_DEST.anterior"
        mover_sem_sobrescrever "$env_antigo" "$ENV_DEST"
        sed -i "s#/var/lib/private/$ANTIGO/#/var/lib/$NOME/#g; s#/var/lib/$ANTIGO/#/var/lib/$NOME/#g" "$destino_env"
    fi

    local origem
    for origem in "$estado_privado" "$estado_link"; do
        if [[ -d "$origem" && ! -L "$origem" ]]; then
            install -d -m 0700 "$RAIZ/var/lib/private"
            install -d -m 0700 "$ESTADO_DEST"
            local arquivo
            for arquivo in credential.json machine-id; do
                if [[ -f "$origem/$arquivo" ]]; then
                    mover_sem_sobrescrever "$origem/$arquivo" "$ESTADO_DEST/$arquivo"
                fi
            done
            rmdir "$origem" 2>/dev/null \
                || echo "  AVISO: ${origem#"$RAIZ"} nao esta vazio e foi mantido; revise e apague"
        fi
    done
    if [[ -L "$estado_link" ]]; then
        rm -f "$estado_link"
    fi

    rm -f "$unit_antiga" "$bin_antigo"
}

migrar_instalacao_antiga

install -d -m 0755 "$(dirname "$BIN_DEST")" "$(dirname "$UNIT_DEST")"
install -m 0755 "$BIN_SRC" "${BIN_DEST}.tmp"
mv -f "${BIN_DEST}.tmp" "$BIN_DEST"
echo "binario instalado em ${BIN_DEST#"$RAIZ"}"

install -m 0644 "$SCRIPT_DIR/$NOME.service" "$UNIT_DEST"

if [[ -f "$ENV_DEST" ]]; then
    echo "config existente preservada: ${ENV_DEST#"$RAIZ"}"
else
    install -m 0600 "$SCRIPT_DIR/agent.env.exemplo" "$ENV_DEST"
    echo "config modelo criada em ${ENV_DEST#"$RAIZ"} — preencha AGENT_ENROLL_TOKEN antes de usar:"
    echo "  sudoedit ${ENV_DEST#"$RAIZ"}"
fi

systemctl daemon-reload
systemctl enable "$NOME" >/dev/null 2>&1
systemctl restart "$NOME"

sleep 1
systemctl --no-pager --lines=3 status "$NOME" || true
echo
echo "logs: journalctl -u $NOME -f"
