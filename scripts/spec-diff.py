#!/usr/bin/env python3
"""Diff two OpenAPI specs and report added/removed paths, fields, and enum values.

Used by .github/workflows/spec-drift-check.yml to detect when the upstream
platform's public API has drifted from the spec vendored at apis/*.yaml.
Exits 0 with no differences, 1 if any drift was found.
"""
import argparse
import sys

import yaml

METHODS = {"get", "post", "put", "patch", "delete"}


def load_spec(path):
    with open(path) as f:
        return yaml.safe_load(f)


def schema_leaf_fields(schema, prefix=""):
    fields = set()
    if not isinstance(schema, dict):
        return fields
    for name, sub in (schema.get("properties") or {}).items():
        full = f"{prefix}.{name}" if prefix else name
        fields.add(full)
        fields |= schema_leaf_fields(sub, full)
    for key in ("allOf", "oneOf", "anyOf"):
        for sub in schema.get(key) or []:
            fields |= schema_leaf_fields(sub, prefix)
    items = schema.get("items")
    if items:
        fields |= schema_leaf_fields(items, prefix)
    return fields


def enum_values(schema, prefix=""):
    values = {}
    if not isinstance(schema, dict):
        return values
    if "enum" in schema:
        values[prefix] = set(schema["enum"])
    for name, sub in (schema.get("properties") or {}).items():
        full = f"{prefix}.{name}" if prefix else name
        values.update(enum_values(sub, full))
    for key in ("allOf", "oneOf", "anyOf"):
        for sub in schema.get(key) or []:
            values.update(enum_values(sub, prefix))
    items = schema.get("items")
    if items:
        values.update(enum_values(items, prefix))
    return values


def diff_paths(old, new):
    old_paths = set((old.get("paths") or {}).keys())
    new_paths = set((new.get("paths") or {}).keys())
    added = sorted(new_paths - old_paths)
    removed = sorted(old_paths - new_paths)
    method_changes = []
    for p in sorted(old_paths & new_paths):
        old_methods = {k.lower() for k in (old["paths"][p] or {}) if k.lower() in METHODS}
        new_methods = {k.lower() for k in (new["paths"][p] or {}) if k.lower() in METHODS}
        added_m = sorted(new_methods - old_methods)
        removed_m = sorted(old_methods - new_methods)
        if added_m or removed_m:
            method_changes.append((p, added_m, removed_m))
    return added, removed, method_changes


def diff_schemas(old, new):
    old_schemas = (old.get("components") or {}).get("schemas") or {}
    new_schemas = (new.get("components") or {}).get("schemas") or {}
    added_schemas = sorted(set(new_schemas) - set(old_schemas))
    removed_schemas = sorted(set(old_schemas) - set(new_schemas))
    field_changes = []
    enum_changes = []
    for name in sorted(set(old_schemas) & set(new_schemas)):
        old_fields = schema_leaf_fields(old_schemas[name])
        new_fields = schema_leaf_fields(new_schemas[name])
        added_f = sorted(new_fields - old_fields)
        removed_f = sorted(old_fields - new_fields)
        if added_f or removed_f:
            field_changes.append((name, added_f, removed_f))

        old_enums = enum_values(old_schemas[name])
        new_enums = enum_values(new_schemas[name])
        for field in sorted(set(old_enums) | set(new_enums)):
            old_vals = old_enums.get(field, set())
            new_vals = new_enums.get(field, set())
            added_v = sorted(new_vals - old_vals)
            removed_v = sorted(old_vals - new_vals)
            if added_v or removed_v:
                label = f"{name}.{field}" if field else name
                enum_changes.append((label, added_v, removed_v))
    return added_schemas, removed_schemas, field_changes, enum_changes


def main():
    parser = argparse.ArgumentParser(description="Diff two OpenAPI specs and report drift.")
    parser.add_argument("old", help="Path to the currently committed spec")
    parser.add_argument("new", help="Path to the candidate/new spec")
    parser.add_argument("--format", choices=["markdown"], default="markdown")
    args = parser.parse_args()

    old = load_spec(args.old)
    new = load_spec(args.new)

    old_version = (old.get("info") or {}).get("version", "unknown")
    new_version = (new.get("info") or {}).get("version", "unknown")

    added_paths, removed_paths, method_changes = diff_paths(old, new)
    added_schemas, removed_schemas, field_changes, enum_changes = diff_schemas(old, new)

    has_drift = any([
        old_version != new_version,
        added_paths, removed_paths, method_changes,
        added_schemas, removed_schemas, field_changes, enum_changes,
    ])

    lines = ["# API spec drift report", "", f"- Spec version: `{old_version}` -> `{new_version}`", ""]

    if not has_drift:
        lines.append("No differences detected between the committed spec and the candidate spec.")
        print("\n".join(lines))
        return 0

    if added_paths:
        lines.append("## New paths")
        lines += [f"- `{p}`" for p in added_paths]
        lines.append("")
    if removed_paths:
        lines.append("## Removed paths")
        lines += [f"- `{p}`" for p in removed_paths]
        lines.append("")
    if method_changes:
        lines.append("## Changed methods on existing paths")
        for p, added_m, removed_m in method_changes:
            if added_m:
                lines.append(f"- `{p}`: added `{', '.join(m.upper() for m in added_m)}`")
            if removed_m:
                lines.append(f"- `{p}`: removed `{', '.join(m.upper() for m in removed_m)}`")
        lines.append("")
    if added_schemas:
        lines.append("## New schemas")
        lines += [f"- `{s}`" for s in added_schemas]
        lines.append("")
    if removed_schemas:
        lines.append("## Removed schemas")
        lines += [f"- `{s}`" for s in removed_schemas]
        lines.append("")
    if field_changes:
        lines.append("## Changed fields on existing schemas")
        for name, added_f, removed_f in field_changes:
            if added_f:
                lines.append(f"- `{name}`: new field(s) `{', '.join(added_f)}`")
            if removed_f:
                lines.append(f"- `{name}`: removed field(s) `{', '.join(removed_f)}`")
        lines.append("")
    if enum_changes:
        lines.append("## Changed enum values")
        for name, added_v, removed_v in enum_changes:
            if added_v:
                lines.append(f"- `{name}`: new value(s) `{', '.join(added_v)}`")
            if removed_v:
                lines.append(f"- `{name}`: removed value(s) `{', '.join(removed_v)}`")
        lines.append("")

    print("\n".join(lines))
    return 1


if __name__ == "__main__":
    sys.exit(main())
