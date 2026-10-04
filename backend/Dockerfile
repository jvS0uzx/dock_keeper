# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSAO=dev

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w -X github.com/jvS0uzx/dock_keeper/internal/versao.Versao=${VERSAO}" -o /out/dockkeeper ./cmd/dockkeeper

FROM alpine:3.24

ARG VERSAO=dev
LABEL org.opencontainers.image.version="${VERSAO}"

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 dockkeeper

WORKDIR /app

COPY --from=build /out/dockkeeper /app/dockkeeper

RUN mkdir -p /app/data && chown dockkeeper:dockkeeper /app/data

VOLUME ["/app/data"]

USER dockkeeper

EXPOSE 8080

ENTRYPOINT ["/app/dockkeeper"]
