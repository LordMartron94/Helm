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

## Configuration profiles

Root-level `configuration` blocks declare named global override profiles (paths, toolchain scripts, compiler flags). Entry targets select the active profile with `configuration = "name"`.

Entities use their default `use` / `deps` for every workspace profile unless they declare an explicit `configuration <name> { ... }` override (e.g. platform-specific `CPPFLAGS`). Top-level `use` / `deps` populate an implicit `default` entity variant.

```helm
ANDROID_OBJ_DIR = "build/android/obj"

configuration android {
    globals {
        OBJ_DIR = ANDROID_OBJ_DIR
        LIB_DIR = ANDROID_LIB_DIR
    }
}

entity nexus {
    use c_static_library { params { ... } }
    interface { CPPFLAGS = [ "-Ilibs/nexus" ] }
    deps = [ "//external/stb:stb" ]
}

target build_testbed_android() {
    help = "Build testbed for Android"
    configuration = "android"
    depends_on [ "//testbed:testbed" ]
    run "build/android/aarch64/bin/testbed"
}
```

Entity labels may include an `@configuration` suffix (`//libs/nexus:nexus@android`). Target `depends_on` entity edges accept `{ configuration = "android" }` options. The execution graph keys instances as `//path:name@configuration`.

When an entry target runs, Helm collects **all entity labels reachable through transitive target `depends_on` edges** (for example `run_app` → `build_app` → `//libs/splash:splash`), not only entity labels declared directly on the entry target.

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

Entities with compiled artifacts **must** declare `use <adapter>`. Metadata-only entities (`kind = "interface"` or `kind = "header_only"`) propagate property bags without an adapter or build steps.

### Metadata-only entity (header-only / interface library)

```helm
entity nexus {
    kind = "interface"
    interface {
        CPPFLAGS = [ "-Ilibs/nexus/include" ]
        HEADERS  = [ "libs/nexus/include/nexus.h" ]
    }
}
```

Metadata-only entities appear in the dependency graph and export-graph with an empty `run_argvs` list. Dependents collect their `interface` values via `deps = [ ... ]` and `collect(DEPENDENCIES, "CPPFLAGS")`. No stamp files or stub compilation is required.

Compiled entities cannot declare `run` or `artifacts`.

## Labels

Cross-file references use `//path/to/dir:name` or `//path/to/dir` (default name = directory basename). Append `@configuration` to select a configured instance (`//libs/nexus:nexus@android`).

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
            "-L${LIB_DIR}",
            "-Wl,-rpath,$ORIGIN/../lib",
            param _compile.OUTPUTS,
            param LDFLAGS,
            collect(DEPENDENCIES, "LDFLAGS"),
        ]
    }
}
```

Workspace library search paths and executable rpath belong in the **adapter**, not in each binary entity. Entity `LDFLAGS` are only for entity-specific flags (e.g. `-pthread`, `-lm`).

**Entity inputs, exports, and usage:** adapter parameters (`CPPFLAGS`, `LDFLAGS`) are **local inputs** passed in `use <adapter> { params { ... } }`. They apply only to that entity's own adapter expansion. The `interface { }` block on libraries declares **exports** for downstream dependents. The `usage { }` block declares **requirements** propagated to dependency entities when building from a root consumer (transitive down the dep graph). Binary entities (`kind = "bin"`) cannot declare an `interface` block but may declare `usage`.

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

```helm
entity testbed {
    kind = "bin"
    use c_executable {
        params {
            SOURCE_FILES = [ glob(".", include="**/*.c", recursive=true) ]
            OUT_NAME = "testbed"
            CPPFLAGS = [ "-I." ]
        }
    }
    usage {
        CPPFLAGS = [ "-DECHO_MAX_SYSTEM_LABEL_LENGTH=15" ]
    }
    deps = [ "//libs/echo:echo" ]
}
```

`interface` declares **exports** for downstream dependents only; values are not applied to the declaring entity's own adapter expansion. Dependents receive them via `collect(DEPENDENCIES, "KEY")`, which flattens bags from `deps = [ ... ]` entity labels. `usage` declares requirements for **dependencies** in the active build closure; the engine merges usage into each dependency's adapter parameters (after local `params`, before `collect(DEPENDENCIES, ...)`). Adapter inputs (`CPPFLAGS`, `LDFLAGS`, etc.) must be passed in `use <adapter> { params { ... } }`.

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
