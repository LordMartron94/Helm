# Helm Syntax Reference

Helm operates on a hybrid execution philosophy: **At the macro level, it is a declarative Directed Acyclic Graph (DAG) orchestrator. At the micro (target) level, it is a procedural task runner.** This reference outlines the lexical and structural rules of the `.helm` language.

---

## 1. File Structure & Global Variables

A `.helm` file consists of global variable declarations and target definitions. 
* **Comments:** `//` for single-line, `/* */` for multiline blocks.
* **Whitespace:** Newlines are structurally significant to terminate statements. Semicolons are not used.

### Global Variables
Variables are declared at the root level and can be interpolated into strings.
```helm
BUILD_DIR = "bin"
VERSION = "1.0.0"
```

---

## 2. Target Anatomy

A `target` is the atomic node of the Helm dependency graph. It consists of a signature, configuration properties, state boundaries, edges, and execution steps.

### Target Signature
Targets can accept parameters. Parameters can be marked as optional using `?`.
```helm
target build(OS, ARCH?) {
    help = "Builds the application for the specified OS and architecture."
    aliases = ["b", "compile"]
}
```

### Execution Context
Isolate the shell execution environment without polluting the `run` strings.
```helm
target test() {
    workdir = "./src"

    env {
        GO_ENV = "testing"
        CGO_ENABLED = "1"
    }
}
```

### Interactive targets (`interactive`)

Use `interactive = true` when a target must control the host terminal for its entire run (REPLs, debuggers, prompts). Helm wires `stdin`, `stdout`, and `stderr` directly to the child process instead of capturing output in buffers.

```helm
target shield() {
    help = "Launch the SHIELD REPL"

    interactive = true

    depends_on [ build_shield ]

    artifacts {
        volatile = true
    }

    run "./build/dev-bin/shield_cli"
}
```

* **`interactive = true`**: Claims the host TTY for all `run` steps on this target. Cache lookup is skipped (the target always runs when reached, like `volatile`).
* **Incompatible with `matrix`**: Matrix legs run in parallel; interactive targets cannot share a matrix block (`TARGET_019`).
* **Parallel phases**: An interactive target must be the only target in its DAG execution phase. Helm fails with `EXEC_002` if another target would run in parallel in the same phase.
* **Typical usage**: Declare `interactive` on a leaf-style entry target (e.g. `helm run shield`) after build dependencies have finished in earlier phases.

### Hidden targets (`hidden`)

Use `hidden = true` to keep a target runnable but omit it from default `help` output. Hidden targets still appear in `run` tab completion and can be invoked with `helm run <name>`.

```helm
target _internal_fixup() {
    help = "Regenerate local fixtures (maintainer only)"
    hidden = true
    ...
}
```

* **`hidden = true`**: Excluded from the main target list in `help`. Use `help --show-hidden` to list them under a separate **hidden targets** section.
* **`hidden = false`**: Default; target is shown in normal help.

---

## 3. The Execution Graph (`depends_on`)

The `depends_on` block defines the edges of the DAG. Helm resolves dependencies into execution phases and runs targets in the same phase in parallel when they do not depend on each other.

Dependencies are declared in a multiline array using brackets `[]`. A dependency is either a target name string or a braced entry with optional modifiers and/or a `params` block.

```helm
target deploy() {
    depends_on [
        "lint"
        "test" { optional = true }
        "build" { confirm = true }
    ]
}
```

### Dependency modifiers

* **`optional = true`**: If the dependency fails or is skipped, the dependent target may still run (subject to the rest of the graph).
* **`confirm = true`**: Pauses before that dependency runs and asks for confirmation (interactive CLI).

Modifiers can use assignment form (`optional = true`) or keyword-then-literal form (`optional true`), as long as the value is a boolean literal.

### Passing parameters to dependencies (`params`)

When a dependency target declares parameters, the caller must supply every **required** parameter in a nested `params { ... }` block on that edge. Optional parameters on the dependency may be omitted.

```helm
BUILD_DIR = "bin"

target build(GOOS, GOARCH, OUT) {
    env {
        GOOS = "${GOOS}"
        GOARCH = "${GOARCH}"
        CGO_ENABLED = "0"
    }

    run "go build -o ${OUT} ./cmd/app"
}

target build_linux_amd64() {
    depends_on [
        build {
            params {
                GOOS = "linux"
                GOARCH = "amd64"
                OUT = "${BUILD_DIR}/app-linux-amd64"
            }
        }
    ]
}
```

**Compile-time rules (semantic validation):**

* Every key in `params { ... }` must name a parameter on the dependency target (`TARGET_015` if unknown).
* Every **required** parameter on the dependency must appear in that edge’s `params` block (`TARGET_016` if missing).
* The same parameter name cannot appear twice in one `params` block (`TARGET_017`).
* The same dependency target cannot be listed twice in one `depends_on` array (`TARGET_014`).

**Runtime behavior:**

* Parameter values are string literals. They may contain `${NAME}` placeholders; Helm resolves those using **global variables** and the **invocation parameters of the dependent target** (the target that owns the `depends_on` edge), then passes the resolved map to the dependency.
* Resolved parameters are in scope for the dependency’s `run` strings, `env` values, and `workdir` (same interpolation rules as a directly invoked target).
* Each dependency target runs **at most once** per graph execution. If two dependents in the same run both list the same dependency with `params`, every edge must supply the **same** resolved parameter map; otherwise the engine reports a conflict and aborts.
* Parameters supplied on the CLI for the entry target (`helm run build_linux_amd64` / `run build GOOS=linux`) apply only to that target’s own signature, not automatically to its dependencies—you wire dependency arguments explicitly in `params`.

