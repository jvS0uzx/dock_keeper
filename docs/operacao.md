# Operação

## Retenção e poda

Cinco prazos, todos configuráveis. Ver [`configuracao.md`](configuracao.md).

| Dado | Padrão | Racional |
|---|---|---|
| Métrica bruta | 7 dias | Inserção a cada poucos segundos por host. Depois de uma semana o que se olha é a tendência |
| Log | 7 dias | Idem |
| Tendência horária | 400 dias | Permite comparar um mês com o mesmo mês do ano anterior |
| Host do inventário | 30 dias | Cadastro, não série. Some quando o equipamento some |
| Auditoria | 365 dias | Consultada **depois** do incidente, que costuma ser descoberto meses depois |

A poda roda de hora em hora, **em lotes com pausa**. Um `DELETE` sem limite numa
tabela grande bloqueia e produz bloat; os lotes têm teto por passada, e o que
sobra fica para o ciclo seguinte, registrado no log.

### A ordem entre rollup e poda importa

`StartTrendWorker` devolve um canal que a poda espera antes da primeira passada.
Se o painel ficou fora do ar mais tempo que a janela de retenção, podar antes de
agregar apagaria dado bruto que o rollup nunca consolidou — e o histórico daquele
período sumiria para sempre.

Por isso também a **primeira** passada do rollup é completa, sem a janela de 3
horas que as demais usam.

## Auditoria

Toda escrita e todo comando remoto deixam rastro em `audit_logs`.

### O que é registrado

| Coluna | Observação |
|---|---|
| `actor_user_id`, `actor_username`, `actor_role` | Nome e papel são **copiados**, não referenciados |
| `source_ip`, `user_agent` | Origem |
| `action` | `recurso.verbo`: `container.stop`, `user.create`, `site.delete` |
| `target_type`, `target_id`, `target_label` | Rótulo copiado pelo mesmo motivo do nome |
| `site_id` | Permite recortar a auditoria por unidade |
| `result` | `ok`, `denied`, `error` ou `pending` |
| `detail` | JSON montado por allowlist, **nunca** o corpo da requisição |

O nome do ator é copiado porque usuário apagado não pode levar embora o próprio
rastro, e porque o papel muda com o tempo — o que importa é qual era **no momento
da ação**.

### O que não é registrado, e por quê

**Sucesso de ingestão.** São milhares de push por minuto num parque de algumas
centenas de hosts. Gravar cada um destruiria a tabela e afogaria o sinal. Só a
**recusa** vira linha — token inválido é exatamente o que a auditoria existe para
capturar.

**Requisições `GET`.** Leitura não é escrita. As duas exceções são a abertura dos
streams de `auth.log` e `docker logs`: leitura de dado sensível, como root, por
decisão de uma pessoa. Uma linha por abertura, nunca por evento — registrar o
conteúdo transmitido faria da tabela uma segunda cópia do log.

**O corpo da requisição.** Uma auditoria que copiasse o corpo de `POST
/api/auth/login` viraria um depósito de senha em claro na mesma tabela que o
administrador consulta. Há teste que falha se a senha aparecer em qualquer campo
da linha gravada.

### Consultar

`GET /api/audit`, admin global, paginado, filtrável por ator, ação, resultado,
unidade e intervalo. Há tela no painel.

O que procurar primeiro num incidente:

| Ação | Significa |
|---|---|
| `result = denied` | Alguém tentou alcançar unidade alheia, ou credencial recusada |
| `ingest.site_mismatch` | Dispositivo com credencial válida declarando outra unidade. **Sinal de comprometimento** |
| `device.enroll` com `denied` | Convite inexistente, expirado ou já usado |
| `auth-log.open`, `container-logs.open` | Quem leu log sensível de qual host |
| `container.*` com `pending` que nunca fechou | Comando que travou a máquina e não retornou |

## Alertas

O motor avalia as regras a cada 30 segundos sobre a última métrica de cada host.

**Duração mínima.** `for_duration_sec` exige que a condição se mantenha por N
segundos sem interrupção. Zero mantém o comportamento antigo — uma amostra acima
do limite já alerta —, que transforma qualquer pico de compilação ou backup em
incidente e ensina o operador a ignorar o canal. Uma amostra dentro do limite
**zera** a contagem.

**Cooldown persistente.** 30 minutos, guardados em `alert_states`. Antes viviam
num mapa em memória, zerado a cada reinício: um deploy fazia o painel recomeçar
notificando tudo.

