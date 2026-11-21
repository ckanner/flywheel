# Changelog

## 0.2.0 — release candidate

- Add the three exact Orchestrator model slugs with fixed empty effort and no native `--effort` override.
- Add versioned `native` / `subagents` / `swarm` selection to defaults, Desktop actions and UI, Agent/PTC and public machine API; bind exact options to session leases.
- Admit explicit subagents through observed `invoke_subagent`; dispatch Swarm through one adapter-owned slash command with JSON task data, version/configuration checks and required child evidence.
- Retain observed child identities, roles and URI metadata without inventing worker models or terminal states; expose live task status and scope root completion to the CLI invocation.
- Stop on an optional total observed-child guard (including verifier/monitor), explicitly `hard:false`; fail closed on aggregate member metadata retention overflow while retaining verified IDs.
- Reject every nonempty native `finished_reason`, even with SUCCESS/exit zero. Explicit collaboration without child evidence fails as `collaboration-not-observed`.
- Keep evidence-only empty-tools admission, slash/spawn exclusion and Orchestrator rejection; preserve owned cancellation and read-only recovery.
- Extend real subprocess, public DSH schema/PTC, React and installed-tarball tests; add a Chinese machine contract.

## 0.1.0 — release candidate

- Official `agy` stream-json runner shared by DSH direct tasks, Agent/PTC tool delegation and public ESM machine API.
- Exact native model/effort admission, subscription-route controls and configuration drift checks; no ACP or alternate paid API/credits path.
- Durable lifecycle journal, caller acknowledgements, owned process-group cancellation, parent-death supervision and read-only recovery.
- Exact conversation resume with binding/lease protection, local task labels and explicit unknown provider/child/cost evidence.
- Evidence-only native custom-agent guard: real pinned-agent/empty-tools init before prompt dispatch.
- Owner-bound Desktop actions, direct task view, native settings form, allowed-preset tool registration and host background jobs.
- Offline protocol/lifecycle/DSH/React tests and installation of the real npm tarball outside the checkout.

This entry describes implemented code, not npm publication, real-account acceptance or a deployment to the user's machine.
