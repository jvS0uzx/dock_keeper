#!/bin/bash

MYSQL_CMD="${DOCKKEEPER_MYSQL_CMD:-mysql}"
SS_CMD="${DOCKKEEPER_SS_CMD:-ss}"
NETSTAT_CMD="${DOCKKEEPER_NETSTAT_CMD:-netstat}"
UID_EFETIVO=$(id -u 2>/dev/null | tr -cd '0-9')
TEMPO_DE_CONEXAO=5
PORTA_PADRAO=3306

SQL_VERSAO="SELECT VERSION(), @@version_comment;"
SQL_REPLICA="SHOW REPLICA STATUS;"
SQL_SLAVE="SHOW SLAVE STATUS;"
SQL_BASES="SELECT s.schema_name, s.default_character_set_name, COALESCE((SELECT SUM(t.data_length + t.index_length) FROM information_schema.tables t WHERE t.table_schema = s.schema_name), 0), CASE WHEN EXISTS (SELECT 1 FROM information_schema.user_privileges u WHERE u.privilege_type = 'PROCESS' AND REPLACE(u.grantee, '''', '') = CURRENT_USER()) THEN (SELECT COUNT(*) FROM information_schema.processlist p WHERE p.db = s.schema_name) END FROM information_schema.schemata s WHERE s.schema_name NOT IN ('mysql', 'information_schema', 'performance_schema', 'sys') ORDER BY s.schema_name;"
SQL_BASES_SEM_CONEXOES="SELECT s.schema_name, s.default_character_set_name, COALESCE((SELECT SUM(t.data_length + t.index_length) FROM information_schema.tables t WHERE t.table_schema = s.schema_name), 0) FROM information_schema.schemata s WHERE s.schema_name NOT IN ('mysql', 'information_schema', 'performance_schema', 'sys') ORDER BY s.schema_name;"

CLIENTE_NO_CONTAINER='senha="${MYSQL_ROOT_PASSWORD:-${MARIADB_ROOT_PASSWORD:-}}"
arquivo="${MYSQL_ROOT_PASSWORD_FILE:-${MARIADB_ROOT_PASSWORD_FILE:-}}"
if [ -z "$senha" ] && [ -n "$arquivo" ] && [ -r "$arquivo" ]; then senha=$(cat "$arquivo"); fi
unset arquivo
if [ -n "$senha" ]; then MYSQL_PWD="$senha"; export MYSQL_PWD; fi
unset senha
cliente=mysql
if ! command -v mysql >/dev/null 2>&1; then cliente=mariadb; fi
exec "$cliente" -uroot --connect-timeout='"$TEMPO_DE_CONEXAO"' -N -B -e "$1"'

limpar_texto() {
  tr -d '\r\n' | tr -cd 'A-Za-z0-9 ._:/()=,@-' | cut -c1-200
}

rodar_cliente() {
  if [ "$EM_CONTAINER" = true ]; then
    docker exec "$CONTAINER_NOME" sh -c "$CLIENTE_NO_CONTAINER" dockkeeper "$1"
  elif [ "$PORTA" = "$PORTA_PADRAO" ]; then
    $MYSQL_CMD --connect-timeout="$TEMPO_DE_CONEXAO" -N -B -e "$1"
  else
    $MYSQL_CMD --connect-timeout="$TEMPO_DE_CONEXAO" --protocol=TCP -h 127.0.0.1 -P "$PORTA" -N -B -e "$1"
  fi
}

consultar() {
  local saida rc
  saida=$(rodar_cliente "$1" 2>/dev/null </dev/null)
  rc=$?
  printf '%s\n' "$saida" | tr -d '\r'
  return "$rc"
}

erro_da_consulta() {
  rodar_cliente "$1" 2>&1 </dev/null | awk 'NF && p == "" { p = $0 } /ERROR/ && e == "" { e = $0 } END { if (e != "") print e; else print p }' | limpar_texto
}

cliente_disponivel() {
  local bin
  if [ "$EM_CONTAINER" = true ]; then
    command -v docker >/dev/null 2>&1
    return
  fi
  bin=$(printf '%s\n' "$MYSQL_CMD" | tr ' ' '\n' | awk 'NF { n = $0; sub(/.*\//, "", n); if (n == "mysql" || n == "mariadb") { print $0; exit } }')
  if [ -z "$bin" ]; then
    bin=$(printf '%s\n' "$MYSQL_CMD" | awk '{print $1}')
  fi
  command -v "$bin" >/dev/null 2>&1
}

