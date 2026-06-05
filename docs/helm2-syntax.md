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

## Adapter (explicit sub-graph macro)

An adapter is a **macro** that expands into an ordered sub-graph of `target` phases. Each phase is a normal matrix/run/outputs node; link phases reference compile outputs with `param _compile.OUTPUTS`. Object paths use workspace mirroring: `${OBJ_DIR}/${SRC}.o` (no per-entity `_obj` bucket).

Compiler flags are strict argv arrays (`param APPLICATION_COMPILER_FLAGS`), not string-packed scalars.

```helm
adapter c_executable(SOURCE_FILES, OUT_NAME, CPPFLAGS, LDFLAGS, DEPENDENCIES?) {
    target _compile {
        matrix SRC in SOURCE_FILES
        outputs = [ "${OBJ_DIR}/${SRC}.o" ]
        run [
            "tools/scripts/compile_object.sh",
            "${SRC}",
            "${OBJ_DIR}/${SRC}.o",
            "${OBJ_DIR}/${SRC}.d",
            param APPLICATION_COMPILER_FLAGS,
            param CPPFLAGS,
            collect(DEPENDENCIES, "CPPFLAGS"),
        ]
    }

    target _link {
        depends_on [ _compile ]
        outputs = [ "${BIN_DIR}/${OUT_NAME}" ]
        run [
            "tools/scripts/link_objects.sh",
            "${BIN_DIR}/${OUT_NAME}",
            param _compile.OUTPUTS,
            param LDFLAGS,
            collect(DEPENDENCIES, "LDFLAGS"),
        ]
    }
}
```

**Entity inputs vs exports:** adapter parameters (`CPPFLAGS`, `LDFLAGS`) are **inputs** passed in `use <adapter> { params { ... } }`. The `interface { }` block on libraries only declares **exports** for downstream dependents. Binary entities (`kind = "bin"`) cannot declare an `interface` block.

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
