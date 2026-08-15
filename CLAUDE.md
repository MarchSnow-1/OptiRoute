# CLAUDE.md

## Read first

This repository's authoritative agent guidance is in **AGENTS.md**.

Before making any code change, read:

1. `AGENTS.md`
2. `docs/tech/technical-design.md`

`docs/tech/technical-design.md` is the technical design index that explains the codebase by package, file, data flow, concurrency, and security boundaries.

## Quick orientation

- OptiRoute is a Go-based distributed Layer 4 reverse proxy.
- It has four roles: Center, Edge, Client Agent, and Server Agent.
- The Go module is not at the repository root; run Go commands from the module root.
- Windows, Linux, and macOS are supported targets.
- The project is pre-STABLE and does not maintain backward compatibility.

## Rules for AI agents

1. Read `AGENTS.md` first.
2. Read `docs/tech/technical-design.md` to build overall context before editing.
3. Propose a plan before modifying code.
4. Keep changes small and compilable in every batch.
5. Do not remove or replace Windows-compatible logic.
6. Do not run repository-wide formatting or line-ending conversion.
7. Do not commit or release without explicit approval.
8. After feature changes, update `docs/tech/technical-design.md` if the affected module or function is documented there.
