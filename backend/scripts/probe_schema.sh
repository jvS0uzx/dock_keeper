#!/bin/bash

set -f

PSQL_CMD="${DOCKKEEPER_PSQL_CMD:-psql}"
BASE="${DOCKKEEPER_ESQUEMA_BASE:-}"
PORTA="${DOCKKEEPER_ESQUEMA_PORTA:-5432}"
CONTAINER_NOME="${DOCKKEEPER_ESQUEMA_CONTAINER:-}"
export PGCONNECT_TIMEOUT="${PGCONNECT_TIMEOUT:-5}"

FILTRO_DE_SCHEMA="n.nspname NOT LIKE 'pg_%' AND n.nspname <> 'information_schema'"

SQL_TABELAS="SELECT n.nspname, c.relname, CASE WHEN c.reltuples < 0 OR (c.reltuples = 0 AND c.relpages = 0) THEN NULL ELSE c.reltuples::bigint END, pg_total_relation_size(c.oid) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE c.relkind IN ('r', 'p') AND $FILTRO_DE_SCHEMA ORDER BY n.nspname, c.relname;"

SQL_TABELAS_SEM_TAMANHO="SELECT n.nspname, c.relname, CASE WHEN c.reltuples < 0 OR (c.reltuples = 0 AND c.relpages = 0) THEN NULL ELSE c.reltuples::bigint END FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE c.relkind IN ('r', 'p') AND $FILTRO_DE_SCHEMA ORDER BY n.nspname, c.relname;"

SQL_COLUNAS="SELECT n.nspname, c.relname, a.attname, format_type(a.atttypid, a.atttypmod), a.attnotnull, EXISTS (SELECT 1 FROM pg_constraint k WHERE k.conrelid = c.oid AND k.contype = 'p' AND a.attnum = ANY (k.conkey)), EXISTS (SELECT 1 FROM pg_constraint k WHERE k.conrelid = c.oid AND k.contype = 'f' AND a.attnum = ANY (k.conkey)) FROM pg_attribute a JOIN pg_class c ON c.oid = a.attrelid JOIN pg_namespace n ON n.oid = c.relnamespace WHERE a.attnum > 0 AND NOT a.attisdropped AND c.relkind IN ('r', 'p') AND $FILTRO_DE_SCHEMA ORDER BY n.nspname, c.relname, a.attnum;"

SQL_RELACOES="SELECT con.conname, n.nspname, c.relname, (SELECT string_agg(a.attname, ',' ORDER BY k.ord) FROM unnest(con.conkey) WITH ORDINALITY AS k(num, ord) JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = k.num), fn.nspname, fc.relname, (SELECT string_agg(a.attname, ',' ORDER BY k.ord) FROM unnest(con.confkey) WITH ORDINALITY AS k(num, ord) JOIN pg_attribute a ON a.attrelid = con.confrelid AND a.attnum = k.num), con.confdeltype FROM pg_constraint con JOIN pg_class c ON c.oid = con.conrelid JOIN pg_namespace n ON n.oid = c.relnamespace JOIN pg_class fc ON fc.oid = con.confrelid JOIN pg_namespace fn ON fn.oid = fc.relnamespace WHERE con.contype = 'f' AND $FILTRO_DE_SCHEMA AND fn.nspname NOT LIKE 'pg_%' AND fn.nspname <> 'information_schema' ORDER BY n.nspname, c.relname, con.conname;"

limpar_texto() {
  tr -d '\r\n' | tr -cd 'A-Za-z0-9 ._:/()[]=,-' | cut -c1-200
}

texto() {
  printf '%s' "$1" | limpar_texto
}

booleano() {
  case "$1" in
    t|true|1) printf 'true' ;;
    *) printf 'false' ;;
  esac
}

booleano_negado() {
  case "$1" in
    t|true|1) printf 'false' ;;
    *) printf 'true' ;;
  esac
}

lista_json() {
  local sep item
  sep=""
  printf '['
  for item in $(printf '%s' "$1" | tr ',' ' '); do
    printf '%s"%s"' "$sep" "$(texto "$item")"
    sep=","
  done
  printf ']'
}

consultar() {
  local saida rc
  if [ -n "$CONTAINER_NOME" ]; then
    saida=$(docker exec "$CONTAINER_NOME" psql -U postgres -w -At -F'|' -d "$BASE" -c "$1" 2>/dev/null </dev/null)
  else
    saida=$($PSQL_CMD -w -At -F'|' -p "$PORTA" -d "$BASE" -c "$1" 2>/dev/null </dev/null)
  fi
  rc=$?
  printf '%s\n' "$saida" | tr -d '\r'
  return "$rc"
}

erro_da_consulta() {
  if [ -n "$CONTAINER_NOME" ]; then
    docker exec "$CONTAINER_NOME" psql -U postgres -w -At -F'|' -d "$BASE" -c "$1" 2>&1 >/dev/null </dev/null | head -1 | limpar_texto
  else
    $PSQL_CMD -w -At -F'|' -p "$PORTA" -d "$BASE" -c "$1" 2>&1 >/dev/null </dev/null | head -1 | limpar_texto
  fi
}

