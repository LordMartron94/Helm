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

---

## 3. The Execution Graph (`depends_on`)

The `depends_on` block defines the edges of the DAG. Helm will automatically resolve these dependencies and execute independent graphs in parallel. 

Dependencies are declared in a multiline array using brackets `[]`. You can pass configuration options to specific dependencies using braces `{}`.

```helm
target deploy() {
    depends_on [
        "lint"
        "test" { optional = true }
        "build" { confirm = true }
    ]
}
```
* `optional = true`: If the dependency fails or is bypassed, the execution continues.
* `confirm = true`: Pauses the graph and prompts the user for manual confirmation before executing the dependency.

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
        
        // Outputs can mix explicit files and constructed paths
        outputs = [
            path("bin", "app-${OS}"),
            "build.log",
        ]
    }
}
```

* **`inputs`**: A single string/glob/path, or a multiline array mixing explicit file strings and `glob()` / `path()` calls. Defines the files Helm must hash to determine if the target needs to run.
* **`outputs`**: A single string/path, or a multiline array mixing explicit file strings and `path()` calls. Defines the deterministic files Helm expects the target to produce.
* **`volatile = true`**: Explicitly tells the engine to *never* cache this target (e.g., for deployments or database migrations). If `outputs` is omitted, the engine uses inputs-only caching unless `volatile` is set.

---

## 5. Procedural Control Flow (`when`)

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

## 6. Execution (`run`)

The `run` keyword accepts a single-line string. Helm parses this string and passes it directly to the native OS process spawner (shlexing), bypassing shell interpreters to enforce complexity limits.

```helm
target migrate() {
    // Single-line execution only. 
    // Shell piping (|, &&) is not evaluated by the Helm engine.
    run "./scripts/db_migrate.sh up"
}
```

---

## 7. Built-in Functions

Helm provides native functions for resolving paths and file trees safely across platforms.

* **`glob(base_dir, kwarg="...")`**: Resolves file patterns.
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

## 8. Strings & Interpolation

Helm uses double quotes `"..."` for strings. Variables and parameters can be injected using `${VAR}`.

```helm
target greet(NAME) {
    run "echo 'Hello ${NAME}, starting build in ${BUILD_DIR}'"
}
```