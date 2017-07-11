package dependencystore

const dependenciesMapping = `{
   "settings":{
      "index.mapping.nested_fields.limit":50,
      "index.requests.cache.enable":true,
      "index.mapper.dynamic":false
   },
   "mappings":{
      "_default_":{
         "_all":{
            "enabled":false
         }
      },
      "dependencies":{ "enabled": false }
   }
}`
