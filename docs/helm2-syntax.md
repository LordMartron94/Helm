# Helm 2.0 Syntax Reference

Helm 2.0 adds **workspace mode**: distributed entity files, property-bag interfaces, and strict segregation from procedural targets.

## Dual-mode execution

| Mode | Trigger | Targets may use `artifacts` | Entities may use `run` |
|------|---------|----------------------------|------------------------|
| Legacy (v1.7) | No `workspace` / `entity` | Yes | N/A |
| Workspace (2.0) | `workspace { }` and/or `entity` | No (`TARGET_024`) | No (`ENTITY_001`) |

Legacy Helmfiles continue to work unchanged. See [ADR 0002](adr/0002-dual-mode-legacy-adapter.md) and the [migration guide](helm2-migration.md).

## Workspace manifest

The root Helmfile declares `workspace { }`. All `*.helm` files under the workspace root are loaded automatically (except the root manifest itself). Use `exclude` to skip directories or individual helm files.

Top-level variables in the root Helmfile are **file-local** (visible to adapters and targets in that file). Only variables declared in `globals { }` are injected into discovered entity files. Workspace globals accept string literals, artifact arrays, or bare variable references to file-local names (e.g. `OBJ_DIR = OBJ_DIR`).

```helm
OBJ_DIR = "build/obj"
LIB_DIR = "build/lib"

workspace {
    globals {
        OBJ_DIR = OBJ_DIR
        LIB_DIR = LIB_DIR
    }
    exclude "build"
    exclude "tools/helm/tests"
}
```

## Entity (artifact producer)

```helm
entity splash {
    kind = "lib"
    use c_shared_library
    sources = [
        glob("src", include="**/*.c", recursive=true),
    ]
    deps = [ "//libs/math:math" ]
    interface {
        CPPFLAGS = [ "-Ilibs/splash/src" ]
        LDFLAGS = [ "-lsplash" ]
    }
}
```

Entities **must** declare `use <adapter>`. They cannot declare `run` or `artifacts`.

## Labels

Cross-file references use `//path/to/dir:name` or `//path/to/dir` (default name = directory basename).

```helm
depends_on [ "//libs/splash:splash" ]
deps = [ "//libs/math:math" ]
```

## Interface (property-bag keys only)

```helm
interface c_link {
    keys = [ "CPPFLAGS", "LDFLAGS", "HEADERS" ]
}
```

Interfaces declare **key names**, not types. The engine flattens bags on `collect()`; Clang/linker validate flags.

## Adapter (dumb argv templates)

Adapters declare **parameters** (same syntax as targets), **`run [ ... ]` argv templates**, and optional `matrix`, `env`, `outputs`. The engine only binds parameters and spawns — it does not know C, compilers, or `.so` vs binaries.

```helm
adapter c_shared_library(SOURCE_FILES, OUT_NAME, DEPENDENCIES?) {
    matrix SRC in SOURCE_FILES

    outputs = [ "${OBJ_DIR}/${OUT_NAME}_obj/${SRC}.o" ]

    run [
        "tools/scripts/compile_object.sh",
        "${SRC}",
        "${OBJ_DIR}/${OUT_NAME}_obj/${SRC}.o",
        "${OBJ_DIR}/${OUT_NAME}_obj/${SRC}.d",
        param LIBRARY_COMPILER_FLAGS,
        param CPPFLAGS,
        collect(DEPENDENCIES, "CPPFLAGS"),
    ]

    env {
        LDFLAGS = [ param LDFLAGS, collect(DEPENDENCIES, "LDFLAGS") ]
    }

    outputs = [ "${LIB_DIR}/lib${OUT_NAME}.so" ]

    run [
        "tools/scripts/link_objects.sh",
        "${LIB_DIR}/lib${OUT_NAME}.so",
        param MATRIX_OUTPUTS,
    ]
}
```

## Entity (binds adapter parameters)

```helm
entity splash {
    use c_shared_library {
        params {
            SOURCE_FILES = [
                glob("src", include="**/*.c"),
            ]
            OUT_NAME = "splash"
        }
    }
    interface {
        CPPFLAGS = [ "-Ilibs/splash/src" ]
        LDFLAGS = [ "-lsplash" ]
    }
}
```

`interface` keys are available as `param` references in adapter templates (property bag for dependents). `collect(DEPENDENCIES, "KEY")` flattens bags from `deps = [ ... ]` entity labels.

## Action targets (workspace mode)

```helm
target run_tests() {
    depends_on [ "//libs/splash:splash" ]
    run "${BIN_DIR}/testbed"
}
```

Targets run actions only. No `artifacts` block in workspace mode.

## Migration

1. Freeze baseline on `v1-build` branch ([contract](../../../vertex-siege/docs/helm-v1-build-contract.md))
2. Add `Helmfile.v2` workspace manifest
3. Move libraries to `libs/<name>/<name>.helm`
4. Parity-test binary + property bag
5. Replace root `Helmfile` when parity holds
