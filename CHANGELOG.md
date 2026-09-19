# Changelog

Mudanças relevantes do DockKeeper, no formato
[Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/).

O projeto ainda não publica versões numeradas. Cada seção abaixo é uma data de
entrega na `main`, da mais recente para a mais antiga.

## [Não lançado]

### Adicionado

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

### Alterado

- Identificadores de runtime renomeados de `vd` para `dockkeeper`: serviços
  `dockkeeper-agent` e `dockkeeper-collector`, arquivos em
  `/etc/dockkeeper-*.env` e `/var/lib/dockkeeper-*`, usuário
  `dockkeeper-monitor` e banco padrão `dockkeeper`. Estação já instalada precisa
  ser reinstalada.
- O `X-Agent-Token` compartilhado só é aceito com
  `ALLOW_LEGACY_INGEST_TOKEN=true`. Desligado, recebe 401 com a instrução de usar
  o convite, e a recusa fica na auditoria com o IP de origem.
- Linhas de log vão para o banco em lote, e o stream ao vivo não espera mais o
  banco.
- Agente de estação encerra limpo no SIGTERM e distingue credencial recusada de
  falha de rede.

### Corrigido

- Regra de alerta criada desligada e usuário criado inativo eram gravados como
  ligado e ativo.
- O tipo da credencial de dispositivo não era conferido: agente enviava
  inventário, e coletor enviava métrica. Agora responde 403, e o `/api/enroll`
  com tipo divergente responde 409 sem consumir o convite.
- Status de container com aspas quebrava a linha JSON da coleta.
- O parser do access log do nginx confundia o tamanho da resposta com o status
  HTTP.

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
