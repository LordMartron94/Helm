# Helm 2.0 Native Entity Build System

STATUS: accepted

DECISION: Evolve Helm into a native-entity build system with property-bag interfaces, strict entity/target segregation, dumb adapters, and dual-mode v1 compatibility.

DATE: 2026-06-05

## RATIONALE

Monolithic Helmfiles with per-library wrapper targets do not scale. Helm 2.0 introduces
**entities** (artifact producers), **labels** (cross-file addressability), and **property bags**
(transitive flag propagation) while keeping the DSL minimal.

Three purity constraints prevent bloat:

1. **Interfaces are property bags** — named key collections flattened on `collect()`. No type system. Clang/linker validate flags.
2. **Strict entity/target segregation** — entities produce artifacts via adapters only; targets run actions only (no `artifacts`).
3. **Dumb adapters** — argv templates mapping metadata to commands. No orchestration inside adapters.

Dual-mode execution preserves v1.7 Helmfiles via a Legacy Adapter until workspaces migrate.

## ALTERNATIVES CONSIDERED

- **Synthesis bridge** (entities lower to v1 targets): rejected; user chose native entity execution.
- **YAML/TOML entity manifests**: rejected; one DSL for data and templates.
- **Binary plugins**: rejected; template adapters in `.helm` files only.
- **Interface type system**: rejected; maintenance burden with no build benefit.

## REFERENCES

- [STYLE_GUIDE](../../../infra/standards/STYLE_GUIDE.md)
- [vertex-siege v1 contract](../../../../vertex-siege/docs/helm-v1-build-contract.md)
