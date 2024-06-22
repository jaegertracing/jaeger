services_list=("driver" "customer" "mysql" "redis" "frontend" "route" "ui")
  for service in "${services_list[@]}"; do
    echo "Processing service: $service"
    while [ 1=1 ]; do
      response=$(curl -s "http://localhost:16686/api/metrics/calls?service=$service&endTs=$(date +%s)000&lookback=1000&step=100&ratePer=60000")
      echo "Response: $response"
      service_name=$(echo "$response" | jq -r 'if .metrics and .metrics[0] then .metrics[0].labels[] | select(.name=="service_name") | .value else empty end')
      if [ "$service_name" != "$service" ]; then
        echo "Service name does not match '$service'"
      else
        echo "Service name matched with '$service'"
        break
      fi
    done
    done