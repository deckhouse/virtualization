#!/usr/bin/env bash


# Global values
sleep_time=5


cd ..

exit_trap() {
  echo "Cleanup"
  echo "Exiting..."
  exit 0
}

format_duration() {
  local total_seconds=$1
  local hours=$((total_seconds / 3600))
  local minutes=$(( (total_seconds % 3600) / 60 ))
  local seconds=$((total_seconds % 60))
  printf "%02d:%02d:%02d\n" "$hours" "$minutes" "$seconds"
}

formatted_date() {
  date -d "@$1" +"%H:%M:%S %d-%m-%Y"
}

trap exit_trap SIGINT SIGTERM

undeploy_vm() {
  task destroy:all
  
  up=false
  
  while [[ $up == false ]]; do
    VDTotal=$(kubectl -n perf get vd -o name | wc -l)
    VMTotal=$(kubectl -n perf get vm -o name | wc -l)
    
    if [ $VDTotal -eq 0 ] && [ $VMTotal -eq 0 ]; then
      up=true
      echo "All vms and vds are destroyed"
      echo "$(formatted_date $(date +%s))"
      echo ""
      break
    fi
    
    echo ""
    echo "Waiting for vms and vds to be destroyed..."
    echo "VM to destroy: $VMTotal"
    echo "VD to destroy: $VDTotal"
    echo ""
    echo "Waiting for $sleep_time seconds..."
    sleep $sleep_time
    echo ""
  done
}

cleanup_dir() {
  local dir=$1
  rm -rf $dir
}

default_storage_class() {
  local STORAGE_CLASS=${1:-$STORAGE_CLASS}
  if [ -n $STORAGE_CLASS ]; then
    echo $STORAGE_CLASS
  else
    kubectl get storageclass -o json \
        | jq -r '.items[] | select(.metadata.annotations."storageclass.kubernetes.io/is-default-class" == "true") | .metadata.name'
  fi
}


deploy_resources() {
  # available resource types: all, vm, vd
  local resourceType=${1:-"all"}
  local VIType=$2
  local COUNT=${3:-10}
  local report_dir=${4:-"report/statistics"}

  if [ -z $VIType ]; then
    echo "VIType is requared"
    echo "Supported types: virtualImage, persistentVolumeClaim"
    exit 1
  fi

  echo "Create vd from ${VIType}"
  echo ""

  case $resourceType in
    "all")
      task apply:all \
        COUNT=${COUNT} \
        STORAGE_CLASS=$(default_storage_class) \
        VIRTUALDISK_TYPE=virtualDisk \
        VIRTUALIMAGE_TYPE=virtualImage
      ;;
    "vm")
      task apply:vms \
        COUNT=${COUNT} \
        NAMESPACE=perf \
        NAME_PREFIX=performance \
        RESOURCES_PREFIX=performance
      ;;
    "vd")
      task apply:disks \
        COUNT=${COUNT} \
        NAMESPACE=perf \
        NAME_PREFIX=performance \
        RESOURCES_PREFIX=performance
      ;;
    *)
      echo "Unknown resource type: $resourceType"
      exit 1
      ;;
  esac
  
  dir_virtualDisk="${report_dir}/vm_vi_${VIType}"
  mkdir -p ${dir_virtualDisk}
  
  up=false
  start_time=$(date +%s)

  echo "Test vm with virtualDisk from ${VIType}" > ${dir_virtualDisk}/report_vm_virtualDisk.txt
  echo "Start time: $(formatted_date $start_time)" >> ${dir_virtualDisk}/report_vm_virtualDisk.txt

  while [[ $up == false ]]; do
    VDReady=$(kubectl -n perf get vd | grep "Ready" | wc -l)
    VDTotal=$(kubectl -n perf get vd -o name | wc -l)

    VMReady=$(kubectl -n perf get vm | grep "Running" | wc -l)
    VMTotal=$(kubectl -n perf get vm -o name | wc -l)
    
    if [ $VDReady -eq $VDTotal ] && [ $VMReady -eq $VMTotal ]; then
      up=true
      echo "All vms and vds are ready"
      end_time=$(date +%s)
      break
    fi
    
    echo ""
    echo "Waiting for vms and vds to be ready..."
    echo "VM Running: $VMReady/$VMTotal"
    echo "VD Ready: $VDReady/$VDTotal"
    echo ""
    echo "Waiting for $sleep_time seconds..."
    sleep $sleep_time
    echo ""
  done

  duration=$((end_time - start_time))
  formatted_duration=$(format_duration "$duration")
  echo "Duration: $duration seconds"
  echo "Execution time: $formatted_duration"

  echo "Execution time: $formatted_duration" >> ${dir_virtualDisk}/report_vm_virtualDisk.txt
  echo "End time: $(formatted_date $end_time)" >> ${dir_virtualDisk}/report_vm_virtualDisk.txt

  echo ""

}