**Notificação de recuperação.** Quando a condição deixa de ser violada e o alerta
havia sido anunciado, sai um aviso de normalização — uma vez, não a cada ciclo.

Alerta abaixo de `ALERT_MIN_SEVERITY` fica só no log e **nunca gera
recuperação**: não se anuncia o fim de um problema que nunca foi comunicado.

### Telegram

Sem `parse_mode`, de propósito. Nome de container com `_` — `nginx_proxy`, o caso
comum do Docker Compose — fazia o Telegram responder `400`, e o alerta virava só
uma linha de log. O alerta que mais importa é justamente o que nunca chega.

As severidades saem como prefixo textual: `[CRITICO]`, `[ALERTA]`, `[AVISO]`,
`[INFO]`.

## Rotação de segredo

Quatro segredos, e a ordem importa:

1. **Senha do Postgres primeiro**, com `ALTER USER`. Se o `.env` for atualizado
   antes, o painel perde o banco até o comando rodar.
2. `API_TOKEN` e `AGENT_INGEST_TOKEN`: `openssl rand -hex 32`.
3. `TELEGRAM_BOT_TOKEN`: só no BotFather, com `/revoke` e depois `/token`.
4. Atualizar `.env` e `frontend/.env`, e conferir `chmod 600` nos dois.

Reiniciar o painel **não derruba mais os logins** — as sessões vivem em
`user_sessions`. O reinício serve só para recarregar o ambiente.

`SSH_KEY_PATH` não faz parte dessa rotação: ela é a chave root única de toda a
frota, e o que ela pede é substituição por usuário dedicado com `sudo` restrito,
não troca de valor.

## O que olhar quando quebra

### O painel não sobe

| Mensagem | Causa |
|---|---|
| `API_TOKEN não definido` | Falta o token. Fail-closed de propósito |
| `Configuração SSH inválida` | Falta `SSH_KNOWN_HOSTS`. Ver [`../backend/deploy/README.md`](../backend/deploy/README.md) |
| `Falha crítica ao conectar no banco` | `DATABASE_URL` errada, ou banco fora |
| `sorry, too many clients` | O teto do pool não cabe no `max_connections`. Ver `DB_MAX_OPEN_CONNS` |

### Uma tela está vazia sem erro

Esta é a classe de falha mais traiçoeira do sistema, e quase sempre é uma destas:

| Tela | Causa provável |
|---|---|
| Segurança (auth.log) | Caminho de log errado para a distribuição. Ver `SSH_AUTH_LOG_PATH` |
| Descoberta de SSL | Nenhum host com `collect_nginx` ligado — não há access log para observar |
| Inventário de rede | `DISCOVERY_CIDRS` vazio, ou painel em container sem `network_mode: host` |
| Inventário, numa unidade só | A varredura local se desligou porque a unidade tem coletor registrado. O log diz |

### Métrica parada, host "offline" reportando

Confira `report_interval_sec` do host. A janela de liveness é três vezes o
intervalo, com piso de 30 s — ver [`metricas.md`](metricas.md). Agente antigo que
não informa o intervalo fica no piso, e um agente com `AGENT_INTERVAL=60` contra
piso fixo apareceria sempre offline.

### Tela de tempo real parada atrás de proxy

`proxy_buffering off` faltando no nginx. Ele segura os eventos do SSE e a tela
fica parada **sem erro nenhum aparecer**. Ver
[`../backend/deploy/README.md`](../backend/deploy/README.md).

### Todo domínio ficou vermelho de uma vez

Esperado, se a instalação usa CA interna. A verificação de SSL passou a conferir
cadeia e hostname de verdade — antes só olhava a data de validade. Popule
`SSL_EXTRA_CA` com o bundle das CAs internas.

Certificado autoassinado continua vermelho, e corretamente: é a tela existir para
detectar isso.

## Comportamento com o banco indisponível

Assimétrico de propósito, e vale conhecer antes do incidente:

| Componente | Comportamento | Por quê |
|---|---|---|
| Sessão | Falha **fechada**: recusa autenticar | Não conceder acesso sem poder verificar |
| Auditoria | Falha **aberta**: registra o erro no log e segue | O painel é a ferramenta de quem apaga incêndio |
| Cooldown de alerta | Falha **aberta**: notifica | Alerta duplicado incomoda, alerta perdido mata |
| Estado de duração de alerta | Segura o disparo de regra com duração; **nenhuma** recuperação é anunciada | Nunca tranquilizar sem evidência |

O detalhe de cada decisão está nos [ADRs](adr/).
