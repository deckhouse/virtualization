
kubectl apply -f - <<EOF
apiVersion: deckhouse.io/v1
kind: PrometheusRemoteWrite
metadata:
  name: okmeter-sel-spb-2
spec:
  customAuthToken: $OKMETER_AUTH_TOKEN
  url: https://api.dop.flant.com/api/v1/push
  writeRelabelConfigs:
  - replacement: d8-virt-hetzner
    targetLabel: dop_ha_cluster
  - replacement: \$(POD_NAME)
    targetLabel: dop_ha_replica
EOF