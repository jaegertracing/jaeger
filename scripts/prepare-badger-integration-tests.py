#!/usr/bin/env python3

import yaml
import os
import tempfile

config_file = os.path.join(os.path.dirname(__file__), '../cmd/jaeger/badger_config.yaml') 

with open(config_file, 'r') as f:
    config = yaml.safe_load(f)
temp_config = config.copy()

if 'storage_cleaner' not in temp_config['service']['extensions'] :
    config['service']['extensions'].insert(1, 'storage_cleaner') 

temp_config['extensions']['storage_cleaner'] = {
    'trace_storage': 'badger_main'
}

with tempfile.NamedTemporaryFile(mode='w', delete=False, dir=os.path.dirname(config_file),suffix='.yaml') as f:
    temp_config_file = f.name
    yaml.dump(temp_config, f, sort_keys=False)

print(temp_config_file)