gather_all_statistics() {
  local report_dir=${1:-"report/statistics"}
  task statistic:get-stat:all

  mv tools/statistic/*.csv ${report_dir}
}

gather_vm_statistics() {
  local report_dir=${1:-"report/statistics"}
  task statistic:get-stat:vm

  mv tools/statistic/*.csv ${report_dir}
}

gather_vd_statistics() {
  local report_dir=${1:-"report/statistics"}
  task statistic:get-stat:vd

  mv tools/statistic/*.csv ${report_dir}
}

start_migration() {
  # supoprt duration format: 0m - infinite, 30s - 30 seconds, 1h - 1 hour, 2h30m - 2 hours and 30 minutes
  local duration=${1:-"5m"}
  local SESSION="test-perf"
  
  echo "Create tmux session: $SESSION"
  tmux -2 new-session -d -s "${SESSION}"

  tmux new-window -t "$SESSION:1" -n "perf"
  tmux split-window -h -t 0      # Pane 0 (left), Pane 1 (right)
  tmux split-window -v -t 1      # Pane 1 (top), Pane 2 (bottom)

  tmux select-pane -t 0
  tmux send-keys "k9s" C-m
  tmux resize-pane -t 1 -x 50%
  
  echo "Start migration in $SESSION, pane 1"
  tmux select-pane -t 1
  tmux send-keys "NS=perf task evicter:run:migration -d ${duration}" C-m
  tmux resize-pane -t 1 -x 50%

  tmux select-pane -t 2
  tmux resize-pane -t 2 -x 50%
  echo "For watching migration in $SESSION, connect to session with command:"
  echo "tmux -2 attach -t ${SESSION}"
  echo ""

}

stop_migration() {
  local SESSION="test-perf"
  tmux -2 kill-session -t "${SESSION}" || true
}

restart_virtualization_components() {
  kubectl -n d8-virtualization rollout restart deployment virtualization-controller
}

collect_vpa() {
  local vpa_dir="report/vpa"
  mkdir -p ${vpa_dir}
  local VPAS=( $(kubectl -n d8-virtualization get vpa -o name) )
  
  for vpa in $VPAS; do
    vpa_name=$(echo $vpa | cut -d "/" -f2)
    file="vpa_${vpa_name}.yaml"
    kubectl -n d8-virtualization get $vpa -o yaml > "${vpa_dir}/${file}_$(formatted_date $(date +%s))"
  done
}

# test only
for vitype in "virtualImage" "persistentVolumeClaim"; do
  cleanup_dir "report"
done

# all resources
vi_type="virtualImage"
migration_duration="5m"
undeploy_vm
sleep 5
deploy_resources "all" $vi_type "30"
sleep 1
start_migration "$migration_duration"
sleep $((5 * 60))
stop_migration
sleep 5
gather_all_statistics "report/statistics/vm_vi_${vi_type}"
collect_vpa


vi_type="persistentVolumeClaim"
undeploy_vm
sleep 5
deploy_resources "all" $vi_type "30"
sleep 1
start_migration "$migration_duration"
sleep $((5 * 60))
stop_migration
sleep 5
gather_all_statistics "report/statistics/vm_vi_${vi_type}"
collect_vpa

# vd only

