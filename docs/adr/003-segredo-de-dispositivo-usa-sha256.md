# 003 — Segredo de dispositivo usa SHA-256, não bcrypt

## Contexto

A credencial de dispositivo (`DeviceCredential.SecretHash`) e o convite de
enrollment (`EnrollmentToken.TokenHash`) precisam ser guardados de forma que um
vazamento da tabela não entregue a credencial.

O reflexo é usar o mesmo bcrypt de custo 10 que `User.PasswordHash` usa. Seria
errado, e vale entender por quê.

## Decisão

SHA-256 hexadecimal, sem sal e sem alongamento de chave.

## Consequência

**O que bcrypt compra.** Bcrypt existe para encarecer ataque de dicionário contra
segredo **escolhido por humano**, que tem entropia baixa e é reutilizado entre
serviços. O custo por verificação é a defesa.

**Por que não se aplica aqui.** O segredo tem 32 bytes de `crypto/rand` — 256
bits de entropia. Não existe dicionário, não existe reutilização, e força bruta
sobre esse espaço não é viável independentemente da função de hash. Bcrypt não
compraria nada.

**O que ele custaria.** A ingestão roda a cada poucos segundos por dispositivo.
Bcrypt de custo 10 são 60 a 100 ms de CPU por verificação; com 500 dispositivos
reportando a cada 5 segundos, seriam 6 a 10 segundos de CPU por segundo de
relógio. Seria negação de serviço embutida no caminho quente.

**A mesma lógica vale para a sessão.** O token de sessão também tem 256 bits de
`crypto/rand`, e a tabela `user_sessions` também guarda SHA-256. Ver
[ADR 004](004-sessao-guarda-hash-e-resolve-concessoes.md).

**O que continua com bcrypt.** `User.PasswordHash` — porque ali o segredo é
escolhido por gente.

**Cuidado ao mudar.** Se algum dia o segredo de dispositivo passar a ser
escolhido por humano, esta decisão se inverte. A justificativa depende
inteiramente da entropia da origem.

## Nota lateral

`DeviceID` existe separado do segredo para a busca ser por chave primária. Sem
ele, verificar uma credencial exigiria varrer a tabela comparando hash linha a
linha, a cada push.
