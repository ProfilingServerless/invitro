import pandas as pd
import numpy as np
import re

import matplotlib.pyplot as plt


EXP_DIR="5_x_e3"

fs = [f'5_{i}' for i in range(10, 105, 5)]


class ExperimentResult:
    name = ""

    pods_created = 0
    e2e_p50 = -1
    e2e_p80 = -1
    e2e_avg = -1

    worker_setup_p50 = -1
    worker_setup_p80 = -1
    worker_setup_avg = -1

    sandbox_created = 0
    sandbox_p50 = -1
    sandbox_p80 = -1
    sandbox_avg = -1

    metrics_sandbox_p50 = -1    
    metrics_sandbox_p80 = -1    
    metrics_sandbox_avg = -1    

    metrics_network_p50 = -1    
    metrics_network_p80 = -1    
    metrics_network_avg = -1

    metrics_container_start_p50 = -1
    metrics_container_start_p80 = -1
    metrics_container_start_avg = -1

    
    concurrency = -1

    kubelet_cpu = -1
    containerd_cpu = -1


allRes = []


for f in fs:
    res = ExperimentResult()
    res.name = f


    df = pd.read_csv(f'{EXP_DIR}/{f}/pods.csv')

    sandbox = list(df['created_sandbox_timestamp'] - df['creating_sandbox_timestamp'])
    mask = ~np.isnan(sandbox)
    sandbox = np.compress(mask, sandbox)

    e2e = list(df['watched_observed_running_timestamp'] - df['first_seen_timestamp'])
    mask = ~np.isnan(e2e)
    e2e = np.compress(mask, e2e)

    worker_setup = list(df['creating_sandbox_timestamp'] - df['first_seen_timestamp'])
    mask = ~np.isnan(worker_setup)
    worker_setup = np.compress(mask, worker_setup)

    res.pods_created= len(e2e)
    res.e2e_p50 = int(round(np.percentile(e2e, 50), 3) * 1000)
    res.e2e_p80 = int(round(np.percentile(e2e, 80), 3) * 1000)
    res.e2e_avg = int(round(np.mean(e2e), 3) * 1000)

    res.sandbox_created = len(sandbox)
    res.sandbox_p50 = int(round(np.percentile(sandbox, 50), 3) * 1000)
    res.sandbox_p80 = int(round(np.percentile(sandbox, 80), 3) * 1000)
    res.sandbox_avg = int(round(np.mean(sandbox), 3) * 1000)

    res.worker_setup_p50 = int(round(np.percentile(worker_setup, 50), 3) * 1000)
    res.worker_setup_p80 = int(round(np.percentile(worker_setup, 80), 3) * 1000)
    res.worker_setup_avg = int(round(np.mean(worker_setup), 3) * 1000)

    starts = df['first_seen_timestamp']
    mask = ~np.isnan(starts)
    starts = np.compress(mask, starts)
    ends = df['watched_observed_running_timestamp']
    mask = ~np.isnan(ends)
    ends = np.compress(mask, ends)
    
    starts = sorted(starts)
    ends = sorted(ends)
    
    c = 0
    i = 0
    j = 0
    m = -1
    while i < len(starts) and j < len(ends):
        if i == len(starts):
            c += 1
            j += 1
        elif j == len(ends):
            c -= 1
            i += 1
        elif starts[i] < ends[j]:
            c += 1
            i += 1
        else:
            c -= 1
            j += 1
    
        if c > m:
            m = c
    
    res.concurrency = m


    # retreiving stass
    with open(f'{EXP_DIR}/{f}/stats.txt', 'r') as file:
        content = file.read()

    pattern = r'\d+\.\d+'
    mchs = re.findall(pattern, content) 

    res.metrics_sandbox_p50 = int(float(mchs[0]) * 1000)
    res.metrics_sandbox_p80 = int(float(mchs[1]) * 1000)
    res.metrics_sandbox_avg = int(float(mchs[2]) * 1000)

    res.metrics_network_p50 = int(float(mchs[3]) * 1000)
    res.metrics_network_p80 = int(float(mchs[4]) * 1000)
    res.metrics_network_avg = int(float(mchs[5]) * 1000)

    res.metrics_container_start_p50 = int(float(mchs[6]) * 1000)
    res.metrics_container_start_p80 = int(float(mchs[7]) * 1000)
    res.metrics_container_start_avg = int(float(mchs[8]) * 1000)

    allRes.append(res)


results = allRes


# Prepare data for plotting
labels = [res.name for res in results]
worker_setup = [res.worker_setup_p50 for res in results]
metrics_sandbox = [res.metrics_sandbox_p50 - res.metrics_network_p50 for res in results]
metrics_network = [res.metrics_network_p50 for res in results]
metrics_container_start = [res.metrics_container_start_p50 * 2 for res in results]

# Create an array for the bottom of each bar segment
bottom_metrics_sandbox = np.array(worker_setup)
bottom_metrics_network = bottom_metrics_sandbox + np.array(metrics_sandbox)
bottom_metrics_container_start = bottom_metrics_network + np.array(metrics_network)

# Create the plot
plt.figure(figsize=(12, 7))

plt.bar(labels, worker_setup, label='Worker Setup P50', color='lightblue')
plt.bar(labels, metrics_sandbox, bottom=bottom_metrics_sandbox, label='Sandbox Creation(cgroups, ...)', color='lightgreen')
plt.bar(labels, metrics_network, bottom=bottom_metrics_network, label='Network Sandbox Creation', color='salmon')
plt.bar(labels, metrics_container_start, bottom=bottom_metrics_container_start, label='Metrics Container Start P50 * 2', color='orange')

# Adding labels and title
plt.xlabel('Experiment Labels')
plt.ylabel('Latency (ms)')
plt.title('Stacked Bar Chart of Latency Metrics')
plt.xticks(rotation=45)
plt.legend()
plt.tight_layout()
plt.savefig('startup_breakdown_5_x_e2.png')


# Show the plot
plt.show()
