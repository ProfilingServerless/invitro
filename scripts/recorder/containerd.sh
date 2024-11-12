#!/usr/bin/bash
sudo tail -f ~/containerd_log.txt | grep --line-buffered -E 'Network sandbox created in' > containerd.txt
