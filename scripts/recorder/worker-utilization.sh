#!/usr/bin/bash

KUBELET_PID=$(ps aux | grep '/usr/bin/kubelet' | grep -v grep | awk '{ print $2 }')
CONTAINERD_PID=$(ps aux | grep containerd | grep -Ev '/containerd|tmux|sudo|grep' | awk '{ print $2 }')

while true; do
    KUBELET_UTIL=$(ps -p $KUBELET_PID -o %cpu,%mem | tail -n+2)
    CONTAINERD_UTIL=$(ps -p $CONTAINERD_PID -o %cpu,%mem | tail -n+2)
    echo "$(date +%s) $KUBELET_UTIL" >> kubelet_util.txt
    echo "$(date +%s) $CONTAINERD_UTIL" >> containerd_util.txt
    sleep 15
done

