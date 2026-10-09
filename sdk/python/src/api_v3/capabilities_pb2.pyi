from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class FilterCapabilities(_message.Message):
    __slots__ = ("levels", "operators")
    LEVELS_FIELD_NUMBER: _ClassVar[int]
    OPERATORS_FIELD_NUMBER: _ClassVar[int]
    levels: _containers.RepeatedScalarFieldContainer[str]
    operators: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, levels: _Optional[_Iterable[str]] = ..., operators: _Optional[_Iterable[str]] = ...) -> None: ...

class SearchCapabilities(_message.Message):
    __slots__ = ("without_service_name", "same_span_conjunction", "filter", "paginated", "span_search", "span_sorting")
    WITHOUT_SERVICE_NAME_FIELD_NUMBER: _ClassVar[int]
    SAME_SPAN_CONJUNCTION_FIELD_NUMBER: _ClassVar[int]
    FILTER_FIELD_NUMBER: _ClassVar[int]
    PAGINATED_FIELD_NUMBER: _ClassVar[int]
    SPAN_SEARCH_FIELD_NUMBER: _ClassVar[int]
    SPAN_SORTING_FIELD_NUMBER: _ClassVar[int]
    without_service_name: bool
    same_span_conjunction: bool
    filter: FilterCapabilities
    paginated: bool
    span_search: bool
    span_sorting: bool
    def __init__(self, without_service_name: _Optional[bool] = ..., same_span_conjunction: _Optional[bool] = ..., filter: _Optional[_Union[FilterCapabilities, _Mapping]] = ..., paginated: _Optional[bool] = ..., span_search: _Optional[bool] = ..., span_sorting: _Optional[bool] = ...) -> None: ...

class GetCapabilitiesRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetCapabilitiesResponse(_message.Message):
    __slots__ = ("search",)
    SEARCH_FIELD_NUMBER: _ClassVar[int]
    search: SearchCapabilities
    def __init__(self, search: _Optional[_Union[SearchCapabilities, _Mapping]] = ...) -> None: ...
