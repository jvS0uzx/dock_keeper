#!/bin/bash

PROC_STAT="${DOCKKEEPER_PROC_STAT:-/proc/stat}"
read -r _ u1 n1 s1 i1 rest < "$PROC_STAT"
prev_total=$((u1+n1+s1+i1)); prev_idle=$i1

NET_DEV="${DOCKKEEPER_NET_DEV:-/proc/net/dev}"
net_sample() {
  awk 'NR > 2 {
    sub(/^ +/, "")
    n = split($0, f, /[: ]+/)
    if (f[1] !~ /^(lo|veth|docker|br-|virbr)/ && n >= 10) { rx += f[2]; tx += f[10]; ok = 1 }
  } END { if (ok) printf "%.0f %.0f", rx, tx }' "$NET_DEV" 2>/dev/null
}
prev_net=""; prev_net_ns=""

enderecos_json() {
  ip -o -4 addr show 2>/dev/null | awk '
    $2 !~ /^(lo|veth|docker|br-|virbr)/ {
      split($4, a, "/")
      if (a[1] != "" && a[1] !~ /^127\./ && a[1] !~ /^169\.254\./) {
        if (vistos[a[1]]++ == 0) {
          if (n++) printf ","
          printf "\"%s\"", a[1]
        }
      }
    }'
}

while true; do
  read -r _ u n s i rest < "$PROC_STAT"
  total=$((u+n+s+i)); idle=$i
  dt=$((total-prev_total)); di=$((idle-prev_idle))
  prev_total=$total; prev_idle=$idle
  if [ "$dt" -le 0 ]; then
    sleep "${DOCKKEEPER_INTERVAL:-2}"
    continue
  fi
  HOST_CPU=$(awk "BEGIN{printf \"%.1f\", (1-$di/$dt)*100}")

  NET_JSON=""
  cur_net=$(net_sample)
  cur_net_ns=$(date +%s%N)
  case "$cur_net_ns" in *[!0-9]*) cur_net_ns="" ;; esac
  if [ -n "$prev_net" ] && [ -n "$cur_net" ] && [ -n "$prev_net_ns" ] && [ -n "$cur_net_ns" ]; then
    NET_JSON=$(awk -v p="$prev_net" -v c="$cur_net" -v t0="$prev_net_ns" -v t1="$cur_net_ns" 'BEGIN {
      split(p, a, " "); split(c, b, " ")
      dt = (t1 - t0) / 1e9
      if (dt > 0 && b[1] >= a[1] && b[2] >= a[2])
        printf ",\"net_rx_bps\":%.1f,\"net_tx_bps\":%.1f", (b[1] - a[1]) / dt, (b[2] - a[2]) / dt
    }')
  fi
  prev_net=$cur_net; prev_net_ns=$cur_net_ns

  MEM_TOTAL_KB=$(awk '/^MemTotal:/{print $2}' /proc/meminfo)
  MEM_AVAIL_KB=$(awk '/^MemAvailable:/{print $2}' /proc/meminfo)
  MEM_TOTAL=$((MEM_TOTAL_KB*1024))
  MEM_USED=$(((MEM_TOTAL_KB-MEM_AVAIL_KB)*1024))

  TEMP_RAW=$(cat /sys/class/hwmon/hwmon*/temp*_input 2>/dev/null \
    | awk '$1 > 0 && $1 < 150000' | sort -n | tail -1)
  if [ -n "$TEMP_RAW" ]; then
    TEMP_JSON=",\"temperature_c\":$(awk "BEGIN{printf \"%.1f\", $TEMP_RAW/1000}")"
  else
    TEMP_JSON=""
  fi

  LOAD1=$(awk '{print $1}' /proc/loadavg)
  UPTIME=$(awk '{print $1}' /proc/uptime)
  DISK_ROOT=$(df -B1 / | awk 'NR==2 {print $3","$2}')

  DOCKER_PS=$(docker ps -a --format '{"docker_id":{{json .ID}},"name":{{json .Names}},"project":{{json (.Label "com.docker.compose.project")}},"state":{{json .State}},"status":{{json .Status}}}' | tr -d '\r' | paste -sd, -)
  DOCKER_STATS=$(docker stats --no-stream --format '{"docker_id":{{json .ID}},"cpu_percent":{{json .CPUPerc}},"mem_usage":{{json .MemUsage}}}' | tr -d '\r' | paste -sd, -)

  ADDR_JSON=",\"addresses\":[$(enderecos_json)]"

  echo "{\"uptime\":$UPTIME,\"host_cpu\":$HOST_CPU,\"mem_used\":$MEM_USED,\"mem_total\":$MEM_TOTAL,\"load1\":$LOAD1,\"disk_root\":\"$DISK_ROOT\"$TEMP_JSON$NET_JSON$ADDR_JSON,\"ps\":[$DOCKER_PS],\"stats\":[$DOCKER_STATS]}"
  sleep "${DOCKKEEPER_INTERVAL:-2}"
done
