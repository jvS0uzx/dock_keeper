# syntax=docker/dockerfile:1

FROM node:22-alpine AS build

WORKDIR /src

COPY package.json package-lock.json ./
RUN npm ci

COPY . .

RUN npm run build

FROM nginx:1.31-alpine

ARG VERSAO=dev
LABEL org.opencontainers.image.version="${VERSAO}"

COPY --from=build /src/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY seguranca.conf /etc/nginx/seguranca.conf

RUN printf '{"apiUrl":"/api"}\n' > /usr/share/nginx/html/config.json

EXPOSE 80
