# Configuração

Todas as variáveis abaixo foram extraídas do código, não do `.env.example`. O
painel lê **36 variáveis**; o agente lê outras **8**; o frontend, **4**.

O `.env` é carregado de `../.env` e depois `.env`, relativo ao diretório de
trabalho — por isso `go run ./cmd/vd_stats` de dentro de `backend/` acha o `.env`
da raiz do repositório.

## Obrigatórias

Estas três derrubam ou incapacitam o processo:

| Variável | Efeito se ausente |
|---|---|
| `DATABASE_URL` | `Falha crítica ao conectar no banco` e o processo termina |
| `API_TOKEN` | `API_TOKEN não definido` e o processo **recusa subir**. Fail-closed: sem token a API exporia SSH root de todas as VPS a quem alcançasse a porta |
| `SSH_KNOWN_HOSTS` | `Configuração SSH inválida` e o processo **recusa subir**, salvo com `SSH_INSECURE_HOST_KEY=true` |

## Banco

| Variável | Padrão | Efeito |
|---|---|---|
| `DATABASE_URL` | — | DSN do Postgres, formato `key=value` do GORM |
| `DB_MAX_OPEN_CONNS` | `20` | Teto de conexões simultâneas. Precisa caber no `max_connections` do servidor, contando todas as réplicas |
| `DB_MAX_IDLE_CONNS` | `5` | Conexões mantidas ociosas. Rebaixado ao teto de abertas se for maior, com aviso |
| `DB_CONN_MAX_LIFETIME` | `30m` | Vida útil da conexão, formato `time.ParseDuration` |

Valor inválido, zero ou negativo cai no padrão com log. Zero desligaria o teto,
que é exatamente o bug original.

## API

| Variável | Padrão | Efeito |
|---|---|---|
| `API_TOKEN` | — | Token de máquina. Obrigatório |
| `API_ADDR` | `:8080` | Endereço de escuta. Útil para subir uma segunda instância sem conflito |
| `ALLOWED_ORIGINS` | `http://localhost:5173` | Origens liberadas no CORS, separadas por vírgula |
| `TRUST_PROXY_HEADERS` | `false` | Autoriza ler `X-Real-IP` e `X-Forwarded-For` |

⚠️ `TRUST_PROXY_HEADERS=true` **só** com proxy reverso à frente. Sem ele o
cabeçalho vem do próprio cliente, e o limite de tentativa por IP vira enfeite.
Com ele, o nginx precisa de `proxy_set_header X-Forwarded-For
$proxy_add_x_forwarded_for;` — senão o parque inteiro conta num balde só e um
atacante insistente tranca a tela de login para todos os usuários.

## Limite de tentativa no login

| Variável | Padrão | Efeito |
|---|---|---|
| `LOGIN_RATE_WINDOW` | `15m` | Janela deslizante de contagem das falhas |
| `LOGIN_RATE_MAX_IP` | `30` | Falhas por endereço de origem na janela |
| `LOGIN_RATE_MAX_USER` | `8` | Falhas por nome de usuário na janela |

A rota de login é pública e cada tentativa gasta um bcrypt de custo 10 (60 a
100 ms de CPU), o que a torna também negação de serviço barata. Passado o teto, a
resposta é `429` **antes** de a senha ser conferida.

## SSH

| Variável | Padrão | Efeito |
|---|---|---|
| `SSH_KEY_PATH` | — | Chave privada usada em **todas** as VPS. `~` é expandido |
| `SSH_KNOWN_HOSTS` | — | Arquivo `known_hosts`. Obrigatório |
| `SSH_INSECURE_HOST_KEY` | `false` | Desliga a verificação de host key, com aviso alto no log |
| `SSH_AUTH_LOG_PATH` | `/var/log/auth.log` | Log de autenticação **no host remoto** |
| `SSH_NGINX_LOG_PATH` | `/var/log/nginx/access.log` | Access log do Nginx no host remoto |
| `SSH_COLLECT_INTERVAL` | `2` | Segundos entre amostras do script de coleta |

Os dois caminhos são de Debian e Ubuntu. Em RHEL o `auth.log` se chama
`/var/log/secure`, e o caminho cravado deixava a tela de Segurança **vazia sem
nenhum erro aparecer** — o pior modo de falha, porque parece "nenhum evento" em
vez de "não consegui ler".

