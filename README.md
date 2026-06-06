<div align="center">

<img src="assets/helm-logo-cropped.png" alt="Helm" width="168" />

# Helm

**Orchestration-first build system — explicit graphs, minimal in-file execution.**

[Syntax](./docs/syntax.md) · [Design](#design-philosophy) · [Examples](#syntax-overview)

<br />

</div>

Helm is my attempt at building a fully featured build system from first principles, as an alternative to Makefile.

I started doing this because I thought it would be enjoyable as well as a good way to battletest my language toolkit before moving on to more complex projects (i.e., my own programming language `force`).

The idea behind Helm is to be flexible and powerful but with a syntax that is simple to understand.

## Table of Contents

- [Design philosophy](#design-philosophy)
- [Syntax overview](#syntax-overview)
- [Building](#building)

## Design philosophy

Helm is built on a ruthless separation of concerns: **Helm orchestrates, it does not execute.**

Build configurations often degrade into unmaintainable technical debt when developers blur the line between dependency graph management and raw shell scripting. Helm prevents this by enforcing structural best practices directly at the compiler level. The goal is to guarantee that your build graph remains universally readable, even at 2:00 AM during a critical deployment failure.

Helm exists to be a transparent map of *what* happens and *when*, aggressively offloading the *how* to standard scripts where it belongs.

As such, the capability of direct execution is intentionally *limited* inside the Helm syntax. No more long shell(-like) blocks such as in Make. We have a `run` command which accepts a single-line string that is parsed into a command and fed to the evaluator. This means complex logic by definition is impossible in runs.

A `run` is intended to do simple things like running an executable or `echoing` something simple.

At runtime, Helm resolves `${...}` in `run`, `env`, and `workdir` using global variables plus the **effective parameters** for that target—either from `helm run` / the REPL for the entry target, or from `depends_on` `params` blocks for dependencies. Shell features (pipes, `&&`, subshells) are not interpreted; commands are tokenized and executed directly.

## Syntax overview

Helm is designed to read like a map of state, not a procedural script. Here are a few examples demonstrating the separation of orchestration, state, and execution.

### 1. The parameterized target

Targets are explicitly parameterized. There are no arcane automatic variables (`$@`) or hacky environment overrides.

```helm
target greet(NAME) {
    help = "Prints a greeting to the specified name."
    run "echo 'Hello, ${NAME}!'"
}
```

### 2. The pure orchestrator (caching & graph)

Helm solves the dependency graph and handles cryptographic caching automatically. The user defines the edges (`depends_on`), the state boundary (`artifacts`), and the single action required to achieve that state (`run`). Independent branches in the same phase run in parallel; each dependency target is executed at most once per run.

```helm
target compile(OS) {
    help = "Compiles the primary binary for a given OS."

    // The execution edges. Helm automatically evaluates these in parallel if possible.
    depends_on [
        "generate-mocks",
        "lint"
    ]

    // The state boundary. Helm computes the hash of these inputs.
    // If the hash matches the cache, the 'run' block is completely bypassed.
    artifacts {
        inputs = glob("src", "*.go")
        outputs = path("bin/app-${OS}")
    }

    run "go build -o bin/app-${OS} ./src"
}
```

### 2b. Wiring dependencies with `params`

Parameterized targets are reusable building blocks. Thin wrapper targets pass concrete arguments on the edge instead of duplicating `run` blocks:

```helm
target build(GOOS, GOARCH, OUT) {
    env {
        GOOS = "${GOOS}"
        GOARCH = "${GOARCH}"
    }
    run "go build -o ${OUT} ./cmd/app"
}

target build_linux_amd64() {
    depends_on [
        build {
            params {
                GOOS = "linux"
                GOARCH = "amd64"
                OUT = "bin/app-linux-amd64"
            }
        }
    ]
}
```

At runtime, `${GOOS}` / `${GOARCH}` inside the dependency’s `env` and `run` are filled from that `params` map (after interpolating any `${GLOBAL}` or parent-target placeholders in the param values themselves). This is how `Helmfile` cross-compile wrappers stay small while sharing one `build` implementation.

### 3. Context & volatility

When a target hits external infrastructure, it cannot be safely cached based on file inputs alone. Helm isolates the execution context (`env`, `workdir`) and allows authors to explicitly declare a target as volatile, forcing the engine to always execute it.

```helm
target db-migrate() {
    help = "Applies pending SQL migrations to the database."

    workdir = "./infrastructure/db"

    env {
        DB_HOST = "localhost"
        DB_USER = "admin"
    }

    artifacts {
        volatile = true // Prevents the engine from caching this execution
        inputs = glob("migrations", "*.sql")
    }

    run "./apply_migrations.sh"
}
```

### 4. Interactive terminal ownership

Targets that launch REPLs or other TTY-driven programs set `interactive = true` at the target body level (not inside `artifacts`). Helm attaches the host `stdin`/`stdout`/`stderr` to the child process and enforces that no other target runs in the same parallel DAG phase. Interactive targets cannot use a `matrix` block.

For more detail see: [syntax reference](./docs/syntax.md)

As with all my syntaxes, the [LangSpec](https://github.com/LordMartron94/LangSpec) definition lives in [Lingua](https://github.com/LordMartron94/Lingua).

## CLI

With no arguments, `helm` starts an interactive shell (`helm>` prompt). Every built-in shell command also works as a one-shot invocation:

```bash
helm help
helm run <target> [key=value ...]
helm run --bypass-cache <target>
helm run -q <target>
helm export-graph <target>[,<target>...] [key=value ...]
helm export-graph -o build-graph.json <target>
helm export-graph -o graphs.json target_a,target_b
helm clean-cache
helm set stream-runs off
helm version
```

Pass an explicit helm file before the command when needed:

```bash
helm path/to/project.helm run <target>
```

`helm -version` prints the binary version without loading a helm file. `helm version` runs through the normal session (discover helm file, interpret, then print version), matching the REPL `version` command.

`helm completion bash` prints a bash tab-completion script (also listed under `helm help`). See `helm help completion` for install examples.

### Graph introspection (`export-graph`)

Before executing anything, Helm can resolve the full dependency graph, expand parameters on every edge, and interpolate every `run` string. The `export-graph` command writes that resolved state as JSON (language-agnostic introspection for external tools):

```bash
helm path/to/Helmfile export-graph run_tests
helm path/to/Helmfile export-graph -o graph.json build_testbed
helm path/to/Helmfile export-graph -o graphs.json build_testbed,run_tests
```

A single target writes one graph object (`entry_target`, `phases`, `targets`). Comma-separated targets write a bundle with a `graphs` map (one full graph per entry target). CLI `key=value` parameters apply only when a single target is exported.

Each key under `targets` is an execution node (canonical target name, or `target#<hash>` for parametric instances). Fields include `directory` (absolute working directory) and `run_commands` (expanded command strings). No subprocesses are started and the artifact cache is not consulted.

### Run output presentation

Helm separates orchestration reporting from the live terminal:

* **Live stream**: raw subprocess stdout/stderr only (no Helm phase/target banners).
* **Post-run stderr**: diagnostics and execution summary (suppressed for `interactive` entry targets; use `-q` to hide `[INFO]` and summary for build targets).
* **Transcript**: `.helm/last-run.log` is rewritten on each `helm run` with phase/target boundaries, captured I/O, and the full diagnostic render for post-mortem review.
* **Exit code**: interactive entry targets propagate the child process exit code; orchestration errors exit `1`.

### Bash completion

Install tab completion for flags, built-in commands, helm files, and (when a helm file can be resolved) target names:

```bash
helm completion bash | sudo tee /etc/bash_completion.d/helm
# or for the current shell only:
source <(helm completion bash)
```

Target and parameter completion interprets the helm file on each Tab press, so it may feel slow on large projects. The internal `helm __complete` command is for shell integration only and is not intended for direct use.

## Building

### Dependencies

Libraries required to build `helm/cmd/helm` (verified with `go list -deps`):

- Autarch, Echo, Foundation, Langspec, Lexarch, Lingua, Memarch, Memcore, Memforge, Memstruct, Persistence, Signal, Splash, Structarch, Syntaxa

Clone them with:

```bash
./scripts/install_dependencies.sh
```

The installer is scoped to helm only (not blaze, statarch, shield, etc.). After cloning, add the modules and this repository to your `go.work` file.

### First install (no helm binary yet)

From a full force checkout (or once `go.work` is configured):

```bash
./scripts/bootstrap.sh
```

This builds `./bin/helm` and installs to `~/.local/bin/helm` by default. Override with `INSTALL_DEST` or `--dest`.