preparar_cliente_do_host() {
  local alternativo
  cliente_disponivel && return 0
  alternativo=$(printf '%s\n' "$MYSQL_CMD" | awk '{ for (i = 1; i <= NF; i++) { n = $i; sub(/.*\//, "", n); if (n == "mysql") { sub(/mysql$/, "mariadb", $i); break } } print }')
  [ "$alternativo" != "$MYSQL_CMD" ] || return 1
  MYSQL_CMD="$alternativo"
  cliente_disponivel
}

classificar_falha() {
  local detalhe baixo
  detalhe="$1"
  if [ -z "$detalhe" ]; then
    detalhe="o cliente mysql nao devolveu detalhe do erro"
  fi
  baixo=$(printf '%s' "$detalhe" | tr 'A-Z' 'a-z')
  case "$baixo" in
    *"access denied"*|*authentic*|*autentic*|*password*|*senha*|*denied*|*"permission"*|*"plugin"*)
      ESTADO="sem_acesso"
      MOTIVO="Instância detectada, sem credencial ou permissão para consultar: $detalhe"
      ;;
    *"executable file not found"*|*"not found"*)
      ESTADO="desconhecido"
      MOTIVO="Instância detectada, sem cliente mysql disponível para consultá-la: $detalhe"
      ;;
    *"can't connect"*|*"cant connect"*|*"lost connection"*|*"connection refused"*|*"no such container"*|*"is not running"*|*"is restarting"*|*"is paused"*|*"server has gone away"*)
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
  local nome enc extras
  nome=$(printf '%s' "$1" | limpar_texto)
  enc=$(printf '%s' "$2" | limpar_texto)
  extras=""
  if printf '%s' "$3" | grep -Eq '^[0-9]+$'; then
    extras="$extras,\"tamanho_bytes\":$3"
  fi
  if printf '%s' "$4" | grep -Eq '^[0-9]+$'; then
    extras="$extras,\"conexoes\":$4"
  fi
  printf '{"nome":"%s","dono":"","encoding":"%s"%s}' "$nome" "$enc" "$extras"
}

emitir_bases() {
  local sep nome enc tam con
  sep=""
  while IFS="$(printf '\t')" read -r nome enc tam con; do
    [ -n "$nome" ] || continue
    printf '%s' "$sep"
    sep=","
    emitir_base "$nome" "$enc" "$tam" "$con"
  done
}

consultar_bases() {
  local saida
  saida=$(consultar "$SQL_BASES")
  if [ $? -ne 0 ] || [ -z "$saida" ]; then
    saida=$(consultar "$SQL_BASES_SEM_CONEXOES")
    if [ $? -ne 0 ] || [ -z "$saida" ]; then
      return
    fi
  fi
  printf '%s\n' "$saida" | emitir_bases
}

consultar_papel() {
  local saida
  saida=$(consultar "$SQL_REPLICA")
  if [ $? -ne 0 ]; then
    saida=$(consultar "$SQL_SLAVE")
    if [ $? -ne 0 ]; then
      PAPEL="desconhecido"
      return
    fi
  fi
  if [ -n "$(printf '%s' "$saida" | tr -d '[:space:]')" ]; then
    PAPEL="replica"
  else
    PAPEL="primario"
  fi
}

motor_da_versao() {
  case "$(printf '%s' "$1" | tr 'A-Z' 'a-z')" in
    *mariadb*) printf 'mariadb' ;;
    *) printf 'mysql' ;;
  esac
}

emitir_instancia() {
  printf '{"porta":%s,"em_container":%s,"container_nome":"%s","motor":"%s","versao":"%s","papel":"%s","wal_level":"","archive_mode":"","estado":"%s","motivo":"%s","bases":[%s]}' \
    "$PORTA" "$EM_CONTAINER" "$CONTAINER_SEGURO" "$MOTOR" "$VERSAO" "$PAPEL" "$ESTADO" "$MOTIVO" "$BASES"
}

