# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o /out/dockkeeper ./cmd/dockkeeper

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 dockkeeper

WORKDIR /app

COPY --from=build /out/dockkeeper /app/dockkeeper

RUN mkdir -p /app/data && chown dockkeeper:dockkeeper /app/data

VOLUME ["/app/data"]

USER dockkeeper

EXPOSE 8080

ENTRYPOINT ["/app/dockkeeper"]
