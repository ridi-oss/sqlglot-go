# CLAUDE.md

This project's agent instructions live in **[AGENTS.md](./AGENTS.md)** — read it first.

@AGENTS.md

## Continuing the port with Claude Code

- The porting method is a multi-model pipeline: **plan → implement → integrate → adversarial
  review**, scoped one coherent slice at a time (see the slice ledger in `ROADMAP.md`). Each slice
  must land `go test ./...` green before the next.
- **Verify every review finding against `.reference/` before acting on it.** During this port,
  reviewers surfaced both real bugs (e.g. `Replace`/`Pop` no-op on single-value args) and phantom
  ones (a "missing guard" that v30.12.0 doesn't actually have). Confirm against the pinned Python
  source, then fix or reject with a one-line rationale.
- Run `scripts/fetch-reference.sh` once to get the pinned Python source (`.reference/`, gitignored).
- After any probe-path change, re-run parity: `go test ./probe/` (hermetic golden) and, with Python
  available, `PROBE_REGEN=1 go test ./probe/ -run TestProbeParity` to re-check live Python + refresh
  goldens. Deferred/unparseable constructs must stay **fail-closed** (probe DENYs, never resolves).
