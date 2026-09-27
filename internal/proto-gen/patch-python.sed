# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Strip the HTTP-gateway and OpenAPI option annotations from api_v3's protos
# before generating Python from them, in the manner of internal/proto-gen/patch.sed.
#
# A gRPC client reads neither annotation, but protoc records the files defining
# them as dependencies, and the generated Python then imports their generated
# modules just to register the option extensions. gnostic publishes no Python
# distribution, so keeping the annotations would mean generating and committing
# gnostic's whole OpenAPI v3 model for options nobody reads.
#
# The `proto` target checks that nothing survived this patch, so a shape these
# expressions do not handle fails codegen instead of leaking into the output.

# The imports that only define option annotations.
\|^import "google/api/annotations.proto";$|d
\|^import "google/api/field_behavior.proto";$|d
\|^import "gnostic/openapiv3/annotations.proto";$|d

# Field behaviour, always a single-line option on one field.
s| \[(google\.api\.field_behavior) = [A-Z]*\];|;|g

# The HTTP bindings, a braced block inside an rpc body. The closing brace of a
# nested additional_bindings is indented deeper, so this stops at the right one.
/^    option (google\.api\.http) = {$/,/^    };$/d

# The OpenAPI property annotations, a bracketed option list spanning several
# lines on one field. Close the field declaration that opens the list, then drop
# the list itself.
/^  [A-Za-z<>, ]* = [0-9]* \[$/ s| = \([0-9]*\) \[$| = \1;|
/^    (openapi\.v3\.property) = {$/,/^  \];$/d

# Message-level OpenAPI schema options, single-line and braced-block forms. These
# arrived with the expression protos, which api_v3 now imports.
/^  option (openapi\.v3\.schema) = {[^}]*};$/d
/^  option (openapi\.v3\.schema) = {$/,/^  };$/d

# Single-line property annotations on a field, such as a list's min_items.
s| \[(openapi\.v3\.property) = {[^}]*}\];|;|g
