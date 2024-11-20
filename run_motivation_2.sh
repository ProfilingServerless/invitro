#!/usr/bin/bash

EXPS=("1_5" "1_10" "1_15" "1_20" "1_25" "1_30" "1_35" "1_40" "1_45" "1_50")

server_exec() {
    ssh -oStrictHostKeyChecking=no -p 22 "$1" "$2";
}

LOADER="node-001.dataplane-exp.simbricks-pg0.wisc.cloudlab.us"
WORKER="node-002.dataplane-exp.simbricks-pg0.wisc.cloudlab.us"

echo "Cleaning up previous results"
rm -rf ~/results
for exp in "${EXPS[@]}"; do
    mkdir -p ~/results/$exp
done

echo "Starting experiments"
for exp in "${EXPS[@]}"; do
    echo "Starting $exp"

    server_exec $WORKER "tmux kill-session -t kubelet"
    server_exec $WORKER "tmux kill-session -t ctr"

    cp ~/traces/$exp ~/loader/data/traces/example/invocations.csv

    server_exec $WORKER "tmux new-session -d -s 'kubelet' 'sleep 200; ~/loader/scripts/recorder/kubelet.sh'"
    server_exec $WORKER "tmux new-session -d -s 'ctr' 'sleep 200; ~/loader/scripts/recorder/containerd.sh'"

    go run cmd/loader.go --config=cmd/config_knative_trace.json

    server_exec $WORKER "tmux kill-session -t kubelet"
    server_exec $WORKER "tmux kill-session -t ctr"
    server_exec $WORKER "python3 ./loader/scripts/parser/parse_kubelet_logs.py"
    
    cp /users/mghgm/loader/data/out/experiment_cluster_usage_480.csv ~/results/$exp
    cp /users/mghgm/loader/data/out/experiment_deployment_scale_480.csv ~/results/$exp
    cp /users/mghgm/loader/data/out/experiment_kn_stats_480.csv ~/results/$exp
    cp /users/mghgm/loader/data/out/experiment_duration_480.csv ~/results/$exp
    scp $WORKER:/users/mghgm/pods.csv ~/results/$exp

    sleep 60
    echo "Experiment $exp ended"
done
