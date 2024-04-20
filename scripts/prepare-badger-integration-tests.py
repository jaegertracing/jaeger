#!/usr/bin/env python3

import yaml
import os

config_file = os.path.join(os.path.dirname(__file__), '../cmd/jaeger/badger_config.yaml') 

with open(config_file, 'r') as f:
    config = yaml.safe_load(f)
if 'badger_cleaner' not in config['service']['extensions'] :
    config['service']['extensions'].insert(1, 'badger_cleaner') 

config['extensions']['badger_cleaner'] = {
    'trace_storage': 'badger_main'
}

with open(config_file, 'w') as f:
    yaml.dump(config, f, sort_keys=False)
