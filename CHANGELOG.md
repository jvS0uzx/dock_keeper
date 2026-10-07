# Changelog

Mudanças relevantes do DockKeeper, no formato
[Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/).

As versões seguem o [Versionamento Semântico](https://semver.org/lang/pt-BR/).
A 1.0.0 é a primeira versão numerada, a mesma no painel, no agente de estação e
no [coletor](https://github.com/jvS0uzx/dockkeeper_collector). As seções datadas
abaixo dela são o histórico de entregas na `main` antes da numeração.

## [Não lançado]

### Corrigido

- `container_down` de container removido do host fecha sozinho, com
  `[INFO] Container <nome> foi removido de <host>`, na primeira rodada do
  `docker ps -a` que lista outros containers e não lista ele. Antes, o alerta de
  container que deixava de existir (os temporários `<12 hex>_<nome>` do Compose,
  por exemplo) ficava `open` para sempre. Rodada com a lista vazia não fecha nada,
  porque `docker ps` que falha chega igual a um host sem containers. O estado de
  reinícios do container removido também é descartado.
- Uma chave de alerta não fica mais com duas linhas abertas. Quando o incidente
  passa de `ALERT_RESUME_HOURS` sem sinal de vida e volta, a linha nova é criada
  e a antiga vira `resolved` na mesma transação, com `resolved_at` no seu
  `last_seen_at` e sem mensagem de recuperação. Na subida, o despachante fecha
  da mesma forma as duplicatas já gravadas e deixa aberta só a linha mais
  recente de cada chave.

## [1.0.0] - 2026-10-04

Primeira versão numerada. Reúne o que foi entregue desde 2026-09-10.

### Adicionado

- Versão 1.0.0 injetada por `-ldflags` no painel (`internal/versao`) e no agente
  (`main.Version`); sem ela o binário se declara `dev`. O compose repassa
  `DOCKKEEPER_VERSAO` para as imagens, a versão sai no log de subida e no campo
  `versao` do `/readyz` e do `/api/readyz`, só para requisição autenticada.
- Contrato de ingestão versionado: campo `schema` (valor 1) em
  `/api/ingest/metrics`, `/api/ingest/inventory` e
  `/api/ingest/network-metrics`. Nas duas primeiras, ausente ou 0 continua
  aceito; outro valor recebe 400 `schema N não suportado; este painel aceita 1`.
- Telemetria SNMP de interfaces (D1): `POST /api/ingest/network-metrics`,
  migração 020, `GET /api/network/hosts/{id}/interfaces`,
  `GET /api/network/interfaces/{id}/serie`, abas Resumo e Interfaces no painel
  lateral do host de rede, `NETWORK_METRIC_RETENTION` e ADR 015.
- Sonda de bancos com interruptor por servidor (`collect_bancos`, ligado por
  padrão, migração 018) e inventário de MySQL e MariaDB no host e em container
  (`probe_mysql.sh`, migração 019, `MYSQL_PROBE_INTERVAL`, `SSH_MYSQL_CMD`).
- Tela Bancos com faixa de resumo, três filtros, tabela de cinco colunas e painel
  lateral com Identificação, Configuração e Bases.
- Alertas com fila persistida, entrega por canal (Telegram, webhook e ntfy), alvo
  estruturado, ciclo de vida com renotificação e estados `sem_canal` e
  `dispensado`; tela de Alertas com reconhecer e resolver.
- Sinal de fila de alertas atrasada: aviso no log acima de 50 pendentes ou com o
  mais antigo esperando 2 min, os contadores `dockkeeper_alertas_na_fila` e
  `dockkeeper_alertas_fila_atraso_seg`, e `fila_pendente` e `fila_atraso_seg` em
  `GET /api/alerts/summary`.
- Poda das anotações de painel com `ANNOTATION_RETENTION_DAYS` (padrão 365; 0
  desliga).
- Aviso no log quando o access log do Nginx não está no formato exigido:
  `[Nginx] <host>: X linhas recebidas, Y reconhecidas; ...`.
- Descoberta do Nginx por sonda (`probe_nginx.sh`), eleição do balanceador
  principal por tráfego e `collect_nginx` como sobreposição manual (ADR 014).
- Migrações SQL versionadas e embutidas no binário, com hash conferido e
  `pg_advisory_lock` (ADR 012).
- Log em JSON, contadores em `/metrics`, `/readyz` declarando degradação e
  `/api/readyz` para a interface.
- Tela de Dispositivos: emitir convite de uso único, listar credenciais e revogar.
- Taxa de rede por host (`net_rx_bps` e `net_tx_bps`, em bytes por segundo) no
  script de coleta, no agente, na tendência horária, no histórico e em
  `/api/metrics/live`. Interfaces `lo`, `veth`, `docker`, `br-` e `virbr` ficam
  fora da soma.
- Motor de regras aceita `temperature`, `net_rx` e `net_tx`. Amostra sem medição
  não dispara nem conta como zero.
- Alertas sem regra: força bruta no `auth.log` por IP de origem, upstream do nginx
  com proporção alta de 5xx, e stream do nginx caído.
- `GET /readyz`, que consulta o banco. O `/healthz` continua só liveness.
- Reconexão SSH com backoff exponencial e variação aleatória, com teto em
  `SSH_RECONNECT_MAX`.
- `SSH_USE_SUDO`, `SSH_KEY_PASSPHRASE` e `SSH_USE_AGENT` para rodar a coleta com
  usuário sem root, chave protegida ou `ssh-agent`.
- Limite de sessões SSH sob demanda por servidor (`SSH_MAX_SESSIONS_PER_HOST`).
- `ALERT_COOLDOWN`, `SESSION_TTL` e `HOST_OFFLINE_AFTER` configuráveis.
- `POST /api/users` aceita `"active"`.
- Testes que barram comentário em código no backend e no frontend.
- `CONTRIBUTING.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md` e este arquivo.
- ADR 009, sobre o token compartilhado de ingestão.
- Teste opcional da sonda de MySQL e MariaDB contra containers reais
  (`TESTE_MYSQL_REAL=1`).
- Dependabot semanal para Go, npm, GitHub Actions e Docker.

### Alterado

- **Incompatível:** identificadores de runtime renomeados de `vd` para
  `dockkeeper`: serviços `dockkeeper-agent` e `dockkeeper-collector`, arquivos em
  `/etc/dockkeeper-*.env` e `/var/lib/dockkeeper-*`, usuário
  `dockkeeper-monitor` e banco padrão `dockkeeper`. Estação já instalada precisa
  ser reinstalada.
- **Incompatível:** o `X-Agent-Token` compartilhado só é aceito com
  `ALLOW_LEGACY_INGEST_TOKEN=true`. Desligado, recebe 401 com a instrução de usar
  o convite, e a recusa fica na auditoria com o IP de origem.
- **Incompatível:** o `API_TOKEN` é somente leitura sem
  `API_TOKEN_ALLOW_WRITE=true`.
- **Incompatível:** `DB_AUTOMIGRATE` foi removido; ligado, é ignorado com aviso.
  O primeiro boot aplica migrações que apagam linha órfã: faça `pg_dump` antes.
- A poda de retenção não tem mais teto por ciclo (ED-05): repete lotes de 5000
  linhas, com pausa de 100 ms entre eles, até esvaziar o que venceu, e é
  cancelada no desligamento.
- A poda tira também a interface SNMP que deixou de vir no envio, depois de
  `NETWORK_METRIC_RETENTION` sem ser vista e sem leitura restante.
- A telemetria SNMP aceita lote parcial: dispositivo com IP inválido é descartado
  e contado em `rejeitados`, e `in_bps`/`out_bps` negativos viram `NULL`. O 400
  ficou para JSON inválido e `schema` diferente de 1.
- `SSH_KEEPALIVE_MAX_MISSES` e `RTT_PROBE_INTERVAL` passam a valer também nos
  streams de logs de container, do `auth.log` (SSE e vigia) e do Nginx.
- A aresta da malha segue a mesma convenção no painel e na tela do Nginx:
  tracejado `4 4` para parada, `5 5` para potencial e contínua animada para
  ativa.
- O `go.mod` aponta o módulo do coletor para um commit que existe no GitHub
  (`098b88d`); o anterior tinha sido apagado por reescrita de histórico.
- `pgx` 5.11, com o espaço do `statement_timeout` no DSN codificado como `%20`.
- Imagens sobre `alpine:3.24` e `nginx:1.30-alpine`; `engines.node` `>=22` no
  frontend.
- CI em `ubuntu-24.04`, com `actions/checkout`, `setup-go` e `setup-node` em
  versões que rodam em Node 24, fixadas por SHA, e `-timeout 25m` na suíte Go.
- Testes do backend falham sem banco quando `TEST_EXIGE_BANCO=1` (ligado no CI),
  e cada pacote cria o próprio banco descartável a partir do `DATABASE_URL`.
- Linhas de log vão para o banco em lote, e o stream ao vivo não espera mais o
  banco.
- Agente de estação encerra limpo no SIGTERM e distingue credencial recusada de
  falha de rede.

### Corrigido

- Painéis laterais e modais de Inventário de Rede, Bancos e Containers paravam
  na altura do conteúdo da tela: a animação de entrada deixava `transform`
  aplicado no contêiner e prendia os elementos `fixed` a ele.
- Envio concorrente do agente sem `machine_id` duplicava o servidor (ED-15): a
  busca e a criação rodam numa transação com `pg_advisory_xact_lock` pelo nome
  do host.
- A primeira amostra de métricas por SSH sai cerca de 1 s depois de conectar, com
  a CPU calculada sobre uma janela de 1 s, em vez de esperar um intervalo inteiro
  ou medir microssegundos.
- O log de subida mostra a URL de escuta a partir do host de `API_ADDR`
  (`0.0.0.0` e `[::]` aparecem como `localhost`), em vez de
  `http://localhost127.0.0.1:18080`.
- O baseline das migrações só troca de hash no banco adotado do `AutoMigrate`;
  qualquer outra edição da `001` recusa o boot.
- Sonda de MySQL e MariaDB corrigida contra bancos reais: host só com o cliente
  `mariadb`, conexões por base só com o privilégio `PROCESS`, container sem `sh`
  ou sem cliente com o motivo vazio, container pausado ou reiniciando como
  `inativo`, senha do root lida de `*_ROOT_PASSWORD_FILE`, e `@` preservado no
  motivo.
- Regra de alerta criada desligada e usuário criado inativo eram gravados como
  ligado e ativo.
- O tipo da credencial de dispositivo não era conferido: agente enviava
  inventário, e coletor enviava métrica. Agora responde 403, e o `/api/enroll`
  com tipo divergente responde 409 sem consumir o convite.
- Status de container com aspas quebrava a linha JSON da coleta.
- O parser do access log do nginx confundia o tamanho da resposta com o status
  HTTP.

### Segurança

- `GET /api/metrics/history` com `container_id` não conferia se o container era
  do servidor autorizado: quem tinha acesso a uma unidade lia a série de um
  container de outra sabendo o UUID. Agora o container precisa ser do servidor
  conferido, e `container_id` inválido responde 404 em vez de 500.
- `undici` 7.30.0 (dependência de desenvolvimento, via `jsdom`): `npm audit`
  limpo.
- `clientIP` só honra `X-Forwarded-For` vindo de `TRUSTED_PROXY_CIDRS`, e a
  ingestão tem teto de recusas anônimas por IP.

## 2026-09-10

### Alterado

- Module path Go passa a ser `github.com/jvS0uzx/dock_keeper`, e o comando
  principal fica em `backend/cmd/dockkeeper`.
- README com números, diagrama de arquitetura e selo do CI.

### Segurança

- Endereços de infraestrutura real trocados por faixas de documentação
  (RFC 5737).
- `gitleaks` no CI, varrendo a árvore e o histórico de todas as branches.

## 2026-08-25

### Alterado

- Licença MIT. O `NOTICE` da licença anterior foi removido.

## 2026-08-23

### Adicionado

- Guarda opcional contra SSRF nas rotas de SSL (`SSL_FORBID_PRIVATE_TARGETS`).
- Exemplo de sudoers e guia de usuário de monitoramento com privilégio mínimo.
- CI no GitHub Actions com Postgres de serviço.
- Redesign do frontend sobre um sistema de design próprio, com malha de
  roteamento adaptativa e velocímetros.

### Corrigido

- Teto de 5000 hosts do inventário passa a ser conferido durante a leitura do
  corpo, sem carregar o envio inteiro na memória.
- Chave duplicada no radar de portas com dois processos na mesma porta.

## 2026-08-22

### Adicionado

- Identidade por dispositivo: convite de uso único, credencial própria por agente
  e coletor, unidade tirada da credencial.
- Log de auditoria das ações, com tela própria.
- Travas de cadastro no inventário de rede e `DISCOVERY_SITE`.
- Temperatura dos hosts coletados por SSH.
- Janela de online derivada do intervalo informado pelo agente.

### Alterado

- Métricas que a fonte não mede passam a ser `NULL`, não zero.
- Sessões de login persistidas no banco.

### Segurança

- Token de administrador fora do bundle de produção.
- Ticket de SSE amarrado à sessão de quem o pediu.
- Administrador de unidade deixa de ser administrador global.
- Recorte por unidade em todas as rotas de leitura.
- Limite de tentativas no login e teto de tamanho de corpo.
- Verificação de cadeia e hostname no certificado SSL.
- `SSH_KNOWN_HOSTS` obrigatório.

## 2026-08-21

### Adicionado

- Ações em containers: start, stop e restart.
- Verificação periódica de certificados SSL.
- Histórico de métricas com agregação no banco.
- Motor de regras de alerta.
- Agente de estação para Linux e Windows.
- Busca de logs.
- Alertas no Telegram.
- CPU, memória e load do host na coleta por SSH.

### Segurança

- Injeção de comando no stream de logs de container.

## 2026-08-20

### Adicionado

- Cadastro de servidores com conexão SSH iniciada e encerrada sem reiniciar o
  backend.
- Logs de container ao vivo por SSE.
- Log de autenticação ao vivo e radar de portas abertas.
- Verificação de certificado SSL por handshake TLS.

### Corrigido

- Vazamento de goroutine nas sessões SSH.
- Servidor duplicado pelo mesmo IP.