sondar_instancia() {
  local linha versao comentario
  VERSAO=""
  PAPEL="desconhecido"
  ESTADO="desconhecido"
  MOTIVO=""
  BASES=""
  MOTOR="$MOTOR_PROVAVEL"
  CONTAINER_SEGURO=$(printf '%s' "$CONTAINER_NOME" | limpar_texto)

  if [ "$EM_CONTAINER" = true ] && ! cliente_disponivel; then
    MOTIVO="Instância detectada, sem docker disponível para consultá-la"
    emitir_instancia
    return
  fi
  if [ "$EM_CONTAINER" = false ] && ! preparar_cliente_do_host; then
    MOTIVO="Instância detectada, sem cliente mysql disponível para consultá-la"
    emitir_instancia
    return
  fi

  linha=$(consultar "$SQL_VERSAO")
  if [ $? -ne 0 ] || [ -z "$linha" ]; then
    classificar_falha "$(erro_da_consulta "$SQL_VERSAO")"
    emitir_instancia
    return
  fi

  ESTADO="ativo"
  linha=$(printf '%s\n' "$linha" | head -1)
  versao=$(printf '%s' "$linha" | cut -f1)
  comentario=$(printf '%s' "$linha" | cut -s -f2)
  MOTOR=$(motor_da_versao "$versao $comentario")
  VERSAO=$(printf '%s' "$versao" | cut -d- -f1 | limpar_texto | cut -c1-32)

  consultar_papel
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
    tolower($0) ~ /mysqld|mariadbd/ {
      motor = "mysql"
      if (tolower($0) ~ /mariadbd/) { motor = "mariadb" }
      for (i = 1; i <= NF; i++) {
        if ($i ~ /:[0-9]+$/) {
          porta = $i
          sub(/.*:/, "", porta)
          porta = porta + 0
          if (porta > 0 && porta != 33060 && porta != 33062) { print porta "|" motor }
          break
        }
      }
    }' | sort -t'|' -k1,1n -u)
}

descobrir_containers() {
  local bruto
  command -v docker >/dev/null 2>&1 || return 0
  bruto=$(docker ps --format '{{.Names}}|{{.Image}}|{{.Ports}}' 2>/dev/null </dev/null)
  [ $? -eq 0 ] || return 0
  DOCKER_OK=true
  CONTAINERS=$(printf '%s\n' "$bruto" | awk -F'|' '
    tolower($2) ~ /mysql|mariadb|percona/ {
      porta = 3306
      if (match($3, /:[0-9]+->3306\//)) {
        porta = substr($3, RSTART + 1, RLENGTH - 8)
      }
      motor = "mysql"
      if (tolower($2) ~ /mariadb/) { motor = "mariadb" }
      nome = $1
      gsub(/[^A-Za-z0-9_.-]/, "", nome)
      if (nome != "") { print nome "|" porta "|" motor }
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

while IFS='|' read -r NOME_LIDO PORTA_LIDA MOTOR_LIDO; do
  [ -n "$NOME_LIDO" ] || continue
  [ -n "$PORTA_LIDA" ] || continue
  case "$PORTAS_VISTAS" in
    *" $PORTA_LIDA "*) continue ;;
  esac
  EM_CONTAINER=true
  CONTAINER_NOME="$NOME_LIDO"
  PORTA="$PORTA_LIDA"
  MOTOR_PROVAVEL="$MOTOR_LIDO"
  PORTAS_VISTAS="$PORTAS_VISTAS$PORTA_LIDA "
  acrescentar_instancia "$(sondar_instancia)"
done <<< "$CONTAINERS"

while IFS='|' read -r PORTA_LIDA MOTOR_LIDO; do
  [ -n "$PORTA_LIDA" ] || continue
  case "$PORTAS_VISTAS" in
    *" $PORTA_LIDA "*) continue ;;
  esac
  EM_CONTAINER=false
  CONTAINER_NOME=""
  PORTA="$PORTA_LIDA"
  MOTOR_PROVAVEL="$MOTOR_LIDO"
  PORTAS_VISTAS="$PORTAS_VISTAS$PORTA_LIDA "
  acrescentar_instancia "$(sondar_instancia)"
done <<< "$PORTAS_HOST"

printf '{"descoberta_host_ok":%s,"descoberta_container_ok":%s,"instancias":[%s]}\n' \
  "$LISTAGEM_OK" "$DOCKER_OK" "$INSTANCIAS"
