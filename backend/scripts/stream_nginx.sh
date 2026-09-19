#!/bin/bash

${DOCKKEEPER_TAIL:-tail} -n 0 -F "${DOCKKEEPER_NGINX_LOG:-/var/log/nginx/access.log}"
