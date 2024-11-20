#!/usr/bin/python3
import re
from datetime import datetime 
import csv

KUBELET_FILE="kubelet.txt"
CONTAINERD_FILE="containerd.txt"

# [pod_name] -> {first_seen_timestamp, creating_sandbox_timestamp, created_sandbox_timestamp, watched_observed_running_timestamp, e2e_duration}
pods = {}

first_seen_pattern = r'(?P<timestamp>\d{2}:\d{2}:\d{2}\.\d+) .*Receiving a new pod.* pod="default/(?P<pod_name>[^"]+)"'
creating_sandbox_pattern = r'(?P<timestamp>\d{2}:\d{2}:\d{2}\.\d+) .*Creating PodSandbox for pod.* pod="default/(?P<pod_name>[^"]+)"'
created_sandbox_pattern = r'(?P<timestamp>\d{2}:\d{2}:\d{2}\.\d+) .*Created PodSandbox for pod.* pod="default/(?P<pod_name>[^"]+)"'
e2e_duration_pattern = r'Observed pod startup duration.* pod="default/(?P<pod_name>[^"]+)" .* podStartE2EDuration="(?P<e2e_duration>\d+\.\d+)s" .* watchObservedRunningTime=".*(?P<observed_running_time>\d{2}:\d{2}:\d{2}\.\d+).*"'
creating_container_pattern = r'(?P<timestamp>\d{2}:\d{2}:\d{2}\.\d+) .*Creating container in pod.* pod="default/(?P<pod_name>[^"]+)"'
created_container_pattern = r'(?P<timestamp>\d{2}:\d{2}:\d{2}\.\d+) .*Created container in pod.* pod="default/(?P<pod_name>[^"]+)"'
network_sandbox_pattern = r'Network sandbox created in (?P<duration>\d+\.\d+)ms.*(?P<timestamp>\d{2}:\d{2}:\d{2}\.\d+).*podsandboxname=(?P<pod_name>[^_]+)'
 

POD_SCHEM =  ['first_seen_timestamp',
              'creating_sandbox_timestamp',
              'created_sandbox_timestamp',
              'watched_observed_running_timestamp',
              'e2e_duration',
              'creating_queue_timestamp',
              'created_queue_timestamp',
              'created_workload_timestamp',
              'creating_workload_timestamp',
              'network_sandbox_duration',
              'network_sandbox_timestamp']


def put_record(pod_name, idx, v):
    if not pod_name in pods:
        pods[pod_name] = [None, None, None, None, None, None, None, None, None, None, None]
    pods[pod_name][idx] = v

# receives string in format of dd:dd:dd.d+ and gives epoch for that in milliseconds
def timestamp2epoch(timestamp):
    if len(timestamp) < 12:
        timestamp += "0" * (12 - len(timestamp))
    today = datetime.now().date()
    combined_format = f"{today} {timestamp[:12]}"
    return datetime.strptime(combined_format, "%Y-%m-%d %H:%M:%S.%f").timestamp()



def process_kubelet_file(file_path):
    with open(file_path, 'r') as file:
        for line in file:
            mch = re.search(first_seen_pattern, line)
            if mch:
                first_seen_timestamp = timestamp2epoch(mch.group('timestamp'))
                pod_name = mch.group('pod_name')
                put_record(pod_name, 0, first_seen_timestamp)
                continue
            
            mch = re.search(creating_sandbox_pattern, line)
            if mch:
                creating_sandbox_timestamp = timestamp2epoch(mch.group('timestamp'))
                pod_name = mch.group('pod_name')
                put_record(pod_name, 1, creating_sandbox_timestamp)
                continue

            mch = re.search(created_sandbox_pattern, line)
            if mch:
                created_sandbox_timestamp = timestamp2epoch(mch.group('timestamp'))
                pod_name = mch.group('pod_name')
                put_record(pod_name, 2, created_sandbox_timestamp)
                continue

            mch = re.search(e2e_duration_pattern, line)
            if mch:
                e2e_duration = int(float(mch.group('e2e_duration')) * 1000)
                watched_observed_running_timestamp = timestamp2epoch(mch.group('observed_running_time'))
                pod_name = mch.group('pod_name')
                put_record(pod_name, 3, watched_observed_running_timestamp)
                put_record(pod_name, 4, e2e_duration)
                continue

            mch = re.search(creating_container_pattern, line)
            if mch:
                timestamp = timestamp2epoch(mch.group('timestamp'))
                pod_name = mch.group('pod_name')
                if 'gcr.io/knative-releases/knative.dev/serving/cmd/queue' in line:
                    put_record(pod_name, 5, timestamp)
                else:
                    put_record(pod_name, 7, timestamp)
                continue

            mch = re.search(created_container_pattern, line)
            if mch:
                timestamp = timestamp2epoch(mch.group('timestamp'))
                pod_name = mch.group('pod_name')
                if 'gcr.io/knative-releases/knative.dev/serving/cmd/queue' in line:
                    put_record(pod_name, 6, timestamp)
                else:
                    put_record(pod_name, 8, timestamp)
                continue


def process_containerd_file(file_path):
    with open(file_path, 'r') as file:
        for line in file:
            mch = re.search(network_sandbox_pattern, line)
            if mch:
                duration = int(float(mch.group('duration')))
                pod_name = mch.group('pod_name')
                timestamp = timestamp2epoch(mch.group('timestamp'))
                put_record(pod_name, 9, duration)
                put_record(pod_name, 10, timestamp)
                continue

process_kubelet_file(KUBELET_FILE)
process_containerd_file(CONTAINERD_FILE)

header = ['pod_name'] + POD_SCHEM

with open('pods.csv', mode='w', newline='') as f:
    w = csv.writer(f)
    w.writerow(header)
    for k, v in pods.items():
        w.writerow([k] + v)
