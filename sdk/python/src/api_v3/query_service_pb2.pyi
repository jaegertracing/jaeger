import datetime

from google.protobuf import duration_pb2 as _duration_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from opentelemetry.proto.trace.v1 import trace_pb2 as _trace_pb2
from expression.v1 import expression_pb2 as _expression_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class GetTraceRequest(_message.Message):
    __slots__ = ("trace_id", "start_time", "end_time", "raw_traces")
    TRACE_ID_FIELD_NUMBER: _ClassVar[int]
    START_TIME_FIELD_NUMBER: _ClassVar[int]
    END_TIME_FIELD_NUMBER: _ClassVar[int]
    RAW_TRACES_FIELD_NUMBER: _ClassVar[int]
    trace_id: str
    start_time: _timestamp_pb2.Timestamp
    end_time: _timestamp_pb2.Timestamp
    raw_traces: bool
    def __init__(self, trace_id: _Optional[str] = ..., start_time: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., end_time: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., raw_traces: _Optional[bool] = ...) -> None: ...

class TraceQueryParameters(_message.Message):
    __slots__ = ("service_name", "operation_name", "attributes", "start_time_min", "start_time_max", "duration_min", "duration_max", "search_depth", "raw_traces", "filter")
    class AttributesEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    SERVICE_NAME_FIELD_NUMBER: _ClassVar[int]
    OPERATION_NAME_FIELD_NUMBER: _ClassVar[int]
    ATTRIBUTES_FIELD_NUMBER: _ClassVar[int]
    START_TIME_MIN_FIELD_NUMBER: _ClassVar[int]
    START_TIME_MAX_FIELD_NUMBER: _ClassVar[int]
    DURATION_MIN_FIELD_NUMBER: _ClassVar[int]
    DURATION_MAX_FIELD_NUMBER: _ClassVar[int]
    SEARCH_DEPTH_FIELD_NUMBER: _ClassVar[int]
    RAW_TRACES_FIELD_NUMBER: _ClassVar[int]
    FILTER_FIELD_NUMBER: _ClassVar[int]
    service_name: str
    operation_name: str
    attributes: _containers.ScalarMap[str, str]
    start_time_min: _timestamp_pb2.Timestamp
    start_time_max: _timestamp_pb2.Timestamp
    duration_min: _duration_pb2.Duration
    duration_max: _duration_pb2.Duration
    search_depth: int
    raw_traces: bool
    filter: _expression_pb2.Call
    def __init__(self, service_name: _Optional[str] = ..., operation_name: _Optional[str] = ..., attributes: _Optional[_Mapping[str, str]] = ..., start_time_min: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., start_time_max: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., duration_min: _Optional[_Union[datetime.timedelta, _duration_pb2.Duration, _Mapping]] = ..., duration_max: _Optional[_Union[datetime.timedelta, _duration_pb2.Duration, _Mapping]] = ..., search_depth: _Optional[int] = ..., raw_traces: _Optional[bool] = ..., filter: _Optional[_Union[_expression_pb2.Call, _Mapping]] = ...) -> None: ...

class FindTracesRequest(_message.Message):
    __slots__ = ("query",)
    QUERY_FIELD_NUMBER: _ClassVar[int]
    query: TraceQueryParameters
    def __init__(self, query: _Optional[_Union[TraceQueryParameters, _Mapping]] = ...) -> None: ...

class GetServicesRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetServicesResponse(_message.Message):
    __slots__ = ("services",)
    SERVICES_FIELD_NUMBER: _ClassVar[int]
    services: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, services: _Optional[_Iterable[str]] = ...) -> None: ...

class GetOperationsRequest(_message.Message):
    __slots__ = ("service", "span_kind")
    SERVICE_FIELD_NUMBER: _ClassVar[int]
    SPAN_KIND_FIELD_NUMBER: _ClassVar[int]
    service: str
    span_kind: str
    def __init__(self, service: _Optional[str] = ..., span_kind: _Optional[str] = ...) -> None: ...

class Operation(_message.Message):
    __slots__ = ("name", "span_kind")
    NAME_FIELD_NUMBER: _ClassVar[int]
    SPAN_KIND_FIELD_NUMBER: _ClassVar[int]
    name: str
    span_kind: str
    def __init__(self, name: _Optional[str] = ..., span_kind: _Optional[str] = ...) -> None: ...

class GetOperationsResponse(_message.Message):
    __slots__ = ("operations",)
    OPERATIONS_FIELD_NUMBER: _ClassVar[int]
    operations: _containers.RepeatedCompositeFieldContainer[Operation]
    def __init__(self, operations: _Optional[_Iterable[_Union[Operation, _Mapping]]] = ...) -> None: ...

