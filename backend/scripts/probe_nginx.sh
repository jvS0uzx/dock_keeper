#!/bin/bash

NGINX_CMD="${DOCKKEEPER_NGINX_CMD:-nginx}"
LOG="${DOCKKEEPER_NGINX_LOG:-/var/log/nginx/access.log}"

limpar_texto() {
  tr -d '\r\n' | tr -cd 'A-Za-z0-9 ._:/()=,-' | cut -c1-200
}

upstreams_json() {
  awk '
    /^[[:space:]]*upstream[[:space:]]+/ {
      bloco = $2
      gsub(/[^A-Za-z0-9_.-]/, "", bloco)
      dentro = 1
      n = 0
      next
    }
    dentro && /^[[:space:]]*server[[:space:]]+/ {
      d = $2
      sub(/;.*$/, "", d)
      gsub(/[^A-Za-z0-9_.:\/-]/, "", d)
      if (d != "") { destinos[n++] = d }
      next
    }
    dentro && /}/ {
      if (bloco != "" && n > 0) {
        if (saida++) printf ","
        printf "{\"bloco\":\"%s\",\"destinos\":[", bloco
        for (i = 0; i < n; i++) { if (i) printf ","; printf "\"%s\"", destinos[i] }
        printf "]}"
      }
      delete destinos
      dentro = 0
      n = 0
      bloco = ""
    }
  '
}

USUARIO=$(id -un 2>/dev/null | limpar_texto)

INSTALADO=false
if command -v nginx >/dev/null 2>&1 || pgrep -x nginx >/dev/null 2>&1; then
  INSTALADO=true
fi

ATIVO=false
if [ "$INSTALADO" = true ]; then
  if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet nginx 2>/dev/null; then
    ATIVO=true
  elif command -v service >/dev/null 2>&1 && service nginx status >/dev/null 2>&1; then
    ATIVO=true
  elif pgrep -x nginx >/dev/null 2>&1; then
    ATIVO=true
  fi
fi

CONFIG_LIDA=false
CONFIG_MOTIVO=""
UPSTREAMS=""
if [ "$ATIVO" = true ]; then
  if CONFIG_OUT=$($NGINX_CMD -T 2>/dev/null) && [ -n "$CONFIG_OUT" ]; then
    CONFIG_LIDA=true
    UPSTREAMS=$(printf '%s\n' "$CONFIG_OUT" | upstreams_json)
  else
    CONFIG_MOTIVO=$($NGINX_CMD -T 2>&1 >/dev/null | head -1 | limpar_texto)
    if [ -z "$CONFIG_MOTIVO" ]; then
      CONFIG_MOTIVO="nginx -T nao retornou configuracao"
    fi
  fi
fi

LOG_LEGIVEL=false
LOG_MOTIVO=""
if [ -r "$LOG" ]; then
  LOG_LEGIVEL=true
elif [ -e "$LOG" ]; then
  LOG_MOTIVO="arquivo existe e nao e legivel"
else
  LOG_MOTIVO="arquivo nao encontrado"
fi

LOG_SEGURO=$(printf '%s' "$LOG" | limpar_texto)

printf '{"instalado":%s,"ativo":%s,"config_lida":%s,"config_motivo":"%s","log_legivel":%s,"log_motivo":"%s","log_caminho":"%s","usuario":"%s","upstreams":[%s]}\n' \
  "$INSTALADO" "$ATIVO" "$CONFIG_LIDA" "$CONFIG_MOTIVO" "$LOG_LEGIVEL" "$LOG_MOTIVO" "$LOG_SEGURO" "$USUARIO" "$UPSTREAMS"
