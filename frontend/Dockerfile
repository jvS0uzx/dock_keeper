# syntax=docker/dockerfile:1

# ---- build ----
FROM node:22-alpine AS build

WORKDIR /src

# package.json e o lockfile primeiro, para a camada de dependência só invalidar
# quando eles mudam. npm ci e não npm install: respeita o lockfile em vez de
# resolver de novo, que é o que faz um build reproduzir o outro.
COPY package.json package-lock.json ./
RUN npm ci

COPY . .

# VITE_API_URL NÃO entra aqui como ARG de propósito. Embutir a URL no bundle
# obrigaria um build por unidade instalada; o painel a resolve em tempo de
# execução lendo /config.json, servido pelo nginx abaixo.
RUN npm run build

# ---- runtime ----
FROM nginx:1.27-alpine

COPY --from=build /src/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

# config.json é montado pelo compose. Este é o padrão para quem sobe a imagem
# sozinha: mesma origem, com o proxy do nginx repassando /api.
RUN printf '{"apiUrl":"/api"}\n' > /usr/share/nginx/html/config.json

EXPOSE 80
