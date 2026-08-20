#!/bin/bash
# O Go injeta as variáveis em base64 no topo deste script em tempo real.
# Este script coleta métricas gerais e Docker de forma contínua.

while true; do
  DOCKER_JSON=$(docker stats --no-stream --format '{"docker_id":"{{.ID}}","name":"{{.Name}}","cpu_percent":"{{.CPUPerc}}","mem_usage":"{{.MemUsage}}"}' | paste -sd, -)
  UPTIME=$(cat /proc/uptime | awk '{print $1}')
  DISK_ROOT=$(df -B1 / | awk 'NR==2 {print $3","$2}')
  
  # Imprime um JSON no stdout (O Golang lê esse fluxo)
  echo "{\"uptime\":$UPTIME,\"disk_root\":\"$DISK_ROOT\",\"containers\":[$DOCKER_JSON]}"
  sleep 2
done
