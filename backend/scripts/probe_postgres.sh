#!/bin/bash

PSQL_CMD="${DOCKKEEPER_PSQL_CMD:-psql}"
SS_CMD="${DOCKKEEPER_SS_CMD:-ss}"
NETSTAT_CMD="${DOCKKEEPER_NETSTAT_CMD:-netstat}"
UID_EFETIVO=$(id -u 2>/dev/null | tr -cd '0-9')
export PGCONNECT_TIMEOUT="${PGCONNECT_TIMEOUT:-5}"

SQL_VERSAO="SHOW server_version;"
SQL_PAPEL="SELECT pg_is_in_recovery();"
SQL_WAL_LEVEL="SHOW wal_level;"
SQL_MAX_WAL_SENDERS="SHOW max_wal_senders;"
SQL_ARCHIVE_MODE="SHOW archive_mode;"
SQL_BASES="SELECT d.datname, pg_catalog.pg_get_userbyid(d.datdba), pg_encoding_to_char(d.encoding), pg_database_size(d.datname), (SELECT count(*) FROM pg_stat_activity a WHERE a.datname = d.datname) FROM pg_database d WHERE NOT d.datistemplate ORDER BY d.datname;"
SQL_BASES_SEM_TAMANHO="SELECT d.datname, pg_catalog.pg_get_userbyid(d.datdba), pg_encoding_to_char(d.encoding), (SELECT count(*) FROM pg_stat_activity a WHERE a.datname = d.datname) FROM pg_database d WHERE NOT d.datistemplate ORDER BY d.datname;"

limpar_texto() {
  tr -d '\r\n' | tr -cd 'A-Za-z0-9 ._:/()=,-' | cut -c1-200
}

consultar() {
  local saida rc
  if [ "$EM_CONTAINER" = true ]; then
    saida=$(docker exec "$CONTAINER_NOME" psql -U postgres -w -At -F'|' -c "$1" 2>/dev/null </dev/null)
  else
    saida=$($PSQL_CMD -w -At -F'|' -p "$PORTA" -c "$1" 2>/dev/null </dev/null)
  fi
  rc=$?
  printf '%s\n' "$saida" | tr -d '\r'
  return "$rc"
}

erro_da_consulta() {
  if [ "$EM_CONTAINER" = true ]; then
    docker exec "$CONTAINER_NOME" psql -U postgres -w -At -F'|' -c "$1" 2>&1 >/dev/null </dev/null | head -1 | limpar_texto
  else
    $PSQL_CMD -w -At -F'|' -p "$PORTA" -c "$1" 2>&1 >/dev/null </dev/null | head -1 | limpar_texto
  fi
}

cliente_disponivel() {
  local bin
  if [ "$EM_CONTAINER" = true ]; then
    command -v docker >/dev/null 2>&1
    return
  fi
  bin=$(printf '%s\n' "$PSQL_CMD" | tr ' ' '\n' | awk 'NF { n = $0; sub(/.*\//, "", n); if (n == "psql") { print $0; exit } }')
  if [ -z "$bin" ]; then
    bin=$(printf '%s\n' "$PSQL_CMD" | awk '{print $1}')
  fi
  command -v "$bin" >/dev/null 2>&1
}

classificar_falha() {
  local detalhe baixo
  detalhe="$1"
  if [ -z "$detalhe" ]; then
    detalhe="psql nao devolveu detalhe do erro"
  fi
  baixo=$(printf '%s' "$detalhe" | tr 'A-Z' 'a-z')
  case "$baixo" in
    *authentic*|*autentic*|*permission*|*permiss*|*password*|*senha*|*pg_hba*|*denied*|*"does not exist"*|*"nao existe"*)
      ESTADO="sem_acesso"
      MOTIVO="Instância detectada, sem credencial ou permissão para consultar: $detalhe"
      ;;
    *"could not connect"*|*"connection refused"*|*"conexao recusada"*|*"server closed the connection"*|*"no such container"*|*"is not running"*)
      ESTADO="inativo"
      MOTIVO="Porta detectada, a instância não respondeu: $detalhe"
      ;;
    *)
      ESTADO="desconhecido"
      MOTIVO="Não foi possível consultar a instância: $detalhe"
      ;;
  esac
}

