#!/usr/bin/bash

# Note.
# Run it directly on a single worker node

EXPS=("5" "50" "100" "150" "200")

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
tmux kill-session -t kubelet
tmux kill-session -t util
tmux kill-session -t ctr

for exp in "${EXPS[@]}"; do
    echo "Starting $exp"
    

    tmux new-session -d -s 'kubelet' '~/loader/scripts/recorder/kubelet.sh'
    tmux new-session -d -s 'util' '~/loader/scripts/recorder/worker-utilization.sh'
    tmux new-session -d -s 'ctr' '~/loader/scripts/recorder/containerd.sh'

    for i in $(seq 1 $exp); do
        sudo cp ~/loader/data/traces/experiments/C/trace-func-$i.yaml /etc/kubernetes/manifests/
    done

    sleep 60
    
    tmux kill-session -t kubelet
    tmux kill-session -t util
    tmux kill-session -t ctr

    python3 ~/loader/scripts/parser/parse_kubelet_logs.py
    cp ~/pods.csv ~/results/$exp

    sudo rm /etc/kubernetes/manifests/*
    sleep 60

    echo "Experiment $exp ended"
done
