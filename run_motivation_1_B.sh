#!/usr/bin/bash

EXPS=("5" "10" "15" "20" "25" "30" "35" "40" "45" "50" "55" "60" "65" "70" "75" "80" "85" "90" "95" "100" "105" "110" "115")

server_exec() {
    ssh -oStrictHostKeyChecking=no -p 22 "$1" "$2";
}

LOADER="node-001.startup-exp.simbricks-pg0.wisc.cloudlab.us"
WORKER="node-002.startup-exp.simbricks-pg0.wisc.cloudlab.us"


echo "Cleaning up previous results"
rm -rf ~/results
for exp in "${EXPS[@]}"; do
    mkdir -p ~/results/$exp
done
server_exec $WORKER "tmux kill-session -t kubelet"
server_exec $WORKER "tmux kill-session -t util"
server_exec $WORKER "tmux kill-session -t ctr"

for exp in "${EXPS[@]}"; do
    echo "Starting $exp"

    cp ~/loader/data/traces/experiments/B/$exp/invocations.csv ~/loader/data/traces/example/invocations.csv
    cp ~/loader/data/traces/experiments/B/$exp/memory.csv ~/loader/data/traces/example/memory.csv
    cp ~/loader/data/traces/experiments/B/$exp/durations.csv ~/loader/data/traces/example/durations.csv
    
    server_exec $WORKER "tmux new-session -d -s 'kubelet' '~/loader/scripts/recorder/kubelet.sh'"
    server_exec $WORKER "tmux new-session -d -s 'util' '~/loader/scripts/recorder/worker-utilization.sh'"
    server_exec $WORKER "tmux new-session -d -s 'ctr' '~/loader/scripts/recorder/containerd.sh'"

    go run cmd/loader.go --config=cmd/config_knative_trace.json

    sleep 60
    
    ./scripts/recorder/post_metrics.sh > ~/results/$exp/stats.txt

    server_exec $WORKER "tmux kill-session -t kubelet"
    server_exec $WORKER "tmux kill-session -t util"
    server_exec $WORKER "tmux kill-session -t ctr"
    server_exec $WORKER "python3 ./loader/scripts/parser/parse_kubelet_logs.py"
    
    cp /users/mghgm/loader/data/out/experiment_cluster_usage_480.csv ~/results/$exp
    cp /users/mghgm/loader/data/out/experiment_deployment_scale_480.csv ~/results/$exp
    cp /users/mghgm/loader/data/out/experiment_kn_stats_480.csv ~/results/$exp
    cp /users/mghgm/loader/data/out/experiment_duration_480.csv ~/results/$exp
    scp $WORKER:/users/mghgm/pods.csv ~/results/$exp
    scp $WORKER:/users/mghgm/containerd_util.txt ~/results/$exp
    scp $WORKER:/users/mghgm/kubelet_util.txt ~/results/$exp

    echo "Experiment $exp ended"
done
