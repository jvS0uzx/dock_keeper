# Dependências de sistema

Extraídas dos comandos que o painel de fato executa. Nenhuma é opcional no
sentido de "funciona sem": sem `ss` o radar de portas fica vazio, sem
`/sys/class/hwmon` não há temperatura — e em nenhum dos dois casos aparece erro
na tela.

## No host monitorado por SSH

O painel envia um script pelo stdin de `bash -s`. O que ele usa:

| Dependência | Para quê | Sem ela |
|---|---|---|
| `bash` | O script é `bash -s`, não POSIX sh | A coleta não inicia |
| `/proc/stat` | CPU, por delta entre duas leituras | Sem métrica de CPU |
| `/proc/meminfo` | Memória usada e total | Sem métrica de memória |
| `/proc/loadavg` | Load de 1 minuto | Sem load |
| `/proc/uptime` | Uptime | Sem uptime |
| `df` (coreutils) | Disco da raiz, com `-B1` | Sem métrica de disco |
| `awk` | Recorta a saída de `df` e do `/proc` | A coleta quebra |
| `paste`, `tr` (coreutils) | Junta as linhas do `docker` em JSON | A lista de containers quebra |
| `/sys/class/hwmon/hwmon*` | Temperatura | Temperatura vem `null` — não zero. Ver [`metricas.md`](metricas.md) |
| CLI do Docker | `docker ps -a` e `docker stats --no-stream` | Sem containers |

Acesso a `docker` exige o usuário no grupo `docker` ou `root`.

### Ações e streams

| Recurso | Comando remoto | Dependência |
|---|---|---|
| Radar de portas | `ss -tulnp \| grep LISTEN` | `iproute2` e `grep` |
| Log de autenticação | `tail -n 20 -f <SSH_AUTH_LOG_PATH>` | `tail`, e o arquivo existir |
| Access log do Nginx | `tail -n 0 -F <SSH_NGINX_LOG_PATH>` | `tail`, e o arquivo existir |
| Ação em container | `docker <verbo> -- <nome>` | CLI do Docker |

`ss -tulnp` só lista o processo dono do socket quando roda como root. Sem
privilégio, a coluna de processo vem vazia e o radar perde metade da utilidade.

### Caminhos de log não são universais

`/var/log/auth.log` e `/var/log/nginx/access.log` são padrões de **Debian e
Ubuntu**. Em RHEL, CentOS e derivados o log de autenticação é `/var/log/secure`.

O caminho cravado deixava a tela de Segurança **vazia sem nenhum erro aparecer** —
o pior modo de falha possível, porque parece "nenhum evento" em vez de "não
consegui ler". Configure `SSH_AUTH_LOG_PATH` e `SSH_NGINX_LOG_PATH`. Ver
[`configuracao.md`](configuracao.md).

### Acesso SSH

`SSH_KEY_PATH` é **uma única chave** usada em todas as VPS, em geral `root`.

É dívida de segurança conhecida: comprometer o host do painel é comprometer a
frota inteira. O caminho correto seria um usuário dedicado de monitoramento com
`sudo` restrito aos comandos acima, e ele **não existe** hoje.

A verificação de host key é obrigatória — o painel recusa subir sem
`SSH_KNOWN_HOSTS`.

## Na máquina com o agente de push

Bem menos exigente, porque o agente mede pela biblioteca e não por shell:

| Dependência | Observação |
|---|---|
| Nenhuma biblioteca externa | Binário estático, `CGO_ENABLED=0` |
| Alcance de rede ao painel | Só saída; nada precisa estar aberto na máquina |
| `/etc/machine-id` | Preferido; há dois níveis de fallback |
| Permissão de escrita no diretório da credencial | `/etc/vd-agent/` no Linux, `%ProgramData%\vd-agent\` no Windows |
| systemd | Só para o instalador `install.sh`; o binário roda sozinho |

## No host do painel

| Dependência | Para quê | Sem ela |
|---|---|---|
| PostgreSQL 15+ | Tudo | O processo não sobe |
| Saída TCP para as portas SSH | Coleta | Sem métrica por SSH |
| Saída TCP 443 (ou `SSL_CHECK_PORT`) | Verificação de certificado | Todo domínio como falha de handshake |
| Certificados raiz do sistema | Verificar cadeia e falar com a API do Telegram | Cadeia não confiável em tudo, e sem alerta |
| Resolvedor de DNS reverso | Nome dos hosts descobertos | Inventário só com IP |
| Saída TCP para as faixas varridas | Sondagem de portas | Inventário sem classificação |
| `/proc/net/arp` | Endereço MAC dos hosts | Inventário sem MAC |

### ⚠️ `/proc/net/arp` em container

Dentro de um container, `/proc/net/arp` é a tabela ARP do **namespace de rede**,
não a do host. A varredura local enxerga quase nada.

Quem depende dela precisa de uma destas saídas:

- `network_mode: host` no serviço do backend — o que descarta a rede do compose,
  e o nome `postgres` deixa de resolver como host do banco;
- o coletor remoto `vd_collector` rodando **fora** do container.

O `docker-compose.yml` traz essa decisão comentada, e o padrão é **não** usar
`network_mode: host`: a maioria das instalações usa o coletor, e ligá-lo por
padrão publicaria o backend em todas as interfaces sem ninguém pedir.

A imagem do backend é `alpine`, e não `scratch`, exatamente por causa de duas
coisas desta lista: o resolvedor de DNS reverso e os certificados raiz. Em
`scratch` as duas falham em silêncio.

## Toolchain de desenvolvimento

| Ferramenta | Versão | Onde está declarada |
|---|---|---|
| Go | 1.26.5+ | `backend/go.mod` |
| Node.js | 22+ | `frontend/Dockerfile`, `.github/workflows/ci.yml` |
| PostgreSQL | 15+ | `docker-compose.yml` |

⚠️ A máquina onde esta documentação foi escrita roda **Node 20**, e o `npm` avisa
que não suporta essa versão. O build e os testes passaram mesmo assim, mas o CI
usa 22 — divergência a resolver antes de confiar em "passa aqui".
