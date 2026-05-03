# Migration Guide: Manual Mapping Updates for Scope and Link Attributes

This guide provides the specific Elasticsearch API calls required for users who manage their own mappings (e.g., `es.create-index-templates: false`). These updates enable storage for the new OpenTelemetry **Instrumentation Scope** and **Span Link** attributes.

---

### 1. Update Existing Indices
You can apply these updates to your current active index using the `_mapping` API. 

💡 **Note:** Unlike the Template API, the `_mapping` API is a **merge** operation by default; existing fields will not be affected. We include `dynamic_templates` so that new attributes appearing in the current index are correctly mapped.

**Note:** Replace `jaeger-span-YYYY-MM-DD` with your actual current index name.

```bash
curl -X PUT "http://localhost:9200/jaeger-span-YYYY-MM-DD/_mapping" -H 'Content-Type: application/json' -d'
{
  "dynamic_templates": [
    {
      "scope_tags_map": {
        "path_match": "scopeTag.*",
        "mapping": { "type": "keyword", "ignore_above": 256 }
      }
    },
    {
      "references_tags_map": {
        "path_match": "references.tag.*",
        "mapping": { "type": "keyword", "ignore_above": 256 }
      }
    }
  ],
  "properties": {
    "scopeTag": { "type": "object" },
    "scopeTags": {
      "type": "nested",
      "dynamic": false,
      "properties": {
        "key": { "type": "keyword", "ignore_above": 256 },
        "value": { "type": "keyword", "ignore_above": 256 },
        "type": { "type": "keyword", "ignore_above": 256 }
      }
    },
    "references": {
      "type": "nested",
      "properties": {
        "traceState": { "type": "keyword", "ignore_above": 256 },
        "flags": { "type": "integer" },
        "tag": { "type": "object" },
        "tags": {
          "type": "nested",
          "dynamic": false,
          "properties": {
            "key": { "type": "keyword", "ignore_above": 256 },
            "value": { "type": "keyword", "ignore_above": 256 },
            "type": { "type": "keyword", "ignore_above": 256 }
          }
        }
      }
    }
  }
}'
```

---

### 2. Update Index Templates (For Future Indices)

⚠️ **WARNING:** The Elasticsearch Template API is a **full replacement** operation. If you send only the snippet below, it will delete your existing Jaeger mappings (like `operationName`, `startTime`, etc.). 

**You must MERGE these changes into your existing template definition.**

#### Steps to Update:
1.  **Fetch your current template:**
    ```bash
    curl -s "http://localhost:9200/_index_template/jaeger-span" | jq ' .index_templates[0].index_template' > full_template.json
    ```
2.  **Merge the following fields** into the `mappings` section of your `full_template.json`:

```json
{
  "dynamic_templates": [
    /* Add these to your existing dynamic_templates list */
    {
      "scope_tags_map": {
        "path_match": "scopeTag.*",
        "mapping": { "type": "keyword", "ignore_above": 256 }
      }
    },
    {
      "references_tags_map": {
        "path_match": "references.tag.*",
        "mapping": { "type": "keyword", "ignore_above": 256 }
      }
    }
  ],
  "properties": {
    /* Add these to your existing root properties */
    "scopeTag": { "type": "object" },
    "scopeTags": {
      "type": "nested",
      "dynamic": false,
      "properties": {
        "key": { "type": "keyword", "ignore_above": 256 },
        "value": { "type": "keyword", "ignore_above": 256 },
        "type": { "type": "keyword", "ignore_above": 256 }
      }
    },
    /* Update your existing references block with these new fields */
    "references": {
      "type": "nested",
      "properties": {
        /* ... keep your existing refType, traceID, spanID ... */
        "traceState": { "type": "keyword", "ignore_above": 256 },
        "flags": { "type": "integer" },
        "tag": { "type": "object" },
        "tags": {
          "type": "nested",
          "dynamic": false,
          "properties": {
            "key": { "type": "keyword", "ignore_above": 256 },
            "value": { "type": "keyword", "ignore_above": 256 },
            "type": { "type": "keyword", "ignore_above": 256 }
          }
        }
      }
    }
  }
}
```

3.  **Upload the full updated template:**
    ```bash
    curl -X PUT "http://localhost:9200/_index_template/jaeger-span" -H 'Content-Type: application/json' -d @full_template.json
    ```

---

### Verification
Verify the update by checking the mapping of any index:
```bash
curl -s "http://localhost:9200/jaeger-span-*/_mapping" | jq '.. | .scopeTags?'
```
