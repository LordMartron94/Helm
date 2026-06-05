# Helm 2.0 Migration Guide

## Overview

Helm 2.0 introduces **workspace mode** with distributed `entity` files while v1.7 **legacy mode** continues to work unchanged.

## Step 1 — Freeze a v1 baseline

Tag or branch your repository with the working v1 Helmfile (see vertex-siege `v1-build`). Document the property-bag contract your libraries export (`CPPFLAGS`, `LDFLAGS`, `HEADERS`).

## Step 2 — Add a workspace manifest

Create a root manifest (for example `Helmfile.v2`) with:

```helm
OBJ_DIR = "build/obj"
LIB_DIR = "build/lib"

workspace {
    globals {
        OBJ_DIR = OBJ_DIR
        LIB_DIR = LIB_DIR
    }
    exclude "build"
}
```

Entity helm files under `libs/` are discovered automatically; remove explicit `include` directives.

Keep the legacy `Helmfile` until parity tests pass.

## Step 3 — Move libraries to entity files

Replace per-library wrapper targets with one entity file per library:

```helm
entity splash {
    kind = "lib"
    use c_shared_library
    sources = [ glob("src", include="**/*.c", recursive=true) ]
    deps = [ "//libs/math:math" ]
    interface {
        CPPFLAGS = [ "-Ilibs/splash/src" ]
        LDFLAGS = [ "-lsplash" ]
    }
}
```

## Step 4 — Slim product targets

Root targets become action-only (`run_tests`, `deploy`, `generate_compilation_database`):

```helm
target run_tests() {
    help = "Run tests"
    depends_on [ "//libs/splash:splash" ]
    run "make test"
}
```

Targets in workspace mode **must not** declare `artifacts`.

## Step 5 — Parity gate

Before switching the default Helmfile:

1. Same binary or `.so` output from v1 and v2 builds
2. Same flattened property bag (`CPPFLAGS`, `LDFLAGS`) consumed by dependents
3. `helm export-graph` emits resolved `entities` with full `run_argvs`

## Dual-mode reference

| Mode | Trigger | Artifact production |
|------|---------|---------------------|
| Legacy | No `workspace` / `entity` | `target` + `artifacts` |
| Workspace | `workspace` and/or `entity` | `entity` + adapter only |

See [Helm 2.0 syntax](helm2-syntax.md) and [ADR 0002](adr/0002-dual-mode-legacy-adapter.md).