classificar_erro() {
  local detalhe baixo
  detalhe="$1"
  if [ -z "$detalhe" ]; then
    detalhe="psql nao devolveu detalhe do erro"
  fi
  baixo=$(printf '%s' "$detalhe" | tr 'A-Z' 'a-z')
  case "$baixo" in
    *"database"*"does not exist"*|*"base de dados"*"nao existe"*)
      CLASSE="base_inexistente"
      ;;
    *permission*|*permiss*|*denied*|*authentic*|*autentic*|*password*|*senha*|*pg_hba*)
      CLASSE="sem_permissao"
      ;;
    *)
      CLASSE="indisponivel"
      ;;
  esac
  DETALHE="$detalhe"
}

emitir_falha() {
  printf '{"ok":false,"classe":"%s","erro":"%s","tabelas":[],"colunas":[],"relacoes":[]}\n' \
    "$1" "$(texto "$2")"
}

emitir_tabela() {
  local extras
  extras=""
  if printf '%s' "$3" | grep -Eq '^[0-9]+$'; then
    extras="$extras,\"linhas_estimadas\":$3"
  fi
  if printf '%s' "$4" | grep -Eq '^[0-9]+$'; then
    extras="$extras,\"tamanho_bytes\":$4"
  fi
  printf '{"schema":"%s","nome":"%s"%s}' "$(texto "$1")" "$(texto "$2")" "$extras"
}

tabelas_com_tamanho() {
  local sep schema nome linhas tamanho
  sep=""
  while IFS='|' read -r schema nome linhas tamanho; do
    [ -n "$nome" ] || continue
    printf '%s' "$sep"
    sep=","
    emitir_tabela "$schema" "$nome" "$linhas" "$tamanho"
  done
}

tabelas_sem_tamanho() {
  local sep schema nome linhas
  sep=""
  while IFS='|' read -r schema nome linhas; do
    [ -n "$nome" ] || continue
    printf '%s' "$sep"
    sep=","
    emitir_tabela "$schema" "$nome" "$linhas" ""
  done
}

colunas_json() {
  local sep schema tabela nome tipo naonulo primaria estrangeira
  sep=""
  while IFS='|' read -r schema tabela nome tipo naonulo primaria estrangeira; do
    [ -n "$nome" ] || continue
    printf '%s' "$sep"
    sep=","
    printf '{"schema":"%s","tabela":"%s","nome":"%s","tipo":"%s","nulo":%s,"chave_primaria":%s,"chave_estrangeira":%s}' \
      "$(texto "$schema")" "$(texto "$tabela")" "$(texto "$nome")" "$(texto "$tipo")" \
      "$(booleano_negado "$naonulo")" \
      "$(booleano "$primaria")" "$(booleano "$estrangeira")"
  done
}

relacoes_json() {
  local sep nome schema tabela colunas fschema ftabela fcolunas apagar
  sep=""
  while IFS='|' read -r nome schema tabela colunas fschema ftabela fcolunas apagar; do
    [ -n "$nome" ] || continue
    [ -n "$colunas" ] || continue
    [ -n "$fcolunas" ] || continue
    printf '%s' "$sep"
    sep=","
    printf '{"nome":"%s","de_schema":"%s","de_tabela":"%s","de_colunas":%s,"para_schema":"%s","para_tabela":"%s","para_colunas":%s,"ao_apagar":"%s"}' \
      "$(texto "$nome")" "$(texto "$schema")" "$(texto "$tabela")" "$(lista_json "$colunas")" \
      "$(texto "$fschema")" "$(texto "$ftabela")" "$(lista_json "$fcolunas")" "$(texto "$apagar")"
  done
}

BASE_SEGURA=$(printf '%s' "$BASE" | tr -cd 'A-Za-z0-9_.-' | cut -c1-63)
if [ -z "$BASE" ] || [ "$BASE_SEGURA" != "$BASE" ]; then
  emitir_falha "base_invalida" "Nome de base recusado pela sonda de esquema: $BASE_SEGURA"
  exit 0
fi

SAIDA_TABELAS=$(consultar "$SQL_TABELAS")
if [ $? -eq 0 ]; then
  TABELAS=$(printf '%s\n' "$SAIDA_TABELAS" | tabelas_com_tamanho)
else
  SAIDA_TABELAS=$(consultar "$SQL_TABELAS_SEM_TAMANHO")
  if [ $? -ne 0 ]; then
    classificar_erro "$(erro_da_consulta "$SQL_TABELAS_SEM_TAMANHO")"
    emitir_falha "$CLASSE" "Nao foi possivel ler o catalogo de tabelas: $DETALHE"
    exit 0
  fi
  TABELAS=$(printf '%s\n' "$SAIDA_TABELAS" | tabelas_sem_tamanho)
fi

SAIDA_COLUNAS=$(consultar "$SQL_COLUNAS")
if [ $? -ne 0 ]; then
  classificar_erro "$(erro_da_consulta "$SQL_COLUNAS")"
  emitir_falha "$CLASSE" "Nao foi possivel ler as colunas do catalogo: $DETALHE"
  exit 0
fi

SAIDA_RELACOES=$(consultar "$SQL_RELACOES")
if [ $? -ne 0 ]; then
  classificar_erro "$(erro_da_consulta "$SQL_RELACOES")"
  emitir_falha "$CLASSE" "Nao foi possivel ler as chaves estrangeiras: $DETALHE"
  exit 0
fi

COLUNAS=$(printf '%s\n' "$SAIDA_COLUNAS" | colunas_json)
RELACOES=$(printf '%s\n' "$SAIDA_RELACOES" | relacoes_json)

printf '{"ok":true,"classe":"","erro":"","tabelas":[%s],"colunas":[%s],"relacoes":[%s]}\n' \
  "$TABELAS" "$COLUNAS" "$RELACOES"
