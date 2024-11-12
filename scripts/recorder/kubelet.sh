#!/usr/bin/bash
sudo journalctl -f -u kubelet | grep --line-buffered -E 'Creating container in pod|Created container in pod|Receiving a new pod|Creating PodSandbox for pod|Created PodSandbox for pod|Observed pod startup duration' > kubelet.txt

