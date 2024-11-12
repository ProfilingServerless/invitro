#!/usr/bin/bash

EXPS=("5_10" "5_15" "5_20" "5_25" "5_30" "5_35" "5_40" "5_45" "5_50" "5_55" "5_60" "5_65" "5_70" "5_75" "5_80" "5_85" "5_90" "5_95" "5_100")

server_exec() {
    ssh -oStrictHostKeyChecking=no -p 22 "$1" "$2";
}

LOADER="node-001.startup-exp.simbricks-pg0.wisc.cloudlab.us"
WORKER="node-002.startup-exp.simbricks-pg0.wisc.cloudlab.us"

for exp in "${EXPS[@]}"; do
    echo "Starting $exp"

    server_exec $WORKER "tmux kill-session -t kubelet"
    server_exec $WORKER "tmux kill-session -t util"
    server_exec $WORKER "tmux kill-session -t ctr"

    cp ~/traces/$exp ~/loader/data/traces/example/invocations.csv
    
    server_exec $WORKER "tmux new-session -d -s 'kubelet' 'sleep 200; ~/loader/scripts/recorder/kubelet.sh'"
    server_exec $WORKER "tmux new-session -d -s 'util' '~/loader/scripts/recorder/worker-utilization.sh'"
    server_exec $WORKER "tmux new-session -d -s 'ctr' 'sleep 200; ~/loader/scripts/recorder/containerd.sh'"

    go run cmd/loader.go --config=cmd/config_knative_trace.json
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

    sleep 60
    echo "Experiment $exp ended"
done
