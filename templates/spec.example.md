---
spec-version: 1
title: Auth token foundation
goal: Provide a verified JWT parsing path for the gateway.
non-goals:
  - Token issuance / refresh flows
  - UI work of any kind
milestones:
  - id: M-01
    title: Token parser + unit tests
    gate: L1 green for all packets, then L2 integration tests pass.
  - id: M-02
    title: Gateway wiring
    gate: L1 green, L2/L3 pass on the real gateway service.
requirements:
  - id: R-01
    title: parseToken returns claims for a valid JWT
    milestone: M-01
    verification: pnpm test auth (unit), L2 integration
  - id: R-02
    title: parseToken returns null for malformed/expired tokens without throwing
    milestone: M-01
    verification: pnpm test auth (unit)
  - id: R-03
    title: Gateway middleware rejects requests with invalid tokens (401)
    milestone: M-02
    verification: pnpm test:integration gateway, e2e/gateway-auth.ts
---

# Spec: Auth token foundation

Frozen contract. Scope changes can only be made by the Plan Loop (DESIGN.md §4.1).

## Numbered requirements

| id | requirement | milestone | verification |
|---|---|---|---|
| R-01 | parseToken returns claims for a valid JWT | M-01 | `pnpm test auth` |
| R-02 | parseToken returns null for malformed/expired tokens, never throws | M-01 | `pnpm test auth` |
| R-03 | Gateway middleware returns 401 on invalid token | M-02 | `pnpm test:integration gateway` |

## Requirement → milestone map

- M-01: R-01, R-02
- M-02: R-03

## Milestones

### M-01 Token parser + unit tests
Packets: M01-T01. Closed when L1 is green for every packet and the milestone
gate (union of packet L2 commands) passes.

### M-02 Gateway wiring
Packets: M02-T01. Closed when L1 is green and L2/L3 pass against the real
gateway service.
