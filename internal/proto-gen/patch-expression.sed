0,/^import "gnostic\/openapiv3\/annotations.proto";$/{
s|^import "gnostic/openapiv3/annotations.proto";$|&\
\
import "gogoproto/gogo.proto";\
\
option (gogoproto.marshaler_all) = true;\
option (gogoproto.unmarshaler_all) = true;\
option (gogoproto.sizer_all) = true;\
|
}
