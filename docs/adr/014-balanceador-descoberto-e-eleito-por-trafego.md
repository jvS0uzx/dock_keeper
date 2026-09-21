# ADR 014 — O balanceador é descoberto por sonda e eleito pelo tráfego

Data: 2026-09-20
Estado: aceito

## Contexto

A malha do painel nunca desenhou tráfego em instalação nenhuma, e a causa não era
o desenho: era um interruptor inalcançável.

O parser do log do Nginx só subia para o servidor cujo `collect_nginx` fosse
verdadeiro (`ssh/manager.go`). A coluna nasce `false`, o `createServer` do
frontend enviava apenas `{name, host_ip, user}`, e o `ServerPatchRequest` não
aceitava o campo. Ou seja: o backend sabia coletar, e não existia caminho, por
tela nenhuma, para pedir que coletasse. Sem linhas em `metric_load_balancers`, a
malha ficava sem arestas e sem animação.

Havia um segundo problema, de modelo. A flag descreve o mundo como "existe um
balanceador, e alguém sabe qual é". O parque real tem uma VPS servindo e outras
capazes de assumir. Um booleano marcado à mão fica errado exatamente no momento
em que a informação mais importa — durante um failover, quando o papel muda e
ninguém vai editar cadastro no meio do incidente.

Por fim, a malha só conhecia destino que o tráfego já tinha revelado. Numa
madrugada sem requisição, a topologia sumia da tela como se não existisse.

## Decisão

**A descoberta substitui a marcação.** Uma sonda roda por SSH em toda VPS, no
cadastro e a cada `NGINX_PROBE_INTERVAL` (padrão 15 min), e responde três coisas:
o Nginx está ativo, a configuração tem blocos `upstream`, e o log de acesso é
legível pelo usuário do SSH. Disso sai `nginx_estado`: `ausente`, `inativo`,
`sem_upstream` ou `candidato`.

**O que a sonda não conseguiu ler não vira fato negativo.** `nginx -T` que falha
por falta de sudo produz `desconhecido`, nunca `sem_upstream` — configuração
ilegível não prova ausência de upstream. Pela mesma razão, sonda cega não apaga a
topologia já conhecida: a reconciliação de `nginx_upstreams` só roda quando a
configuração foi de fato lida. É a regra do projeto, "ausente é nulo, nunca zero",
aplicada à topologia.

**Todo candidato coleta.** O log passa a ser seguido em todos os candidatos, não
em um eleito. É o que permite ver o tráfego migrar. O portão é reavaliado a cada
30 s em vez de decidido no boot, então instalar Nginx numa reserva passa a ser
reconhecido sem reiniciar o processo — e, enquanto o portão está fechado, nenhuma
sessão SSH é aberta.

**O principal é eleito pelo tráfego, por unidade.** Entre os candidatos de um
mesmo `site_id`, principal é quem recebeu mais requisição na janela `LB_WINDOW`;
os demais ficam `reserva`. O agrupamento por unidade evita que duas unidades com
balanceadores próprios disputem um posto só.

**A eleição tem histerese, e ela também protege contra a sonda.** O desafiante
precisa superar o incumbente por `MALHA_MARGEM_TROCA_PCT` (padrão 25%) durante
`MALHA_CICLOS_TROCA` ciclos seguidos (padrão 2). O contador vale para qualquer
mudança de papel, inclusive a perda de candidatura: sem isso, uma sonda que
tropeça por um ciclo demoveria o principal e promoveria uma reserva que só tem
tráfego de health check. Empate não elege. Tráfego zero em todos preserva o
principal — madrugada silenciosa não é failover.

**A malha nasce da configuração e é animada pelo tráfego.** As arestas vêm de
`nginx_upstreams`, então existem antes da primeira requisição; o tráfego anima o
que já está desenhado. Aresta que sai de reserva é potencial e nunca anima.

**`collect_nginx` sobrevive como sobreposição manual**, agora acessível pelo PATCH
e exposta no cartão do servidor: coleta mesmo que a descoberta não classifique
como candidato.

## Consequências

- Cadastrar a VPS passa a bastar. O papel é descoberto, e a troca de principal é
  reconhecida sozinha, que era o pedido do dono.
- Descoberta que falha vira texto na tela — "Nginx ativo, log sem permissão de
  leitura para o usuário deploy" — em vez de malha vazia. O silêncio era o defeito
  de fundo; a flag inalcançável foi só onde ele apareceu.
- A troca de principal vira alerta com alvo estruturado (`servico`), viabilizado
  pelo [ADR 013](013-entrega-por-canal-e-alvo-estruturado.md). A chave inclui os
  dois lados da transição, senão a sequência A→B→A seria tratada como incidente
  vivo e renotificada em vez de abrir incidente novo.
- A primeira eleição, de "ninguém" para um principal, sai como `info` e é
  descartada pelo `ALERT_MIN_SEVERITY` padrão. Instalação nova não é incidente.
- O contador de desafios vive em memória. Reinício do processo zera, e o custo é
  esperar mais alguns ciclos até a próxima troca. Persistir exigiria migração.
- **O painel reconhece o failover; não o executa.** Nginx de pé numa reserva não
  atrai tráfego: quem decide o destino é o endereço. Mover o endereço exige
  keepalived, IP flutuante do provedor ou DNS, e nenhum deles é decisão que um
  observador único deva tomar — se o painel perde a rede para o principal, ele o
  vê morto enquanto ele serve, e promover a reserva aí produz dois principais.
  Acionar a troca, quando existir, é ação de runbook auditada, não decisão
  autônoma.
