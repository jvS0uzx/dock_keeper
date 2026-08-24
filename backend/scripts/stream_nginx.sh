#!/bin/bash
# Nenhuma variável declarada aqui. O Go injeta no topo.

tail -n 0 -F "${VD_NGINX_LOG:-/var/log/nginx/access.log}"
