# 001 — Falha de auditoria não derruba a requisição

## Contexto

Toda escrita do painel grava uma linha em `audit_logs`. O painel comanda SSH como
root nas VPS, então o rastro é requisito de adoção, não conforto.

A pergunta é o que fazer quando a gravação falha — banco indisponível, tabela
cheia, conexão derrubada no meio.

A leitura estrita de auditoria é **fail-closed**: sem poder registrar, não
executar. É a postura de sistema financeiro e de conformidade, e tem argumento
forte.

## Decisão

`audit.Record` registra a falha no log e devolve `0`. A requisição prossegue.

## Consequência

**A favor.** O painel é a ferramenta de quem está apagando incêndio. Recusar
parar um container porque o Postgres soluçou tira a ferramenta na hora em que ela
mais importa — e o incidente que motivou o acesso continua acontecendo.

**Contra.** Existe uma janela em que uma ação acontece sem rastro. Para um sistema
que executa comando como root remotamente, isso é exatamente o que a auditoria
deveria impedir.

**Mitigação.** A falha precisa gritar: `[Auditoria] AVISO: ação %q de %q não foi
registrada`. Auditoria que falha em silêncio é pior que auditoria nenhuma, porque
a ausência da linha passa a significar duas coisas diferentes — "não aconteceu" e
"não conseguimos gravar" — e quem consulta não distingue.

**Revisável.** Para o comando SSH especificamente, fail-closed é defensável: são
poucas ações por dia, e a linha `pending` gravada antes da execução já é o ponto
onde a recusa caberia. A decisão atual vale para toda a superfície por
uniformidade, não porque as duas metades sejam igualmente sensíveis.
