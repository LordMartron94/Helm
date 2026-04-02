# HELM

Helm is a modern makefile alternative.

I built this because I thought it would be enjoyable and a good way to battletest my language toolkit before moving on to more complex projects (i.e., my own programming language `force`).

The idea behind Helm is to be flexible and powerful but with a syntax that is simple to understand.

## Table of Contents
- [Design Philosophy](#design-philosophy)
- [Syntax](#syntax)

## Design Philosophy

Helm is built on a ruthless separation of concerns: **Helm orchestrates, it does not execute.**

Build configurations often degrade into unmaintainable technical debt when developers blur the line between dependency graph management and raw shell scripting. Helm prevents this by enforcing structural best practices directly at the compiler level. The goal is to guarantee that your build graph remains universally readable, even at 2:00 AM during a critical deployment failure.

Our core design tenets are:

- **Flat Dependency Graphs:** Deeply nested, inline execution chains are by design impossible. Orchestration must be readable top-to-bottom. Parallel and sequential executions are declared in explicit, linear blocks rather than dense micro-DSLs.
- **Hostile to Complexity:** Inline commands (via `run`) are strictly limited to single-line executions. Shell control flow operators (`&&`, `||`, `|`, `;`) and loops are explicitly rejected by the semantic analyzer. If a build step requires complex logic or piping, it belongs in a dedicated, testable shell script executed via `run_script`. 
- **Predictability Over Cleverness:** The syntax minimizes visual noise. Newlines act as significant tokens to terminate statements without the clutter of semicolons, and assignment operators are universally standardized. 

Helm exists to be a transparent map of *what* happens and *when*, aggressively offloading the *how* to standard scripts where it belongs.

## Syntax

Once the language is further developed, I will show a few examples here.

For more detail see: [syntax reference](./docs/syntax.md)

As with all my syntaxes, the [LangSpec](https:github.com/LordMartron94/LangSpec) definition lives in [Lingua](https:github.com/LordMartron94/Lingua).