`params` can be combined with `optional` and `confirm` in the same braced dependency entry:

```helm
depends_on [
    preflight {
        optional = true
        params {
            MODE = "quick"
        }
    }
]
```

---

## 4. State & Caching (`artifacts`)

The `artifacts` block defines the I/O state boundary of the target. Helm uses this block to cryptographically hash the state and automatically skip redundant executions.

```helm
target compile(OS) {
    artifacts {
        // Inputs can mix explicit files and glob patterns in a single array
        inputs = [
            "go.mod",
            "go.sum",
            glob("src", "*.go"),
        ]
        
        // Outputs can mix explicit files, globs, and constructed paths
        outputs = [
            path("bin", "app-${OS}"),
            glob("bin", "*.exe"),
            "build.log",
        ]
    }
}
```

* **`inputs`**: A single string/glob/path, or a multiline array mixing explicit file strings and `glob()` / `path()` calls. Defines the files Helm must hash to determine if the target needs to run.
* **`outputs`**: A single string/glob/path, or a multiline array mixing explicit file strings, `glob()` / `path()` calls. Defines the deterministic files Helm expects the target to produce.
* **`volatile = true`**: Explicitly tells the engine to *never* cache this target (e.g., for deployments or database migrations). If `outputs` is omitted, the engine uses inputs-only caching unless `volatile` is set.

Target and matrix variables may appear in `glob()` / `path()` / string literals as `${NAME}` placeholders; they are resolved at execution time using the effective parameter map for that run.

---

## 5. Matrix execution (`matrix`)

A `matrix` block turns one target into multiple parallel execution units. Each unit binds the matrix variable for that run. Matrix legs share the same target name in the DAG (dependents still list the target once).

```helm
target generate() {
    help = "Generates all Go modules independently"

    matrix MOD in [
        "libs/lingua",
        "libs/syntaxa",
    ]

    artifacts {
        inputs = [
            glob("${MOD}", include="*.go"),
        ]
        outputs = [
            glob("${MOD}", include="*_gen.go"),
        ]
    }

    run "cd ${MOD} && go generate ."
}
```

* **`matrix VAR in [...]`**: Required list of literal strings, `path()` values, variable references, or a single `glob()` whose matches become separate bindings (one instance per matched path).
* **Caching**: Each matrix leg has its own cache record keyed by target name and binding (e.g. `MOD=libs/lingua`). Unchanged legs can be skipped independently on later runs.
* **Parallelism**: All legs of a matrix target in a phase run concurrently, like unrelated targets in the same phase.
* The matrix variable name must not match a target parameter name.

---

## 6. Procedural Control Flow (`when`)

While Helm targets are nodes in a DAG, their internal execution is procedural. `when` blocks allow you to conditionally gate specific `run` commands based on target parameters.

`when` blocks cannot be nested.

### Supported Conditions
* `defined(PARAM)`
* `not_defined(PARAM)`
* `equals(PARAM, "value" | 123)`
* `not_equals(PARAM, "value" | 123)`

```helm
target publish(TAG?) {
    when defined(TAG) {
        run "docker push myapp:${TAG}"
    }

    when not_defined(TAG) {
        run "docker push myapp:latest"
    }
}
```

---

## 7. Execution (`run`)

The `run` keyword accepts a single-line string. Helm parses this string and passes it directly to the native OS process spawner (shlexing), bypassing shell interpreters to enforce complexity limits.

```helm
target migrate() {
    // Single-line execution only. 
    // Shell piping (|, &&) is not evaluated by the Helm engine.
    run "./scripts/db_migrate.sh up"
}
```

---

## 8. Built-in Functions

Helm provides native functions for resolving paths and file trees safely across platforms.

* **`glob(base_dir, kwarg="...")`**: Declares a file-tree scan boundary for artifact `inputs` and `outputs`. Recognized keyword arguments (stored in IR for the runtime walker; not expanded at compile time):
  * `include`, `exclude` (string patterns). Each may appear multiple times; patterns are merged (`include` matches the union of all patterns, `exclude` removes paths matching any pattern).
  * `follow_symlinks` (boolean literal `true`/`false` or string `"true"`/`"false"`, default `false`)
  * `recursive` (boolean literal `true`/`false` or string `"true"`/`"false"`, default `true`)
  * `types` (string, default `"files"`): which entries under `base_dir` are collected:
    * `"files"` — regular files only. When `recursive = true`, directories are descended and matching files inside are included.
    * `"directories"` — directories only (the directory paths themselves, not their contents).
* **`path(element1, element2, ...)`**: Constructs OS-safe paths safely.

```helm
target clean() {
    artifacts {
        inputs = glob(".", include="*.go")
        outputs = path(BUILD_DIR, "cache", "temp")
    }
}
```

---

## 9. Strings & Interpolation

Helm uses double quotes `"..."` for strings. Variables and parameters can be injected using `${VAR}`.

```helm
target greet(NAME) {
    run "echo 'Hello ${NAME}, starting build in ${BUILD_DIR}'"
}
```