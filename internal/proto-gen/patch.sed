# Strip gnostic annotations import (not needed for backend)
/import "gnostic\/openapiv3\/annotations.proto";/d

# Strip field_behavior import (not needed for backend)
/import "google\/api\/field_behavior.proto";/d

# Strip field_behavior annotations like [(google.api.field_behavior) = REQUIRED]
s/ \[(google\.api\.field_behavior) = REQUIRED\]//g
s/ \[(google\.api\.field_behavior) = \w+\]//g

# Strip openapi.v3 property annotations (keep the field, remove the annotation block)
/(openapi\.v3\.property)/,/^[[:space:]]*\];/d
s/ \[$/;/

0,/import "google\/protobuf\/.*.proto";/{
s|import "google/protobuf/.*.proto";|&\
\
import "gogoproto/gogo.proto";\
\
option (gogoproto.marshaler_all) = true;\
option (gogoproto.unmarshaler_all) = true;\
option (gogoproto.sizer_all) = true;\
|
}
s|google.protobuf.Timestamp \(.*\);|google.protobuf.Timestamp \1 \
  [\
  (gogoproto.nullable) = false,\
  (gogoproto.stdtime) = true\
  ];|g
s|google.protobuf.Duration \(.*\);|google.protobuf.Duration \1 \
  [\
  (gogoproto.nullable) = false,\
  (gogoproto.stdduration) = true\
  ];|g
