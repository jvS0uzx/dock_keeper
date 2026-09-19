# ADR 012 — Esquema por migração versionada, não por AutoMigrate

Data: 2026-09-18
Estado: aceito

## Contexto

O esquema saía do `AutoMigrate` do GORM no boot, mais dois blocos de DDL cru
(índice de `machine_id` e unicidade do inventário). Funciona para criar tabela
nova, e falha em tudo que é mudança:

- **Sem versão e sem registro.** Nada dizia qual esquema uma instalação tinha.
- **Mudança silenciosa.** O `AutoMigrate` aplica o que a tag da struct disser.
  Foi assim que o R1 aconteceu: uma tag `default:true` virou default de coluna no
  banco, e regra criada desligada passou a nascer ligada.
- **Sem volta.** Voltar o binário não desfaz um `ALTER`.
- **Sem lugar para o que o GORM não faz.** FK, CHECK, extensão e limpeza de
  órfão não cabem numa tag.

## Decisão

O esquema passa a ser aplicado por migrações SQL numeradas, embutidas no binário
(`internal/database/migracoes/NNN_nome.sql`), com um migrador próprio de ~200
linhas e nenhuma dependência nova.

- `schema_migrations` guarda versão, nome, hash do arquivo e quando aplicou.
- Cada migração roda na própria transação, em ordem crescente.
- `pg_advisory_lock` serializa instâncias concorrentes: a segunda espera e depois
  encontra tudo aplicado.
- **Hash conferido:** editar um arquivo já aplicado derruba o boot com mensagem
  dizendo para criar uma migração nova. Drift entre instalações vira erro, não
  surpresa.
- **A `001_baseline` é convergente e roda sempre**, inclusive em banco que já a
  registrou. Ela usa `CREATE TABLE IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS` para
  cada coluna, índice com `IF NOT EXISTS` e constraint dentro de bloco `DO` que
  consulta `pg_constraint`. Nada de tabela existente é recriado, nenhum tipo é
  alterado e nenhuma linha é tocada: ela só cria o que falta.
- **Regra de hash do baseline:** como a 001 é convergente, o hash dela é
  **atualizado** quando muda, em vez de derrubar o boot. Da 002 em diante a regra
  continua estrita: arquivo aplicado que muda recusa subir.
- `AutoMigrate` continua disponível atrás de `DB_AUTOMIGRATE=true`, para quem
  quiser o caminho antigo num ambiente descartável. O padrão é migração.

## Por que convergente, e não "adota e segue"

A primeira versão marcava a 001 como aplicada quando o banco já tinha tabelas, sem
executar nada. Isso presume que um banco antigo é igual ao snapshot, e não é: o do
dono nasceu em agosto com 21 tabelas, e as quatro criadas depois (`alerts`,
`dashboards`, `dashboard_panels`, `annotations`) nunca foram criadas. A 002 então
quebrava no boot ao criar FK de tabela inexistente, e o painel não subia. Uma coluna
acrescentada a uma tabela antiga pela convergência nasce **nulável**, enquanto num
banco novo ela vem do `CREATE TABLE` com a restrição original; é a diferença aceita
em troca de nunca reescrever tabela com dado.

## Consequências

- Mudança de esquema passa a ser um arquivo revisável em PR, com nome e ordem.
- Dá para exigir o que o GORM não expressa: as FKs do ED-13, os CHECK de enum do
  ED-12 e o índice de trigrama do ED-07 entraram como migrações 002, 003 e 004.
- O custo é disciplina: mudar uma struct não muda mais o banco sozinho. Quem
  esquecer a migração vê o erro no teste, porque a suíte roda sobre o esquema
  migrado.
- A tabela `migracao_limpeza` guarda quantas linhas órfãs cada migração apagou, e
  o migrador ecoa isso no log — a limpeza fica auditável depois do fato.
- Ainda não há rollback automático. Voltar uma migração é escrever a inversa, e
  isso é proposital: `DROP` automático em produção é pior que o problema.

## Alternativas descartadas

- **goose, golang-migrate ou atlas.** Resolvem o mesmo e trazem dependência e CLI
  extra para um projeto que tem uma tabela de controle e quatro arquivos.
- **Manter o AutoMigrate e adicionar SQL solto no boot.** É o que existia, e foi
  exatamente o que produziu o R1 e o esquema sem versão.
