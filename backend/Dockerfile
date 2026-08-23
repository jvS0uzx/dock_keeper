# syntax=docker/dockerfile:1

# ---- build ----
# A versão do Go acompanha o go.mod. Subir aqui sem subir lá compila com um
# toolchain que o módulo não declara, e o erro só aparece na próxima diretiva
# nova de linguagem.
FROM golang:1.26-alpine AS build

WORKDIR /src

# go.mod e go.sum primeiro: a camada de dependência só invalida quando eles
# mudam, e não a cada edição de código.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 gera binário estático, que é o que permite a imagem final ser
# uma base sem libc. -s -w tira símbolos e DWARF.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o /out/dockkeeper ./cmd/vd_stats

# ---- runtime ----
# alpine e não scratch: o painel precisa de resolvedor de DNS reverso para o
# inventário de rede, e de certificados raiz para o verificador de SSL e para a
# API do Telegram. Em scratch os dois falham em silêncio.
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 dockkeeper

# O painel não precisa de root: ele fala SSH como root no host REMOTO, usando a
# chave montada, e isso não exige privilégio nenhum no container.
USER dockkeeper
WORKDIR /app

COPY --from=build /out/dockkeeper /app/dockkeeper

# Plantas baixas enviadas pelo painel. Monte um volume aqui, senão o upload
# desaparece no primeiro deploy.
VOLUME ["/app/data"]

EXPOSE 8080

ENTRYPOINT ["/app/dockkeeper"]