class GetDependenciesRequest(_message.Message):
    __slots__ = ("start_time", "end_time")
    START_TIME_FIELD_NUMBER: _ClassVar[int]
    END_TIME_FIELD_NUMBER: _ClassVar[int]
    start_time: _timestamp_pb2.Timestamp
    end_time: _timestamp_pb2.Timestamp
    def __init__(self, start_time: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., end_time: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class DependenciesResponse(_message.Message):
    __slots__ = ("dependencies",)
    DEPENDENCIES_FIELD_NUMBER: _ClassVar[int]
    dependencies: _containers.RepeatedCompositeFieldContainer[Dependency]
    def __init__(self, dependencies: _Optional[_Iterable[_Union[Dependency, _Mapping]]] = ...) -> None: ...

class Dependency(_message.Message):
    __slots__ = ("parent", "child", "call_count")
    PARENT_FIELD_NUMBER: _ClassVar[int]
    CHILD_FIELD_NUMBER: _ClassVar[int]
    CALL_COUNT_FIELD_NUMBER: _ClassVar[int]
    parent: str
    child: str
    call_count: int
    def __init__(self, parent: _Optional[str] = ..., child: _Optional[str] = ..., call_count: _Optional[int] = ...) -> None: ...

class ServiceSummary(_message.Message):
    __slots__ = ("name", "span_count", "error_span_count")
    NAME_FIELD_NUMBER: _ClassVar[int]
    SPAN_COUNT_FIELD_NUMBER: _ClassVar[int]
    ERROR_SPAN_COUNT_FIELD_NUMBER: _ClassVar[int]
    name: str
    span_count: int
    error_span_count: int
    def __init__(self, name: _Optional[str] = ..., span_count: _Optional[int] = ..., error_span_count: _Optional[int] = ...) -> None: ...

class TraceSummary(_message.Message):
    __slots__ = ("trace_id", "root_service_name", "root_operation_name", "min_start_time_unix_nano", "max_end_time_unix_nano", "span_count", "error_span_count", "orphan_span_count", "services")
    TRACE_ID_FIELD_NUMBER: _ClassVar[int]
    ROOT_SERVICE_NAME_FIELD_NUMBER: _ClassVar[int]
    ROOT_OPERATION_NAME_FIELD_NUMBER: _ClassVar[int]
    MIN_START_TIME_UNIX_NANO_FIELD_NUMBER: _ClassVar[int]
    MAX_END_TIME_UNIX_NANO_FIELD_NUMBER: _ClassVar[int]
    SPAN_COUNT_FIELD_NUMBER: _ClassVar[int]
    ERROR_SPAN_COUNT_FIELD_NUMBER: _ClassVar[int]
    ORPHAN_SPAN_COUNT_FIELD_NUMBER: _ClassVar[int]
    SERVICES_FIELD_NUMBER: _ClassVar[int]
    trace_id: str
    root_service_name: str
    root_operation_name: str
    min_start_time_unix_nano: int
    max_end_time_unix_nano: int
    span_count: int
    error_span_count: int
    orphan_span_count: int
    services: _containers.RepeatedCompositeFieldContainer[ServiceSummary]
    def __init__(self, trace_id: _Optional[str] = ..., root_service_name: _Optional[str] = ..., root_operation_name: _Optional[str] = ..., min_start_time_unix_nano: _Optional[int] = ..., max_end_time_unix_nano: _Optional[int] = ..., span_count: _Optional[int] = ..., error_span_count: _Optional[int] = ..., orphan_span_count: _Optional[int] = ..., services: _Optional[_Iterable[_Union[ServiceSummary, _Mapping]]] = ...) -> None: ...

class FindTraceSummariesRequest(_message.Message):
    __slots__ = ("query",)
    QUERY_FIELD_NUMBER: _ClassVar[int]
    query: TraceQueryParameters
    def __init__(self, query: _Optional[_Union[TraceQueryParameters, _Mapping]] = ...) -> None: ...

class FindTraceSummariesResponse(_message.Message):
    __slots__ = ("summaries",)
    SUMMARIES_FIELD_NUMBER: _ClassVar[int]
    summaries: _containers.RepeatedCompositeFieldContainer[TraceSummary]
    def __init__(self, summaries: _Optional[_Iterable[_Union[TraceSummary, _Mapping]]] = ...) -> None: ...

class GRPCGatewayError(_message.Message):
    __slots__ = ("error",)
    class GRPCGatewayErrorDetails(_message.Message):
        __slots__ = ("grpcCode", "httpCode", "message", "httpStatus")
        GRPCCODE_FIELD_NUMBER: _ClassVar[int]
        HTTPCODE_FIELD_NUMBER: _ClassVar[int]
        MESSAGE_FIELD_NUMBER: _ClassVar[int]
        HTTPSTATUS_FIELD_NUMBER: _ClassVar[int]
        grpcCode: int
        httpCode: int
        message: str
        httpStatus: str
        def __init__(self, grpcCode: _Optional[int] = ..., httpCode: _Optional[int] = ..., message: _Optional[str] = ..., httpStatus: _Optional[str] = ...) -> None: ...
    ERROR_FIELD_NUMBER: _ClassVar[int]
    error: GRPCGatewayError.GRPCGatewayErrorDetails
    def __init__(self, error: _Optional[_Union[GRPCGatewayError.GRPCGatewayErrorDetails, _Mapping]] = ...) -> None: ...

class GRPCGatewayWrapper(_message.Message):
    __slots__ = ("result",)
    RESULT_FIELD_NUMBER: _ClassVar[int]
    result: _trace_pb2.TracesData
    def __init__(self, result: _Optional[_Union[_trace_pb2.TracesData, _Mapping]] = ...) -> None: ...
