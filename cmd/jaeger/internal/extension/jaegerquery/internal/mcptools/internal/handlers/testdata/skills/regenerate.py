# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Trim a captured HotROD trace into minimal skill fixtures.

Reads the OTLP JSON returned by Jaeger's v3 API and emits, for each fixture, the
smallest span set that still exercises the pattern: the sibling group, its
parent, and the ancestry back to the trace root so the trace stays well formed.
"""
import json
import collections
import sys

src = sys.argv[1]
outdir = sys.argv[2]

docs = [json.loads(l) for l in open(src) if l.strip()]

# Flatten while remembering which resource each span came from, so the trimmed
# output can be regrouped into the same resourceSpans shape.
resources = []          # list of (resource, scope)
spans = []              # list of (resource_index, span, service_name)
res_index = {}
for d in docs:
    for rs in d.get("result", {}).get("resourceSpans", []):
        svc = next(a["value"]["stringValue"] for a in rs["resource"]["attributes"]
                   if a["key"] == "service.name")
        for ss in rs["scopeSpans"]:
            key = json.dumps(rs["resource"], sort_keys=True) + "|" + json.dumps(ss.get("scope", {}), sort_keys=True)
            if key not in res_index:
                res_index[key] = len(resources)
                resources.append((rs["resource"], ss.get("scope", {}), svc))
            for s in ss["spans"]:
                spans.append((res_index[key], s, svc))

by_trace = collections.defaultdict(list)
for ri, s, svc in spans:
    by_trace[s["traceId"]].append((ri, s, svc))


def is_err(s):
    return s.get("status", {}).get("code") in (2, "STATUS_CODE_ERROR")


def groups(items):
    g = collections.defaultdict(list)
    for _, s, svc in items:
        g[(s.get("parentSpanId", ""), svc, s["name"])].append(s)
    return g


used_traces = set()


def pick(pred):
    """First trace whose sibling groups satisfy pred, largest first.

    Fixtures must come from distinct traces: two files sharing a trace ID would
    collide in a store keyed by it.
    """
    for tid, items in sorted(by_trace.items(), key=lambda kv: -len(kv[1])):
        if tid in used_traces:
            continue
        g = groups(items)
        hit = pred(g, items)
        if hit:
            used_traces.add(tid)
            return tid, items, hit
    return None, None, None


def ancestry(items, span_ids):
    """Span ids plus every ancestor, so the trimmed trace has no orphans."""
    idx = {s["spanId"]: s for _, s, _ in items}
    keep = set(span_ids)
    frontier = list(span_ids)
    while frontier:
        sid = frontier.pop()
        p = idx.get(sid, {}).get("parentSpanId", "")
        if p and p in idx and p not in keep:
            keep.add(p)
            frontier.append(p)
    return keep


def emit(items, keep, path):
    """Rebuild OTLP JSON containing only `keep`, preserving resource grouping."""
    buckets = collections.defaultdict(list)
    for ri, s, _ in items:
        if s["spanId"] in keep:
            buckets[ri].append(s)
    out = {"resourceSpans": []}
    for ri, ss in sorted(buckets.items()):
        resource, scope, _ = resources[ri]
        entry = {"resource": resource, "scopeSpans": [{"spans": sorted(ss, key=lambda x: x["startTimeUnixNano"])}]}
        if scope:
            entry["scopeSpans"][0]["scope"] = scope
        out["resourceSpans"].append(entry)
    with open(path, "w") as f:
        json.dump(out, f, indent=2)
        f.write("\n")
    return sum(len(v) for v in buckets.values())


def timing(grp):
    grp = sorted(grp, key=lambda x: int(x["startTimeUnixNano"]))
    ov = sum(1 for a, b in zip(grp, grp[1:])
             if int(b["startTimeUnixNano"]) < int(a["endTimeUnixNano"]))
    wall = (max(int(x["endTimeUnixNano"]) for x in grp) - min(int(x["startTimeUnixNano"]) for x in grp)) / 1e6
    total = sum(int(x["endTimeUnixNano"]) - int(x["startTimeUnixNano"]) for x in grp) / 1e6
    return ov, wall, total


# --- positive: serial GetDriver siblings, none of them errored -------------
def serial_getdriver(g, items):
    for (pid, svc, name), grp in g.items():
        if name != "GetDriver" or len(grp) < 10:
            continue
        # HotROD fails a GetDriver call on every dispatch and retries it, so an
        # error-free group does not exist. The retries stay in: they are what a
        # real N+1 looks like.
        ov, wall, total = timing(grp)
        if ov == 0 and total / wall > 0.9:
            return (pid, svc, name, grp)
    return None


# --- negative: overlapping route calls -------------------------------------
def parallel_routes(g, items):
    for (pid, svc, name), grp in g.items():
        if svc != "frontend" or len(grp) < 8:
            continue
        ov, wall, total = timing(grp)
        if ov >= 5 and total > wall:
            return (pid, svc, name, grp)
    return None


report = {}
for label, pred, fname in (
    ("n-plus-one-positive", serial_getdriver, "n_plus_one_positive.json"),
    ("n-plus-one-near-miss", parallel_routes, "n_plus_one_near_miss.json"),
):
    tid, items, hit = pick(pred)
    if not hit:
        print(f"!! no trace matched {label}")
        continue
    pid, svc, name, grp = hit
    keep = ancestry(items, {s["spanId"] for s in grp} | {pid})
    n = emit(items, keep, f"{outdir}/{fname}")
    ov, wall, total = timing(grp)
    report[label] = dict(trace_id=tid, parent_span_id=pid, service=svc, operation=name,
                         siblings=len(grp), overlaps=ov, wall_ms=round(wall, 1),
                         sum_ms=round(total, 1), spans_kept=n, file=fname)
    print(f"{label}: trace={tid[:16]}… parent={pid} {svc}/{name} n={len(grp)} "
          f"overlaps={ov} wall={wall:.1f}ms sum={total:.1f}ms -> {n} spans")

json.dump(report, open(f"{outdir}/report.json", "w"), indent=2)
