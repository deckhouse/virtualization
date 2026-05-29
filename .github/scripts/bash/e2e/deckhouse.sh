#!/usr/bin/env bash

show_deckhouse_state() {
  echo "[DEBUG] Show deckhouse pods"
  kubectl -n d8-system get pods -l app=deckhouse -o wide || true
  echo "[DEBUG] Show queue (first 25 lines)"
  d8 s queue list | head -n25 || true
}

wait_for_deckhouse_queue() {
  local count=60
  local delay=10
  local queue_count

  for i in $(seq 1 "$count"); do
    queue_count="$(d8 s queue list | grep -Po '([0-9]+)(?= active)' || true)"
    echo "[INFO] Wait until Deckhouse queue is empty ${i}/${count}, active=${queue_count:-unknown}"

    if [ "$queue_count" = "0" ]; then
      echo "[SUCCESS] Deckhouse queue is empty"
      return 0
    fi

    if (( i % 5 == 0 )); then
      show_deckhouse_state
    fi

    if [ "$i" -lt "$count" ]; then
      sleep "$delay"
    fi
  done

  echo "[ERROR] Deckhouse queue is not empty"
  show_deckhouse_state
  return 1
}
