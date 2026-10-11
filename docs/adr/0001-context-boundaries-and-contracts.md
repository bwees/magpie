# Contexts talk only through published contracts

Magpie is split into eight bounded contexts. Each one lives in its own Go package, and other contexts may use only its `api` package:
- a read-only query port, implemented by the context's repository layer;
- event structs;
- command types for actions another context asks it to take.

Writes never cross a boundary directly. A context changes another's state only by publishing an event or command that the owner handles. Events are stored in a transactional outbox and delivered at least once, so handlers must be idempotent. The query-port dependency graph is acyclic and enforced by an import lint.

We chose this over direct service calls, which the layering rules forbid and which let cycles form. We also rejected two alternatives:
- Event-carried state for all reads: duplicate storage and sync code for little gain in a single process.
- A central orchestrator layer: it becomes a god object.

The contracts are written up front in `docs/spec/contexts/` so the eight contexts can be built in parallel against fakes.