Só caminho absoluto sem espaço nem metacaractere é aceito (`^/[A-Za-z0-9._/-]+$`).
O valor é interpolado num comando que roda como root na máquina remota; um
operador que cole um caminho com aspas por engano não pode transformar
configuração em execução de comando. Valor recusado cai no padrão com log.

## Verificação de SSL

| Variável | Padrão | Efeito |
|---|---|---|
| `SSL_CHECK_PORT` | `443` | Porta do handshake TLS. Fora de 1-65535 cai no padrão |
| `SSL_CHECK_TIMEOUT` | `5s` | Timeout do handshake, formato `time.ParseDuration` |
| `SSL_EXTRA_CA` | — | Bundle PEM com as CAs internas, somado às raízes do sistema |
| `SSL_FORBID_PRIVATE_TARGETS` | `false` | Recusa domínio que resolva para endereço privado, loopback ou link-local |

`SSL_EXTRA_CA` é a válvula que impede a verificação real de ser revertida na
primeira madrugada de alerta: a partir do momento em que o painel passou a
conferir cadeia e hostname de verdade, **todo serviço interno atrás de CA própria
aparece como `cadeia_nao_confiavel`** e dispara alerta. Arquivo ilegível ou sem
nenhum certificado válido é registrado no log e ignorado.

`SSL_FORBID_PRIVATE_TARGETS` fica desligada por padrão porque o painel é
auto-hospedado e monitorar serviço da rede interna é o uso normal da tela de
SSL. Ligue quando o painel ficar exposto a vários operadores: sem a guarda, o
cadastro de domínio funciona como sonda da rede onde o painel roda (SSRF). Um
alvo recusado volta com `invalid_reason` `alvo_privado_bloqueado`, sem abrir
conexão; quando o alvo passa, o handshake é feito no IP já conferido — não há
segunda resolução de DNS entre a checagem e a conexão.

## Alertas

| Variável | Padrão | Efeito |
|---|---|---|
| `TELEGRAM_BOT_TOKEN` | — | Sem ele os alertas só vão para o log |
| `TELEGRAM_CHAT_ID` | — | Idem |
| `ALERT_MIN_SEVERITY` | `warning` | Piso de notificação: `info`, `warning`, `high` ou `critical` |

## Ingestão e identidade de dispositivo

| Variável | Padrão | Efeito |
|---|---|---|
| `AGENT_INGEST_TOKEN` | — | Token compartilhado, **em descontinuação**. Sem ele e sem credencial apresentada, a ingestão fica desligada (fail-closed) |

O substituto é a credencial por dispositivo. Ver [`agente.md`](agente.md).

## Inventário de rede

| Variável | Padrão | Efeito |
|---|---|---|
| `DISCOVERY_CIDRS` | — | Faixas varridas, separadas por vírgula. Vazio desliga o recurso |
| `DISCOVERY_INTERVAL_MIN` | `15` | Minutos entre varreduras |
| `DISCOVERY_PORTS` | lista padrão | Portas sondadas. Vazio usa `22,80,135,139,443,445,515,631,3389,5000,8080,9100` |
| `DISCOVERY_SITE` | — | Código da unidade a que a varredura pertence |

Só faixa privada (RFC1918) é aceita, e prefixo maior que `/16` é recusado. Unidade
inexistente é apenas registrada no log; a varredura continua, sem classificar.

## Retenção

| Variável | Padrão | Efeito |
|---|---|---|
| `METRIC_RETENTION_DAYS` | `7` | Métrica bruta |
| `LOG_RETENTION_DAYS` | `7` | Linhas de log |
| `TREND_RETENTION_DAYS` | `400` | Tendência agregada por hora |
| `HOST_RETENTION_DAYS` | `30` | Host do inventário sem ser visto |
| `AUDIT_RETENTION_DAYS` | `365` | Log de auditoria |

Os prazos não são arbitrários e só fazem sentido juntos: métrica bruta é volumosa
e vira tendência; tendência é barata e serve à comparação ano a ano; inventário é
cadastro e some quando o equipamento some de verdade; auditoria é o que se
consulta **depois** do incidente, e o incidente costuma ser descoberto meses
depois.

