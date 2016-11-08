#!/bin/bash

set -e

python scripts/updateLicense.py $(go list -json $(glide nv) | jq -r '.Dir + "/" + (.GoFiles | .[])')
