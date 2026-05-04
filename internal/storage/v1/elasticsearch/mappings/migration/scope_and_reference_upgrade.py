#!/usr/bin/env python3

import os
import sys
import argparse
import requests
from requests.auth import HTTPBasicAuth

"""
Script to update Jaeger Elasticsearch templates with OTLP-compatible fields:
- scopeTag (object for Kibana/UI support)
- scopeTags (nested for search)
- references.tags (nested for span link attributes)

Security environment variables:
- ES_USERNAME: User for basic auth
- ES_PASSWORD: Password for basic auth
- ES_API_KEY: API Key for auth (takes precedence over basic auth)
- ES_CA_BUNDLE: Path to CA bundle or 'False' to disable SSL verification
"""

def update_template(es_url, index_prefix):
    template_name = f"{index_prefix}-jaeger-span"
    url = f"{es_url.rstrip('/')}/_template/{template_name}"

    # Security parameters from environment
    username = os.getenv('ES_USERNAME')
    password = os.getenv('ES_PASSWORD')
    api_key = os.getenv('ES_API_KEY')
    ca_bundle = os.getenv('ES_CA_BUNDLE', True)

    if ca_bundle in ['False', 'false', '0']:
        ca_bundle = False

    headers = {}
    auth = None

    if api_key:
        headers['Authorization'] = f'ApiKey {api_key}'
    elif username and password:
        auth = HTTPBasicAuth(username, password)

    # 1. Fetch existing template
    print(f"[*] Fetching template: {template_name} from {url}")
    try:
        response = requests.get(url, auth=auth, headers=headers, verify=ca_bundle)
        if response.status_code == 404:
            print(f"[!] Error: Template '{template_name}' not found in Elasticsearch.")
            sys.exit(1)
        response.raise_for_status()
    except requests.exceptions.RequestException as e:
        print(f"[!] HTTP Request failed: {e}")
        sys.exit(1)

    # ES returns { "template_name": { ... } }
    raw_data = response.json()
    template_data = raw_data[template_name]

    # 2. Modify template mappings
    mappings = template_data.get("mappings", {})

    # Handle both ES 6 (type-named) and ES 7+ (properties-first) structures
    target_mappings = mappings
    if "properties" not in mappings:
        for key, value in mappings.items():
            if isinstance(value, dict) and "properties" in value:
                target_mappings = value
                break

    properties = target_mappings.setdefault("properties", {})
    modified = False

    # OTLP Tag structure definition
    nested_tag_props = {
        "type": "nested",
        "dynamic": False,
        "properties": {
            "key": {"type": "keyword", "ignore_above": 256},
            "value": {"type": "keyword", "ignore_above": 256},
            "type": {"type": "keyword", "ignore_above": 256}
        }
    }

    # Inject scopeTag (object)
    if "scopeTag" not in properties:
        print("[+] Injecting 'scopeTag' mapping")
        properties["scopeTag"] = {"type": "object"}
        modified = True

    # Inject scopeTags (nested)
    if "scopeTags" not in properties:
        print("[+] Injecting 'scopeTags' mapping")
        properties["scopeTags"] = nested_tag_props
        modified = True

    # Inject references.tags (link attributes)
    if "references" in properties:
        ref_props = properties["references"].setdefault("properties", {})
        if "tags" not in ref_props:
            print("[+] Injecting 'references.tags' mapping")
            ref_props["tags"] = nested_tag_props
            modified = True

    # Ensure dynamic_templates handles scopeTag.*
    dynamic_templates = target_mappings.setdefault("dynamic_templates", [])
    if not any("scope_tags_map" in dt for dt in dynamic_templates):
        print("[+] Injecting 'scope_tags_map' dynamic template")
        dynamic_templates.append({
            "scope_tags_map": {
                "mapping": {"type": "keyword", "ignore_above": 256},
                "path_match": "scopeTag.*"
            }
        })
        modified = True

    if not modified:
        print("[*] Template is already up to date.")
        return

    # 3. Upload the updated template
    print(f"[*] Uploading updated template '{template_name}'...")
    try:
        put_response = requests.put(url, json=template_data, auth=auth, headers=headers, verify=ca_bundle)
        put_response.raise_for_status()
        print("[+] Success: Template updated.")
    except requests.exceptions.RequestException as e:
        print(f"[!] Failed to update template: {e}")
        if e.response is not None:
            print(f"[!] Server Response: {e.response.text}")
        sys.exit(1)

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Update Jaeger ES template with OTLP fields.")
    parser.add_argument("--index-prefix", default="", help="Jaeger index prefix")
    parser.add_argument("--es-url", required=True, help="Elasticsearch base URL")

    args = parser.parse_args()
    update_template(args.es_url, args.index_prefix)
