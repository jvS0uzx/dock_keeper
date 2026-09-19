# 010 — Dashboards relacionais, com dono e recorte na leitura

## Contexto

O painel ganhou dashboards montados pelo usuário: uma lista de gráficos, cada um
com servidor, métrica e janela. O modelo precisava responder três perguntas: onde
guardar os painéis, de quem é o dashboard e o que acontece quando o usuário perde
acesso a um servidor que já estava num dashboard dele.

## Decisão

- **Painéis em tabela própria** (`dashboard_panels`), com chave estrangeira para
  `dashboards` em `ON DELETE CASCADE`, em vez de uma coluna JSON no dashboard.
- **Dono obrigatório.** `owner_user_id` não aceita nulo, o nome é único por dono,
  e a rota de dashboards recusa sessão de máquina. Não há dashboard compartilhado
  nem global. As anotações seguem a mesma regra só na escrita, que exige autor; a
  leitura aceita a sessão de máquina, com concessão global.
- **Recorte na leitura.** O servidor de cada painel é conferido contra o alcance do
  usuário na criação (404 quando não alcança) e de novo a cada `GET`, que omite o
  painel que saiu do alcance.

## Consequência

**A favor.**

- O banco valida o que o JSON deixaria passar: tipo do `server_id`, tamanho do
  título, painel órfão.
- Achar os painéis de um servidor é um `WHERE`, sem varrer documentos.
- Apagar o dashboard limpa os painéis sem código extra.
- Com dono obrigatório, "quem vê este dashboard" tem uma resposta só, e o recorte
  por unidade não depende de alguém lembrar de filtrar na escrita.

**Contra.**

- Salvar é substituir: o `PUT` apaga e recria todos os painéis numa transação, então
  o `id` de painel não é estável entre edições. O frontend não deve guardar esse id.
- Não existe compartilhamento. Dois operadores que querem a mesma visão montam cada
  um a sua.
- O painel que sai do alcance some da resposta sem aviso. Se o acesso volta, ele
  reaparece: o registro continua no banco.

**Limites.** 12 painéis por dashboard e 20 dashboards por usuário, conferidos na
API. Ao apagar um usuário, os dashboards dele vão junto. As anotações ficam, porque
são registro de evento, não preferência pessoal.
