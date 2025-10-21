# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import os
import requests
import random
import time

TARGET_URL = os.getenv("TARGET_URL", "http://jaeger-hotrod.jaeger.svc.cluster.local/dispatch")
SLEEP_SECONDS = float(os.getenv("SLEEP_SECONDS", "5"))

CUSTOMER_IDS = [123, 392, 731, 567]

print(f"Starting HotROD load generator → {TARGET_URL} (interval={SLEEP_SECONDS}s)")

i = 0
session = requests.Session()

while True:
    customer = random.choice(CUSTOMER_IDS)
    nonse = random.random()
    params = {
        "customer": customer,
        "nonse": nonse,
    }
    try:
        res = session.get(TARGET_URL, params=params, timeout=5)
        print(f"[{i}] Sent to {res.url} → {res.status_code}")
    except Exception as e:
        print(f"[{i}] Error: {e}")
    i += 1
    time.sleep(SLEEP_SECONDS)