Valor inválido, zero ou negativo cai no padrão com aviso. Zero significaria
"apagar tudo a cada passada", e um erro de digitação não pode ter esse efeito.

## Usuários e armazenamento

| Variável | Padrão | Efeito |
|---|---|---|
| `ADMIN_USER` | — | Nome do primeiro administrador |
| `ADMIN_PASSWORD` | — | Senha dele, mínimo 10 caracteres |
| `FLOORPLAN_DIR` | `data/floorplans` | Diretório das plantas baixas enviadas |

`ADMIN_USER`/`ADMIN_PASSWORD` só valem quando a tabela de usuários está vazia.
Ver [`autenticacao.md`](autenticacao.md).

## Tetos de corpo (não configuráveis)

Constantes de código, listadas porque a mensagem de erro `413` não diz o valor:

| Grupo | Teto |
|---|---|
| Formulário e JSON em geral | 128 KB |
| `POST /api/ingest/inventory` | 4 MB |
| Upload de planta baixa | `maxPlanBytes + 1 MB` |
| Hosts por envio de inventário | 5000 |

---

## Variáveis do agente

Definidas **na máquina monitorada**, não no painel. Ver [`agente.md`](agente.md).

| Variável | Padrão | Efeito |
|---|---|---|
| `AGENT_SERVER_URL` | — | URL do painel. Obrigatória |
| `AGENT_ENROLL_TOKEN` | — | Convite de uso único; trocado por credencial no primeiro boot |
| `AGENT_TOKEN` | — | Modo compartilhado, em descontinuação |
| `AGENT_CREDENTIAL_PATH` | `/etc/vd-agent/credential.json` | Onde a credencial é gravada (modo `0600`) |
| `AGENT_MACHINE_ID` | `/etc/machine-id` | Sobrescreve o identificador de máquina |
| `AGENT_HOSTNAME` | hostname do sistema | Sobrescreve o nome reportado |
| `AGENT_SITE` | — | Código da unidade. Ignorado quando há credencial: a unidade sai dela |
| `AGENT_INTERVAL` | `5` | Segundos entre envios |

Sem `AGENT_ENROLL_TOKEN`, sem credencial gravada e sem `AGENT_TOKEN`, o agente
**não sobe**: um agente que roda sem conseguir enviar é pior que um que não roda,
porque a máquina some do painel sem ninguém perceber.

## Variáveis do frontend

Todas de desenvolvimento. Ver [`../frontend/README.md`](../frontend/README.md).

| Variável | Efeito |
|---|---|
| `VITE_API_URL` | Base da API em desenvolvimento. Em produção, `public/config.json` |
| `VITE_API_TOKEN` | Token de máquina, **ignorado no build de produção** |
| `VITE_TARGET_VPS_IPS` | Endereços como o Nginx os reporta em `upstream_addr` |
| `VITE_LB_IP` | IP do host do balanceador, comparado com `servers.host_ip` |

---

## Divergências encontradas

O `.env.example` está **em sincronia perfeita com o código Go**: 36 variáveis
documentadas, 36 lidas, nenhuma sobrando dos dois lados. Isso foi verificado
comparando `os.Getenv` e os helpers (`envInt`, `envDuration`, `RetentionDays`,
`remotePathFromEnv`) contra as chaves do arquivo.

Cinco variáveis são lidas pelo `docker-compose.yml` e **não** pelo backend. Elas
estavam ausentes do `.env.example`, o que fazia `docker compose up` falhar na
cara de quem tinha copiado o exemplo — `POSTGRES_PASSWORD` é declarada no compose
com `:?`, ou seja, obrigatória. Foram acrescentadas:

| Variável | Padrão | Quem lê |
|---|---|---|
| `POSTGRES_PASSWORD` | — (obrigatória) | compose |
| `POSTGRES_USER` | `postgres` | compose |
| `POSTGRES_DB` | `vd_stats` | compose |
| `POSTGRES_PORT` | `5433` | compose |
| `PANEL_PORT` | `8081` | compose |

Havia também uma contradição de porta que mordia na primeira instalação: a
`DATABASE_URL` do `.env.example` apontava para 5432 enquanto o compose publica o
Postgres em **5433** no host, para não conflitar com um Postgres já instalado na
máquina. As duas foram alinhadas em 5433.
