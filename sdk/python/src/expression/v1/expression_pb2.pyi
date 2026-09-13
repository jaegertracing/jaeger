from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Expression(_message.Message):
    __slots__ = ("attr", "field", "nested", "scalar", "list", "call")
    ATTR_FIELD_NUMBER: _ClassVar[int]
    FIELD_FIELD_NUMBER: _ClassVar[int]
    NESTED_FIELD_NUMBER: _ClassVar[int]
    SCALAR_FIELD_NUMBER: _ClassVar[int]
    LIST_FIELD_NUMBER: _ClassVar[int]
    CALL_FIELD_NUMBER: _ClassVar[int]
    attr: AttributeReference
    field: FieldReference
    nested: NestedReference
    scalar: Scalar
    list: List
    call: Call
    def __init__(self, attr: _Optional[_Union[AttributeReference, _Mapping]] = ..., field: _Optional[_Union[FieldReference, _Mapping]] = ..., nested: _Optional[_Union[NestedReference, _Mapping]] = ..., scalar: _Optional[_Union[Scalar, _Mapping]] = ..., list: _Optional[_Union[List, _Mapping]] = ..., call: _Optional[_Union[Call, _Mapping]] = ...) -> None: ...

class AttributeReference(_message.Message):
    __slots__ = ("key", "level")
    KEY_FIELD_NUMBER: _ClassVar[int]
    LEVEL_FIELD_NUMBER: _ClassVar[int]
    key: str
    level: str
    def __init__(self, key: _Optional[str] = ..., level: _Optional[str] = ...) -> None: ...

class FieldReference(_message.Message):
    __slots__ = ("name", "level")
    NAME_FIELD_NUMBER: _ClassVar[int]
    LEVEL_FIELD_NUMBER: _ClassVar[int]
    name: str
    level: str
    def __init__(self, name: _Optional[str] = ..., level: _Optional[str] = ...) -> None: ...

class NestedReference(_message.Message):
    __slots__ = ("level",)
    LEVEL_FIELD_NUMBER: _ClassVar[int]
    level: str
    def __init__(self, level: _Optional[str] = ...) -> None: ...

class Scalar(_message.Message):
    __slots__ = ("value", "type")
    VALUE_FIELD_NUMBER: _ClassVar[int]
    TYPE_FIELD_NUMBER: _ClassVar[int]
    value: str
    type: str
    def __init__(self, value: _Optional[str] = ..., type: _Optional[str] = ...) -> None: ...

class List(_message.Message):
    __slots__ = ("values", "type")
    VALUES_FIELD_NUMBER: _ClassVar[int]
    TYPE_FIELD_NUMBER: _ClassVar[int]
    values: _containers.RepeatedScalarFieldContainer[str]
    type: str
    def __init__(self, values: _Optional[_Iterable[str]] = ..., type: _Optional[str] = ...) -> None: ...

class Call(_message.Message):
    __slots__ = ("op", "args")
    OP_FIELD_NUMBER: _ClassVar[int]
    ARGS_FIELD_NUMBER: _ClassVar[int]
    op: str
    args: _containers.RepeatedCompositeFieldContainer[Expression]
    def __init__(self, op: _Optional[str] = ..., args: _Optional[_Iterable[_Union[Expression, _Mapping]]] = ...) -> None: ...
