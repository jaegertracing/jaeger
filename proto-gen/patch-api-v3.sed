s|import "google/protobuf/duration.proto";|import "google/protobuf/duration.proto";\
\
import "gogoproto/gogo.proto";\
\
option (gogoproto.marshaler_all) = true;\
option (gogoproto.unmarshaler_all) = true;\
option (gogoproto.sizer_all) = true;\
|g

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
