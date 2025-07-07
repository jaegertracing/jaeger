# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import requests
import random
import time

url = "http://jaeger-hotrod.default.svc.cluster.local:80/dispatch" 

customer_ids = [123, 392, 731, 567]
i = 0
print("Starting load generator script")
while True:  #Keep sending requests
    customer = random.choice(customer_ids)
    nonse = random.random()
    params = {
        "customer": customer,
        "nonse": nonse
    }

    try:
        res = requests.get(url, params=params, timeout=5)
        print(f"[{i}]th request Sent to {res.url} → Status: {res.status_code}")
    except Exception as e:
        print(f"[{i}]th request Error: {e}")
    i = i + 1
    time.sleep(10)  # Pause between requests to avoid overload
