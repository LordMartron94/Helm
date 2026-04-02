# KEEP LOGIC OUT OF HELM SYNTAX

STATUS: accepted

SUPERSEDING:
SUPERSEDED:

DECISION: Helm syntax remains orchestration-only; complex logic is implemented in external scripts, not in Helm source files.
DATE: 2026-04-02

## RATIONALE

Helm is designed to express build intent clearly: what runs, in what order, and under which simple conditions.
Allowing logic-heavy syntax (shell control flow, chained operators, loops, nested conditional composition, or ad-hoc scripting patterns) would turn Helm files into opaque execution programs rather than readable orchestration maps.

By keeping logic out of Helm syntax:

- Build graphs stay flat and debuggable.
- The language surface remains small and predictable.
- Complex behavior moves into dedicated scripts that can be tested, linted, benchmarked, and reused independently.
- Semantic analysis can enforce structural constraints early and consistently.

This aligns Helm with its design principle: Helm orchestrates, it does not execute complex program logic.

## ALTERNATIVES CONSIDERED

1. Allow inline shell control flow in `run` statements.
   Rejected because it hides complexity in-place and quickly degrades readability.

2. Add advanced conditional expressions directly to Helm syntax.
   Rejected because it pushes Helm toward a general-purpose language and increases parser and analyzer complexity.

3. Permit nested conditional blocks for convenience.
   Rejected because it encourages tree-shaped control flow in files intended to stay linear and operationally transparent.

## REFERENCES

- `README.md` design philosophy for Helm