emitir_base() {
  local nome dono enc extras
  nome=$(printf '%s' "$1" | limpar_texto)
  dono=$(printf '%s' "$2" | limpar_texto)
  enc=$(printf '%s' "$3" | limpar_texto)
  extras=""
  if printf '%s' "$4" | grep -Eq '^[0-9]+$'; then
    extras="$extras,\"tamanho_bytes\":$4"
  fi
  if printf '%s' "$5" | grep -Eq '^[0-9]+$'; then
    extras="$extras,\"conexoes\":$5"
  fi
  printf '{"nome":"%s","dono":"%s","encoding":"%s"%s}' "$nome" "$dono" "$enc" "$extras"
}

bases_com_tamanho() {
  local sep nome dono enc tam con
  sep=""
  while IFS='|' read -r nome dono enc tam con; do
    [ -n "$nome" ] || continue
    printf '%s' "$sep"
    sep=","
    emitir_base "$nome" "$dono" "$enc" "$tam" "$con"
  done
}

bases_com_tamanho_avulso() {
  local sep nome dono enc con nome_sql tam
  sep=""
  while IFS='|' read -r nome dono enc con; do
    [ -n "$nome" ] || continue
    tam=""
    nome_sql=$(printf '%s' "$nome" | tr -cd 'A-Za-z0-9_.-')
    if [ "$nome_sql" = "$nome" ]; then
      tam=$(consultar "SELECT pg_database_size('$nome_sql');" | head -1 | tr -cd '0-9')
    fi
    printf '%s' "$sep"
    sep=","
    emitir_base "$nome" "$dono" "$enc" "$tam" "$con"
  done
}

consultar_bases() {
  local saida
  saida=$(consultar "$SQL_BASES")
  if [ $? -eq 0 ] && [ -n "$saida" ]; then
    printf '%s\n' "$saida" | bases_com_tamanho
    return
  fi
  saida=$(consultar "$SQL_BASES_SEM_TAMANHO")
  if [ $? -ne 0 ] || [ -z "$saida" ]; then
    return
  fi
  printf '%s\n' "$saida" | bases_com_tamanho_avulso
}

emitir_instancia() {
  local extras
  extras=""
  if printf '%s' "$MAX_WAL_SENDERS" | grep -Eq '^[0-9]+$'; then
    extras=",\"max_wal_senders\":$MAX_WAL_SENDERS"
  fi
  printf '{"porta":%s,"em_container":%s,"container_nome":"%s","versao":"%s","papel":"%s","wal_level":"%s"%s,"archive_mode":"%s","estado":"%s","motivo":"%s","bases":[%s]}' \
    "$PORTA" "$EM_CONTAINER" "$CONTAINER_SEGURO" "$VERSAO" "$PAPEL" "$WAL_LEVEL" "$extras" \
    "$ARCHIVE_MODE" "$ESTADO" "$MOTIVO" "$BASES"
}

sondar_instancia() {
  local versao recovery
  VERSAO=""
  PAPEL="desconhecido"
  WAL_LEVEL=""
  MAX_WAL_SENDERS=""
  ARCHIVE_MODE=""
  ESTADO="desconhecido"
  MOTIVO=""
  BASES=""
  CONTAINER_SEGURO=$(printf '%s' "$CONTAINER_NOME" | limpar_texto)

  if ! cliente_disponivel; then
    MOTIVO="Instância detectada, sem cliente psql disponível para consultá-la"
    emitir_instancia
    return
  fi

  versao=$(consultar "$SQL_VERSAO")
  if [ $? -ne 0 ] || [ -z "$versao" ]; then
    classificar_falha "$(erro_da_consulta "$SQL_VERSAO")"
    emitir_instancia
    return
  fi

  ESTADO="ativo"
  VERSAO=$(printf '%s' "$versao" | head -1 | limpar_texto)

  recovery=$(consultar "$SQL_PAPEL" | head -1 | tr -d ' ')
  case "$recovery" in
    f|false|0) PAPEL="primario" ;;
    t|true|1) PAPEL="replica" ;;
    *) PAPEL="desconhecido" ;;
  esac

  WAL_LEVEL=$(consultar "$SQL_WAL_LEVEL" | head -1 | limpar_texto)
  MAX_WAL_SENDERS=$(consultar "$SQL_MAX_WAL_SENDERS" | head -1 | tr -cd '0-9')
  ARCHIVE_MODE=$(consultar "$SQL_ARCHIVE_MODE" | head -1 | limpar_texto)
  BASES=$(consultar_bases)

  emitir_instancia
}

binario_de() {
  printf '%s\n' "$1" | awk '{print $1}'
}

