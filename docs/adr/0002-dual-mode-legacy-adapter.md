# Dual-Mode Engine and Legacy Adapter

STATUS: accepted

DECISION: Helm 2.0 engine runs in legacy mode for v1.7-only files and workspace mode when a `workspace` block or `entity` declarations are present.

DATE: 2026-06-05

## RATIONALE

vertex-siege must keep building during the Helm 2.0 transition. A hard cutover would stall development for months.

## Modes

| Mode | Trigger | Target `artifacts` | Entity `run` | Execution |
|------|---------|-------------------|--------------|-----------|
| **Legacy** | No `workspace` block and no `entity` blocks | Allowed | N/A | Existing target executor |
| **Workspace** | `workspace { }` and/or `entity` blocks | Forbidden (`TARGET_024`) | Forbidden (`ENTITY_001`) | Native entity executor + action targets |

## Legacy Adapter

v1.7 targets with `artifacts`, `export`, and `collect()` run unchanged through the existing
[`targetexecutor`](../../internal/targetexecutor/) pipeline. No rewrite required.

Workspace roots may `include` entity files while product targets remain v2 action targets
(`run_tests`, `deploy`) that `depends_on` entity labels.

## Migration path

1. Freeze `v1-build` on vertex-siege
2. Add workspace + entity files alongside legacy Helmfile sections
3. Parity-test binary + property bag
4. Remove legacy wrapper targets when parity holds
