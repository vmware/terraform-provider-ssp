#!/usr/bin/env python3
"""Render docs/guides/version-compatibility.md and README.md's generated
compatibility section from internal/compat/compatibility.yaml.

Run via `make docs-compat`. CI (.github/workflows/docs-lint.yml) re-runs this
and fails the build if the committed docs don't match, so
internal/compat/compatibility.yaml stays the single source of truth instead
of README.md/docs/guides drifting from it by hand.
"""
import pathlib
import sys

import yaml

ROOT = pathlib.Path(__file__).resolve().parent.parent
COMPAT_PATH = ROOT / "internal" / "compat" / "compatibility.yaml"
GUIDE_PATH = ROOT / "docs" / "guides" / "version-compatibility.md"
README_PATH = ROOT / "README.md"

BEGIN_MARKER = "<!-- BEGIN GENERATED: version-compatibility (see internal/compat/compatibility.yaml, scripts/gen-compat-docs.py) -->"
END_MARKER = "<!-- END GENERATED: version-compatibility -->"


def render_guide(compat, provider_name):
    lines = [
        "---",
        f'page_title: "{provider_name}: Version Compatibility"',
        "---",
        "",
        "# Version Compatibility",
        "",
        "This page is generated from `internal/compat/compatibility.yaml` by "
        "`scripts/gen-compat-docs.py` -- edit that file, then run `make docs-compat`, "
        "rather than editing this page directly.",
        "",
        f"- Provider version: `{compat['provider_version']}`",
        f"- Minimum supported platform version: `{compat['platform']['min_supported']}`",
        f"- Tested against platform version: `{compat['platform']['tested_against']}`",
        "",
    ]
    attrs = compat.get("attributes") or []
    if not attrs:
        lines.append(
            "No resource attribute currently has a documented minimum-platform-version "
            "requirement beyond the baseline above."
        )
    else:
        lines += [
            "## Attribute-level requirements",
            "",
            "| Resource | Attribute | Introduced in | Notes |",
            "|---|---|---|---|",
        ]
        for a in attrs:
            lines.append(
                f"| `{a['resource']}` | `{a['attribute']}` | `{a['introduced_in']}` | {a.get('description', '')} |"
            )
    return "\n".join(lines) + "\n"


def render_readme_section(compat):
    return (
        f"{BEGIN_MARKER}\n"
        f"**Platform compatibility:** tested against platform version "
        f"`{compat['platform']['tested_against']}`, minimum supported "
        f"`{compat['platform']['min_supported']}`. See "
        f"[Version Compatibility](docs/guides/version-compatibility.md) for details.\n"
        f"{END_MARKER}"
    )


def update_readme(compat):
    text = README_PATH.read_text()
    if BEGIN_MARKER not in text or END_MARKER not in text:
        raise SystemExit(
            f"README.md is missing the version-compatibility markers "
            f"({BEGIN_MARKER!r} / {END_MARKER!r}) -- add them once before this "
            "generator can maintain the section."
        )
    start = text.index(BEGIN_MARKER)
    end = text.index(END_MARKER) + len(END_MARKER)
    text = text[:start] + render_readme_section(compat) + text[end:]
    README_PATH.write_text(text)


def main():
    provider_name = sys.argv[1] if len(sys.argv) > 1 else "provider"
    compat = yaml.safe_load(COMPAT_PATH.read_text())
    GUIDE_PATH.parent.mkdir(parents=True, exist_ok=True)
    GUIDE_PATH.write_text(render_guide(compat, provider_name))
    update_readme(compat)


if __name__ == "__main__":
    main()
