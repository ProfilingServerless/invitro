#!/usr/bin/bash

echo "Setup kubelet"
sudo systemctl stop kubelet
sudo sed -i 's/--v=[0-9]\+/--v=4/' /etc/default/kubelet
wget https://github.com/ProfilingServerless/kubernetes/releases/download/v1.28.10/kubelet -O ~/kubelet_x86_1.29.10_logged
chmod +x kubelet_x86_1.29.10_logged
sudo cp ~/kubelet_x86_1.29.10_logged /usr/bin/kubelet
sudo systemctl daemon-reload
sudo systemctl restart kubelet

# echo "Setup zipkin"
# ~/vhive/scripts/setup_tool setup_zipkin; sleep 5

echo "Setup containerd"
wget https://github.com/ProfilingServerless/containerd/releases/download/v1.6.36/containerd_x86_1.6_logged
chmod +x ~/containerd_x86_1.6_logged
tmux kill-session -t containerd
sudo cp ~/containerd_x86_1.6_logged /usr/local/bin/containerd 
sudo mkdir -p /etc/containerd
sudo cp ~/loader/config/containerd.toml /etc/containerd/config.toml
tmux new -s containerd
tmux send -t containerd "sudo containerd 2>&1 | tee ~/containerd_log.txt" ENTER
