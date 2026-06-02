# LOG

Newest entries on top. Format per ~/.claude/prompts/SOFTWARE_DEV.md.

---

[2026-06-02 12:40] | Scaffolded Express + TS static-demo app: 4 API endpoints + cohort-array visualization with >20 middle-collapse.
VERIFIED:
  npm install
  npm run dev            # http://localhost:3000 — click Create / Add x5 / Take / Total
  # CLI:
  curl -s -X POST localhost:3000/api/create -H 'content-type: application/json' -d '{"capacity":10}'
  curl -s -X POST localhost:3000/api/add    -H 'content-type: application/json' -d '{"n":13}'
  curl -s -X POST localhost:3000/api/take   -H 'content-type: application/json' -d '{"n":4}'
  curl -s localhost:3000/api/total
  npm run typecheck      # tsc --noEmit, exit 0
METRICS:
  serviceMetric: api.{create,add,take,total}.latency_ms emitted per request (withMetrics wrapper, src/server.ts).
  applicationMetric: WARN on bad n/capacity (400 path); ERROR + stack on any handler throw (500 path) — no silent catches.
NOTES:
  - This is the STATIC demo only: src/demo-state.ts serves a hand-authored scripted "tour"
    (grow adds-on-left to a 25-cohort peak, then drain takes-from-right). Request body `n` is
    validated but ignored — snapshots are canned. NOT the real engine.
  - Known limitation: module-level cursor is shared across requests (fine for single-user demo).
NEXT:
  - Implement the real WaitingList engine (add = top-off-then-open-left, take = drain-from-right,
    capacity enforcement) + edge cases: take 0, take > total, add 0, capacity 1, empty-cohort cleanup.
  - Wire endpoints to the engine using the real `n`; add unit tests replaying the PROBLEMS.md table.
