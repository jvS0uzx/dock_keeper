#!/usr/bin/env bash
set -euo pipefail

raiz="$(cd "$(dirname "$0")/../.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

: > "$tmp/chave"
: > "$tmp/known_hosts"

python3 - "$raiz/.env.example" "$tmp" <<'PY'
import re
import sys

exemplo, tmp = sys.argv[1], sys.argv[2]
fixos = {
    "SSH_KEY_PATH": f"{tmp}/chave",
    "SSH_KNOWN_HOSTS": f"{tmp}/known_hosts",
}
nomes = []
linhas = []
for linha in open(exemplo, encoding="utf-8"):
    m = re.match(r"^#?\s*([A-Z][A-Z0-9_]*)=(.*)$", linha.rstrip("\n"))
    if not m:
        continue
    nome, valor = m.group(1), m.group(2).strip()
    if nome in nomes:
        continue
    nomes.append(nome)
    if nome in fixos:
        valor = fixos[nome]
    elif valor in ('""', "''", ""):
        valor = "valor-de-teste"
    linhas.append(f"{nome}={valor}")
open(f"{tmp}/.env", "w", encoding="utf-8").write("\n".join(linhas) + "\n")
open(f"{tmp}/nomes", "w", encoding="utf-8").write("\n".join(nomes) + "\n")
PY

docker compose --project-directory "$tmp" -f "$raiz/docker-compose.yml" config --format json > "$tmp/config.json"

python3 - "$tmp" <<'PY'
import json
import sys

tmp = sys.argv[1]
config = json.load(open(f"{tmp}/config.json", encoding="utf-8"))
ambiente = config["services"]["backend"].get("environment") or {}
nomes = [n for n in open(f"{tmp}/nomes", encoding="utf-8").read().split("\n") if n]

faltando = [n for n in nomes if n not in ambiente]
erros = []
if faltando:
    erros.append(f"{len(faltando)} de {len(nomes)} variáveis do .env.example não chegam ao backend: " + ", ".join(faltando))

esperado = {
    "SSH_KEY_PATH": "/run/secrets/ssh_key",
    "SSH_KNOWN_HOSTS": "/run/secrets/known_hosts",
    "TRUST_PROXY_HEADERS": "true",
    "API_ADDR": ":8080",
    "FLOORPLAN_DIR": "data/floorplans",
}
for nome, valor in esperado.items():
    if ambiente.get(nome) != valor:
        erros.append(f"{nome} deveria ser fixado pelo compose em {valor!r}, veio {ambiente.get(nome)!r}")
if "host=postgres" not in (ambiente.get("DATABASE_URL") or ""):
    erros.append("DATABASE_URL deveria apontar para o serviço postgres do compose")

portas = config["services"]["frontend"].get("ports") or []
abertas = [p for p in portas if p.get("host_ip") not in ("127.0.0.1",)]
if abertas:
    erros.append(f"o painel deveria publicar só em 127.0.0.1 por padrão, veio {[p.get('host_ip') for p in portas]}")

if erros:
    print("\n".join(erros))
    sys.exit(1)
print(f"ok: {len(nomes)} variáveis do .env.example chegam ao backend, sobrescritas conferidas, painel em 127.0.0.1")
PY
