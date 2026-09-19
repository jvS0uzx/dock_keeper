#!/usr/bin/env bash
set -euo pipefail

RAIZ="${DOCKKEEPER_ROOT:-}"
NOME="dockkeeper-agent"
ANTIGO="vd-agent"

if [[ -z "$RAIZ" && $EUID -ne 0 ]]; then
    echo "erro: rode como root (sudo $0)" >&2
    exit 1
fi

for servico in "$NOME" "$ANTIGO"; do
    systemctl disable --now "$servico.service" >/dev/null 2>&1 || true
    rm -f "$RAIZ/etc/systemd/system/$servico.service" "$RAIZ/usr/local/bin/$servico"
done
systemctl daemon-reload
echo "servico e binario removidos"

configs=()
for arquivo in "$RAIZ/etc/$NOME.env" "$RAIZ/etc/$NOME.env.anterior" "$RAIZ/etc/$ANTIGO.env" \
    "$RAIZ/var/lib/private/$ANTIGO/credential.json" "$RAIZ/var/lib/private/$ANTIGO/machine-id" \
    "$RAIZ/var/lib/$ANTIGO/credential.json" "$RAIZ/var/lib/$ANTIGO/machine-id"; do
    [[ -f "$arquivo" && ! -L "$(dirname "$arquivo")" ]] && configs+=("$arquivo")
done

if (( ${#configs[@]} > 0 )); then
    read -r -p "apagar tambem config e credencial (${configs[*]#"$RAIZ"})? [s/N] " resposta || resposta=""
    if [[ "${resposta,,}" == "s" ]]; then
        rm -f "${configs[@]}"
        rmdir "$RAIZ/var/lib/private/$ANTIGO" 2>/dev/null || true
        if [[ -L "$RAIZ/var/lib/$ANTIGO" ]]; then
            rm -f "$RAIZ/var/lib/$ANTIGO"
        else
            rmdir "$RAIZ/var/lib/$ANTIGO" 2>/dev/null || true
        fi
        echo "config removida"
    else
        echo "config preservada"
    fi
fi