listagem_privilegiada() {
  case "$(binario_de "$1")" in
    sudo|*/sudo) return 0 ;;
  esac
  [ "$UID_EFETIVO" = "0" ]
}

descobrir_portas() {
  local bruto alternativo
  bruto=""
  if command -v "$(binario_de "$SS_CMD")" >/dev/null 2>&1; then
    alternativo=$($SS_CMD -tulnp 2>/dev/null </dev/null)
    if [ $? -eq 0 ]; then
      bruto="$alternativo"
      if listagem_privilegiada "$SS_CMD"; then
        LISTAGEM_OK=true
      fi
    fi
  fi
  if [ "$LISTAGEM_OK" = false ] && command -v "$(binario_de "$NETSTAT_CMD")" >/dev/null 2>&1; then
    alternativo=$($NETSTAT_CMD -tulnp 2>/dev/null </dev/null)
    if [ $? -eq 0 ] && [ -n "$alternativo" ]; then
      if listagem_privilegiada "$NETSTAT_CMD"; then
        LISTAGEM_OK=true
        bruto="$alternativo"
      elif [ -z "$bruto" ]; then
        bruto="$alternativo"
      fi
    fi
  fi
  [ -n "$bruto" ] || return 0
  PORTAS_HOST=$(printf '%s\n' "$bruto" | awk '
    $0 !~ /(^|[[:space:]])LISTEN([[:space:]]|$)/ { next }
    tolower($0) ~ /postgres|postmaster/ {
      for (i = 1; i <= NF; i++) {
        if ($i ~ /:[0-9]+$/) {
          porta = $i
          sub(/.*:/, "", porta)
          if (porta + 0 > 0) { print porta + 0 }
          break
        }
      }
    }' | sort -un)
}

descobrir_containers() {
  local bruto
  command -v docker >/dev/null 2>&1 || return 0
  bruto=$(docker ps --format '{{.Names}}|{{.Image}}|{{.Ports}}' 2>/dev/null </dev/null)
  [ $? -eq 0 ] || return 0
  DOCKER_OK=true
  CONTAINERS=$(printf '%s\n' "$bruto" | awk -F'|' '
    tolower($2) ~ /postgres/ {
      porta = 5432
      if (match($3, /:[0-9]+->/)) {
        porta = substr($3, RSTART + 1, RLENGTH - 3)
      } else if (match($3, /[0-9]+\/tcp/)) {
        porta = substr($3, RSTART, RLENGTH - 4)
      }
      nome = $1
      gsub(/[^A-Za-z0-9_.-]/, "", nome)
      if (nome != "") { print nome "|" porta }
    }')
}

acrescentar_instancia() {
  if [ -n "$INSTANCIAS" ]; then
    INSTANCIAS="$INSTANCIAS,$1"
  else
    INSTANCIAS="$1"
  fi
}

INSTANCIAS=""
PORTAS_VISTAS=" "
LISTAGEM_OK=false
DOCKER_OK=false
PORTAS_HOST=""
CONTAINERS=""

descobrir_containers
descobrir_portas

while IFS='|' read -r NOME_LIDO PORTA_LIDA; do
  [ -n "$NOME_LIDO" ] || continue
  [ -n "$PORTA_LIDA" ] || continue
  case "$PORTAS_VISTAS" in
    *" $PORTA_LIDA "*) continue ;;
  esac
  EM_CONTAINER=true
  CONTAINER_NOME="$NOME_LIDO"
  PORTA="$PORTA_LIDA"
  PORTAS_VISTAS="$PORTAS_VISTAS$PORTA_LIDA "
  acrescentar_instancia "$(sondar_instancia)"
done <<< "$CONTAINERS"

while read -r PORTA_LIDA; do
  [ -n "$PORTA_LIDA" ] || continue
  case "$PORTAS_VISTAS" in
    *" $PORTA_LIDA "*) continue ;;
  esac
  EM_CONTAINER=false
  CONTAINER_NOME=""
  PORTA="$PORTA_LIDA"
  PORTAS_VISTAS="$PORTAS_VISTAS$PORTA_LIDA "
  acrescentar_instancia "$(sondar_instancia)"
done <<< "$PORTAS_HOST"

printf '{"descoberta_host_ok":%s,"descoberta_container_ok":%s,"instancias":[%s]}\n' \
  "$LISTAGEM_OK" "$DOCKER_OK" "$INSTANCIAS"
