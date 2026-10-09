# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Fluent construction of the RFC 0005 structured query filter.

The filter that Jaeger's trace search accepts is an expression tree: a ``Call``
applying an operator to argument expressions, where an argument is a reference to
a value on the span, a constant, or another call. Written out by hand that tree is
verbose, so this module gives it Python syntax::

    (span.duration > "2s") & span.attr("http.status_code").one_of([500, 503])

The objects here build the tree and render it as the proto3 JSON that
``jaeger.expression.v1.Call`` expects. They perform no I/O and depend on nothing
outside the standard library; :mod:`jaeger_query.client` sends the result.

**The design is to make a wrong filter unbuildable rather than to validate one.**
The server validates, it owns those rules, and a copy of them here would only have
somewhere to drift. So this module spends its effort on shape instead: each level
exposes its built-in fields as real attributes, so a misspelled field raises
``AttributeError`` where it is written; only ``event`` and ``link`` can produce the
collection reference that ``some`` takes; and the two fields whose values have no
order carry no ordering operators. What is left for the server is the questions a
builder cannot answer, such as whether a backend indexes the level you asked for.

The vocabulary here mirrors ``query/expression/v1`` in jaeger-idl, which is the Go
counterpart of this module and the definition both follow.
"""

from __future__ import annotations

import json
from collections.abc import Iterable, Sequence

__all__ = [
    "AttributeRef",
    "Call",
    "Expr",
    "FieldRef",
    "ListValue",
    "NestedRef",
    "OrderedRef",
    "Scalar",
    "SpanKind",
    "SpanStatus",
    "ValueRef",
    "and_",
    "attr",
    "event",
    "link",
    "not_",
    "or_",
    "resource",
    "scope",
    "some",
    "span",
]

#: The kinds ``span.kind`` matches and the statuses ``span.status`` matches. Both
#: are closed sets in the data model, so they are named here rather than guessed.
SpanKind = ("unspecified", "internal", "server", "client", "producer", "consumer")
SpanStatus = ("unset", "ok", "error")


class Expr:
    """A node in the filter AST.

    Subclasses render themselves as one variant of the ``Expression`` oneof, so a
    node knows how to appear as an argument of a call.
    """

    __slots__ = ()

    def to_expression(self) -> dict:
        """Render as an ``Expression``, tagged with its oneof variant."""
        raise NotImplementedError


class ValueRef(Expr):
    """A reference to a single value, which an operator can compare.

    Ordering lives on :class:`OrderedRef` instead, so a field whose values have no
    order simply has no ``>`` to reach for.
    """

    __slots__ = ()

    def __eq__(self, other: object) -> Call:  # type: ignore[override]
        return self.eq(other)

    def __ne__(self, other: object) -> Call:  # type: ignore[override]
        return self.ne(other)

    def eq(self, value: object) -> Call:
        return self._compare("eq", value)

    def ne(self, value: object) -> Call:
        return self._compare("ne", value)

    def matches(self, pattern: str) -> Call:
        """Match the reference against an RE2 regular expression."""
        return Call("regex", [self, Scalar(pattern)])

    def exists(self) -> Call:
        """Match spans where the reference has a value."""
        return Call("exists", [self])

    def one_of(self, values: Iterable[object] | ListValue) -> Call:
        return self._member("in", values)

    def not_one_of(self, values: Iterable[object] | ListValue) -> Call:
        return self._member("not_in", values)

    def _compare(self, op: str, value: object) -> Call:
        return Call(op, [self, value if isinstance(value, Expr) else Scalar.of(value)])

    def _member(self, op: str, values: Iterable[object] | ListValue) -> Call:
        if isinstance(values, ListValue):
            return Call(op, [self, values])
        return Call(op, [self, ListValue.of(values)])


class OrderedRef(ValueRef):
    """A value reference whose values have an order, so ``>`` and its kin mean something."""

    __slots__ = ()

    def __gt__(self, other: object) -> Call:
        return self.gt(other)

    def __lt__(self, other: object) -> Call:
        return self.lt(other)

    def __ge__(self, other: object) -> Call:
        return self.gte(other)

    def __le__(self, other: object) -> Call:
        return self.lte(other)

    def gt(self, value: object) -> Call:
        return self._compare("gt", value)

    def lt(self, value: object) -> Call:
        return self._compare("lt", value)

    def gte(self, value: object) -> Call:
        return self._compare("gte", value)

    def lte(self, value: object) -> Call:
        return self._compare("lte", value)


class AttributeRef(OrderedRef):
    """An entry in one of the span's attribute maps, named by key (RFC 0005 §5.1).

    An empty level is the unqualified span-or-resource search. An attribute
    declares no type of its own, which is why a list compared against one has to
    declare the type of its elements.
    """

    __slots__ = ("key", "level")

    def __init__(self, key: str, level: str = "") -> None:
        self.key = key
        self.level = level

    def to_expression(self) -> dict:
        ref: dict = {"key": self.key}
        if self.level:
            ref["level"] = self.level
        return {"attr": ref}

    def __repr__(self) -> str:
        return f"AttributeRef(key={self.key!r}, level={self.level!r})"


class FieldRef(OrderedRef):
    """A built-in field of a level — a value the data model defines directly rather
    than an attribute-map entry (RFC 0005 §5.2).

    The field declares its own type, so a list compared against one needs none.
    Read these off their level rather than building them: ``span.duration``.
    """

    __slots__ = ("level", "name")

    def __init__(self, name: str, level: str) -> None:
        self.name = name
        self.level = level

    def to_expression(self) -> dict:
        return {"field": {"name": self.name, "level": self.level}}

    def __repr__(self) -> str:
        return f"FieldRef(name={self.name!r}, level={self.level!r})"


class UnorderedFieldRef(ValueRef):
    """A built-in field whose values are a closed set with no order — a span's kind
    or status. Comparing one with ``>`` asks nothing, so it has no ``>``.
    """

    __slots__ = ("level", "name")

    def __init__(self, name: str, level: str) -> None:
        self.name = name
        self.level = level

    def to_expression(self) -> dict:
        return {"field": {"name": self.name, "level": self.level}}

    def __repr__(self) -> str:
        return f"UnorderedFieldRef(name={self.name!r}, level={self.level!r})"


class NestedRef(Expr):
    """A span's events or links collection, which is what ``some`` quantifies over
    and the only place it may appear (RFC 0005 §5.5).

    Only :data:`event` and :data:`link` produce one, so ``some`` cannot be pointed
    at a level that holds a single element per span.
    """

    __slots__ = ("level",)

    def __init__(self, level: str) -> None:
        self.level = level

    def to_expression(self) -> dict:
        return {"nested": {"level": self.level}}

    def __repr__(self) -> str:
        return f"NestedRef(level={self.level!r})"


class Scalar(Expr):
    """A constant, carried as text with an optional type (RFC 0005 §5.4).

    The type stays empty unless a caller sets one. An omitted type means the
    backend matches the value at whatever type it was stored; a type that is set is
    *authoritative*, so declaring one narrows the match and a wrong guess matches
    nothing. A duration (``"2s"``) or a timestamp (RFC 3339) has no type of its own
    — it travels as text, and the field it is compared against gives it meaning.
    """

    __slots__ = ("type", "value")

    def __init__(self, value: str, type: str = "") -> None:
        self.value = value
        self.type = type

    @classmethod
    def of(cls, value: object) -> Scalar:
        """Build an untyped scalar from a Python value."""
        return cls(_render(value))

    def to_expression(self) -> dict:
        scalar: dict = {"value": self.value}
        if self.type:
            scalar["type"] = self.type
        return {"scalar": scalar}

    def __repr__(self) -> str:
        return f"Scalar(value={self.value!r}, type={self.type!r})"


class ListValue(Expr):
    """A homogeneous list constant, the right argument of ``in`` / ``not_in``.

    A built-in field supplies the type for a list compared against it. An attribute
    supplies no type, and the elements then match values of any stored type,
    exactly as an untyped scalar does. So :meth:`ValueRef.one_of` declares no type,
    and a caller who knows the type builds the list here instead. Declaring a type
    narrows the match, and beside an attribute a backend that indexes attribute
    values as text can honor only ``string``: it refuses a number or a boolean,
    having no typed value to search.

    Named ``ListValue`` here for the proto ``jaeger.expression.v1.List``, to keep
    the builtin ``list`` readable in this module.
    """

    __slots__ = ("type", "values")

    def __init__(self, values: Sequence[str], type: str = "") -> None:
        if not values:
            raise ValueError("a list constant needs a value: membership in nothing matches nothing")
        self.values = list(values)
        self.type = type

    @classmethod
    def of(cls, values: Iterable[object]) -> ListValue:
        """Build a list constant that declares no element type."""
        return cls([_render(v) for v in values])

    def to_expression(self) -> dict:
        constant: dict = {"values": list(self.values)}
        if self.type:
            constant["type"] = self.type
        return {"list": constant}

    def __repr__(self) -> str:
        return f"ListValue(values={self.values!r}, type={self.type!r})"


class Call(Expr):
    """An operator applied to argument expressions (RFC 0005 §6.1).

    Comparison, boolean combination and set membership are all this one node, so
    ``a AND b`` and ``span.startTime < span.endTime`` have the same shape.
    """

    __slots__ = ("args", "op")

    def __init__(self, op: str, args: Sequence[Expr]) -> None:
        self.op = op
        self.args = list(args)

    def to_expression(self) -> dict:
        return {"call": self.to_dict()}

    def to_dict(self) -> dict:
        """Render as a bare ``Call``, the form the ``filter`` field takes.

        The top-level filter is a ``Call``, not an ``Expression``, so it carries no
        oneof envelope; nested calls reached through ``args`` do.
        """
        return {"op": self.op, "args": [a.to_expression() for a in self.args]}

    def to_json(self) -> str:
        """Render as compact JSON, for the ``query.filter`` URL parameter."""
        return json.dumps(self.to_dict(), separators=(",", ":"))

    def __and__(self, other: Call) -> Call:
        return and_(self, other)

    def __or__(self, other: Call) -> Call:
        return or_(self, other)

    def __invert__(self) -> Call:
        return not_(self)

    def __repr__(self) -> str:
        return f"Call(op={self.op!r}, args={self.args!r})"


class Level:
    """One attribute level, holding the built-in fields the data model defines for
    it and reaching that level's attributes through :meth:`attr`.

    The fields are real attributes of the object, so a name the data model does not
    define fails where it is written rather than at the server.
    """

    __slots__ = ("level",)

    def __init__(self, level: str) -> None:
        self.level = level

    def attr(self, key: str) -> AttributeRef:
        """Name an attribute of this level."""
        return AttributeRef(key, self.level)

    def __call__(self, key: str) -> AttributeRef:
        """Name an attribute of this level — the terser spelling of :meth:`attr`."""
        return self.attr(key)

    def __repr__(self) -> str:
        return f"{type(self).__name__}({self.level!r})"


class CollectionLevel(Level):
    """A level holding many elements per span, so ``some`` can quantify over it."""

    __slots__ = ()

    def nested(self) -> NestedRef:
        """Name the whole collection, the first argument of :func:`some`."""
        return NestedRef(self.level)

    def some(self, predicate: Call) -> Call:
        """Match spans holding one element of this collection that satisfies ``predicate``.

        This is the quantifier of :func:`some`, reached from the collection it
        quantifies over. Only ``event`` and ``link`` have it, so a level holding a
        single element per span offers nothing to quantify.
        """
        return Call("some", [self.nested(), predicate])


class SpanLevel(Level):
    """The span level and its built-in fields."""

    __slots__ = (
        "duration",
        "endTime",
        "kind",
        "name",
        "parentSpanID",
        "spanID",
        "startTime",
        "status",
        "statusMessage",
        "traceID",
        "traceState",
    )

    def __init__(self) -> None:
        super().__init__("span")
        self.traceID = FieldRef("traceID", "span")
        self.spanID = FieldRef("spanID", "span")
        self.parentSpanID = FieldRef("parentSpanID", "span")
        self.traceState = FieldRef("traceState", "span")
        self.name = FieldRef("name", "span")
        self.startTime = FieldRef("startTime", "span")
        self.endTime = FieldRef("endTime", "span")
        self.duration = FieldRef("duration", "span")
        self.statusMessage = FieldRef("statusMessage", "span")
        self.kind = UnorderedFieldRef("kind", "span")
        self.status = UnorderedFieldRef("status", "span")


class ResourceLevel(Level):
    """The resource level and its built-in fields."""

    __slots__ = ("schemaURL", "service")

    def __init__(self) -> None:
        super().__init__("resource")
        self.service = FieldRef("service", "resource")
        self.schemaURL = FieldRef("schemaURL", "resource")


class ScopeLevel(Level):
    """The instrumentation scope level and its built-in fields."""

    __slots__ = ("name", "schemaURL", "version")

    def __init__(self) -> None:
        super().__init__("scope")
        self.name = FieldRef("name", "scope")
        self.version = FieldRef("version", "scope")
        self.schemaURL = FieldRef("schemaURL", "scope")


class EventLevel(CollectionLevel):
    """The event level and its built-in fields."""

    __slots__ = ("name", "time", "timeSinceStart")

    def __init__(self) -> None:
        super().__init__("event")
        self.name = FieldRef("name", "event")
        self.time = FieldRef("time", "event")
        self.timeSinceStart = FieldRef("timeSinceStart", "event")


class LinkLevel(CollectionLevel):
    """The link level and its built-in fields."""

    __slots__ = ("spanID", "traceID", "traceState")

    def __init__(self) -> None:
        super().__init__("link")
        self.traceID = FieldRef("traceID", "link")
        self.spanID = FieldRef("spanID", "link")
        self.traceState = FieldRef("traceState", "link")


span = SpanLevel()
resource = ResourceLevel()
scope = ScopeLevel()
event = EventLevel()
link = LinkLevel()


def attr(key: str) -> AttributeRef:
    """Name an unqualified attribute, searched at the span and resource levels.

    This is the level-free reference of RFC 0005 §5.1, and the closest equivalent
    of the legacy ``attributes`` map.
    """
    return AttributeRef(key)


def and_(*args: Call) -> Call:
    """Combine predicates conjunctively. A single predicate is returned as is."""
    return _boolean("and", args)


def or_(*args: Call) -> Call:
    """Combine predicates disjunctively. A single predicate is returned as is."""
    return _boolean("or", args)


def not_(arg: Call) -> Call:
    """Negate a predicate."""
    return Call("not", [arg])


def some(collection: CollectionLevel | NestedRef, predicate: Call) -> Call:
    """Match spans holding one event or link that satisfies ``predicate``.

    Without this quantifier a filter naming two event fields is uncorrelated: each
    conjunct may be satisfied by a different event. Inside ``some``, the references
    at the quantified level bind to a single element, so
    ``some(event, (event.name == "exception") & (event.timeSinceStart > "50us"))``
    asks for one event that is both (RFC 0005 §5.5).
    """
    if isinstance(collection, CollectionLevel):
        return collection.some(predicate)
    return Call("some", [collection, predicate])


def _boolean(op: str, args: Sequence[Call]) -> Call:
    """Combine predicates under one n-ary call.

    An argument that is already a call to the same operator is absorbed rather than
    nested, so ``a | b | c`` is one three-way ``or``. And and or are associative, so
    this only changes the shape of the tree, and the flatter shape is the one a
    backend restricted to a flat conjunction can read.
    """
    if not args:
        raise ValueError(f"{op} needs at least one predicate")
    flattened: list[Call] = []
    for a in args:
        flattened.extend(a.args if a.op == op else [a])  # type: ignore[arg-type]
    if len(flattened) == 1:
        return flattened[0]
    return Call(op, flattened)


def _render(value: object) -> str:
    """Render a Python value as the text a Scalar or List carries."""
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, str):
        return value
    if isinstance(value, (int, float)):
        return str(value)
    raise TypeError(f"cannot use {type(value).__name__} as a filter value")
