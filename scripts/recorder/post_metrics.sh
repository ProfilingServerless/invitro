#!/usr/bin/bash

PROMETHEUS_IP=$(kubectl -n monitoring get svc prometheus-kube-prometheus-prometheus | tail -n+2 | awk '{ print $3 }')

TIMESTAMP=$(date +%s)
    
RES=$(curl -s -G --data-urlencode 'query=histogram_quantile(0.5, sum(rate(containerd_cri_sandbox_runtime_create_seconds_bucket[350s])) by (le))' http://$PROMETHEUS_IP:9090/api/v1/query)
STATUS=$(echo $RES | jq -r '.status')
if [[ "$STATUS" == "success" ]]; then
    SANDBOX_P50=$(echo $RES | jq -r '.data.result[0].value[1]') 
else
    SANDBOX_P50="-99"
fi

RES=$(curl -s -G --data-urlencode 'query=histogram_quantile(0.8, sum(rate(containerd_cri_sandbox_runtime_create_seconds_bucket[350s])) by (le))' http://$PROMETHEUS_IP:9090/api/v1/query)
STATUS=$(echo $RES | jq -r '.status')
if [[ "$STATUS" == "success" ]]; then
    SANDBOX_P80=$(echo $RES | jq -r '.data.result[0].value[1]') 
else
    SANDBOX_P80="-99"
fi

RES=$(curl -s -G --data-urlencode 'query=rate(containerd_cri_sandbox_runtime_create_seconds_sum[350s]) / rate(containerd_cri_sandbox_runtime_create_seconds_count[350s])' http://$PROMETHEUS_IP:9090/api/v1/query)
STATUS=$(echo $RES | jq -r '.status')
if [[ "$STATUS" == "success" ]]; then
    SANDBOX_AVG=$(echo $RES | jq -r '.data.result[0].value[1]') 
else
    SANDBOX_AVG="-99"
fi

RES=$(curl -s -G --data-urlencode 'query=histogram_quantile(0.5, sum(rate(containerd_cri_sandbox_create_network_seconds_bucket[350s])) by (le))' http://$PROMETHEUS_IP:9090/api/v1/query)
STATUS=$(echo $RES | jq -r '.status')
if [[ "$STATUS" == "success" ]]; then
    NETWORK_SANDBOX_P50=$(echo $RES | jq -r '.data.result[0].value[1]') 
else
    NETWORK_SANDBOX_P50="-99"
fi

RES=$(curl -s -G --data-urlencode 'query=histogram_quantile(0.8, sum(rate(containerd_cri_sandbox_create_network_seconds_bucket[350s])) by (le))' http://$PROMETHEUS_IP:9090/api/v1/query)
STATUS=$(echo $RES | jq -r '.status')
if [[ "$STATUS" == "success" ]]; then
    NETWORK_SANDBOX_P80=$(echo $RES | jq -r '.data.result[0].value[1]') 
else
    NETWORK_SANDBOX_P80="-99"
fi

RES=$(curl -s -G --data-urlencode 'query=rate(containerd_cri_sandbox_create_network_seconds_sum[350s]) / rate(containerd_cri_sandbox_create_network_seconds_count[350s])' http://$PROMETHEUS_IP:9090/api/v1/query)
STATUS=$(echo $RES | jq -r '.status')
if [[ "$STATUS" == "success" ]]; then
    NETWORK_SANDBOX_AVG=$(echo $RES | jq -r '.data.result[0].value[1]') 
else
    NETWORK_SANDBOX_AVG="-99"
fi

echo "timestamp: $TIMESTAMP"
echo "sandbox: p50. $SANDBOX_P50 p80. $SANDBOX_P80 avg. $SANDBOX_AVG"
echo "network sandbox: p50. $NETWORK_SANDBOX_P50 p80. $NETWORK_SANDBOX_P80 avg. $NETWORK_SANDBOX_AVG"
