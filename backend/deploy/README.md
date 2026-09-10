# Deploy do painel

Este diretório reúne o que a instalação precisa e não cabe no código: host keys,
o instalador do agente e as decisões de configuração que só fazem sentido com o
ambiente na frente.

## `known_hosts.exemplo`

O painel **se recusa a subir sem `SSH_KNOWN_HOSTS`**. Antes ele caía em silêncio
para "aceita qualquer host key", registrando só um aviso — e um painel que roda
comando como `root` por SSH nessa condição está aberto a máquina no meio.

O `known_hosts.exemplo` deste diretório traz **apenas o formato** — nenhuma
chave nele é real. Gere o seu com `ssh-keyscan`, que abre a conexão e lê a chave
pública apresentada sem autenticar nem alterar nada no servidor:

```bash
ssh-keyscan -T 8 -H 203.0.113.25 203.0.113.39 203.0.113.38 > known_hosts
```

Os endereços acima são da faixa reservada pela RFC 5737 para documentação:
troque pelos seus. Um comando desses cobre Node 1, Node 2 e Load Balancer. A VM
de estado não tem IP público e só é alcançável pela malha privada — se o painel
passar a monitorá-la, acrescente um `ssh-keyscan` do endereço dela a partir de
uma máquina dentro da malha.

A flag `-H` grava o nome do host em hash, de modo que o arquivo resultante não
revela quais máquinas você monitora — vale manter mesmo em uso local.

Aponte `SSH_KNOWN_HOSTS` para o arquivo gerado e confira com `ssh-keygen -l
-f`. **Reveja as impressões digitais antes de confiar nelas**: um `ssh-keyscan`
executado numa rede já comprometida grava a chave do atacante com a mesma
naturalidade com que gravaria a legítima. A conferência fora de banda — pelo
console do provedor, ou por uma sessão SSH que você já sabe boa — é o que
transforma o arquivo em garantia em vez de ritual.

Para desligar a verificação de propósito, em laboratório:

```
SSH_INSECURE_HOST_KEY=true
```

Sai aviso alto no log a cada conexão. Não use em produção.

## Atrás de proxy reverso

O painel deriva o IP de origem do `RemoteAddr` por padrão. Atrás de um proxy,
`RemoteAddr` é o proxy, e o limite de tentativa de login passaria a contar o
parque inteiro num balde só — um atacante insistente trancaria a tela de login
para todos os usuários.

A saída é `TRUST_PROXY_HEADERS=true`, que autoriza a leitura de `X-Real-IP` e
`X-Forwarded-For`. Ela **só** pode ser ligada quando existe de fato um proxy à
frente: sem ele, o cabeçalho vem do próprio cliente e o limite por IP vira
enfeite.

Ligando, o `server` correspondente no nginx precisa de:

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host              $host;
    proxy_set_header X-Real-IP         $remote_addr;
    proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;

    # O painel usa SSE. Sem estas três linhas o nginx bufferiza e as telas de
    # tempo real ficam paradas, sem erro nenhum aparecer.
    proxy_buffering off;
    proxy_cache off;
    proxy_read_timeout 3600s;
}
```

Estado em 22/08/2026: o painel **não** está atrás do Load Balancer da infra —
escuta em `localhost:8080` na máquina de desenvolvimento. `TRUST_PROXY_HEADERS`
fica `false` até isso mudar.

## `agent/`

Instalador do agente de push para Linux, Windows e Ansible. Veja
[`agent/README.md`](agent/README.md).
