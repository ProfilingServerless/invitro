#!/usr/bin/bash

# Note.
# Run it on a single worker

EXPS=("5" "50" "100" "150" "200")
# EXPS=("150")

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
        kubectl apply -f ~/loader/data/traces/experiments/C/trace-func-$i.yaml &
    done
    wait

    sleep 60
    
    tmux kill-session -t kubelet
    tmux kill-session -t util
    tmux kill-session -t ctr

    python3 ~/loader/scripts/parser/parse_kubelet_logs.py
    cp ~/pods.csv ~/results/$exp

    for i in $(seq 1 $exp); do
        kubectl delete po trace-func-$i & 
    done
    wait
    sleep 30

    echo "Experiment $exp ended"
done
