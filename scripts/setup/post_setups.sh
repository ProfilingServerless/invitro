#!/usr/bin/bash

echo "Increase kubelet log level"
sudo sed -i 's/--v=[0-9]\+/--v=4/' /etc/default/kubelet
sudo systemctl daemon-reload
sudo systemctl restart kubelet

echo "Setup zipkin"
~/vhive/scripts/setup_tool setup_zipkin; sleep 5

echo "Setup containerd"
cd ~
wget https://github.com/ProfilingServerless/containerd/releases/download/v1.6.36/containerd_x86_1.6_logged
chmod +x ~/containerd_x86_1.6_logged
tmux send-keys -t containerd:0 'C-c'
sudo cp ~/containerd_x86_1.6_logged /usr/local/bin/containerd 
tmux send -t containerd "sudo containerd 2>&1 | tee ~/containerd_log.txt" ENTER


