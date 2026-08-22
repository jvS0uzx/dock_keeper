#!/bin/bash
# O Go injeta variáveis em base64 no topo deste script em tempo real.
# Coleta métricas do HOST (cpu/ram/load/disco/uptime) e dos containers Docker.

# Amostra inicial de CPU para calcular o delta no primeiro loop.
read -r _ u1 n1 s1 i1 rest < /proc/stat
prev_total=$((u1+n1+s1+i1)); prev_idle=$i1

while true; do
  # --- CPU do host: delta entre duas leituras de /proc/stat ---
  read -r _ u n s i rest < /proc/stat
  total=$((u+n+s+i)); idle=$i
  dt=$((total-prev_total)); di=$((idle-prev_idle))
  if [ "$dt" -gt 0 ]; then
    HOST_CPU=$(awk "BEGIN{printf \"%.1f\", (1-$di/$dt)*100}")
  else
    HOST_CPU=0
  fi
  prev_total=$total; prev_idle=$idle

  # --- RAM do host (bytes): usado = total - disponível ---
  MEM_TOTAL_KB=$(awk '/^MemTotal:/{print $2}' /proc/meminfo)
  MEM_AVAIL_KB=$(awk '/^MemAvailable:/{print $2}' /proc/meminfo)
  MEM_TOTAL=$((MEM_TOTAL_KB*1024))
  MEM_USED=$(((MEM_TOTAL_KB-MEM_AVAIL_KB)*1024))

  LOAD1=$(awk '{print $1}' /proc/loadavg)
  UPTIME=$(awk '{print $1}' /proc/uptime)
  DISK_ROOT=$(df -B1 / | awk 'NR==2 {print $3","$2}')

  # Containers: ps (estado) + stats (consumo), unidos no Go pelo docker_id.
  DOCKER_PS=$(docker ps -a --format '{"docker_id":"{{.ID}}","name":"{{.Names}}","project":"{{.Label "com.docker.compose.project"}}","state":"{{.State}}","status":"{{.Status}}"}' | tr -d '\r' | paste -sd, -)
  DOCKER_STATS=$(docker stats --no-stream --format '{"docker_id":"{{.ID}}","cpu_percent":"{{.CPUPerc}}","mem_usage":"{{.MemUsage}}"}' | tr -d '\r' | paste -sd, -)

  echo "{\"uptime\":$UPTIME,\"host_cpu\":$HOST_CPU,\"mem_used\":$MEM_USED,\"mem_total\":$MEM_TOTAL,\"load1\":$LOAD1,\"disk_root\":\"$DISK_ROOT\",\"ps\":[$DOCKER_PS],\"stats\":[$DOCKER_STATS]}"
  sleep 2
done
