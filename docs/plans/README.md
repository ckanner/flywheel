# Design proposals

[Documentation](../README.md) · [中文文档](../README.zh-CN.md)

These dated proposals explain why features were designed and how their work
was staged. Proposed work is not automatically shipped work. The
[architecture specification](../design.md) describes the maintained contract;
[validation records](../validation/README.md) identify the checks actually performed.

| Date | Proposal | Focus |
| --- | --- | --- |
| 2026-11-16 | [Reviewed design and progressive planning](2026-11-16-reviewed-planning.md) (Chinese; proposal, not implemented) | New / Plan semantics, independent design review, version-specific approval, rolling execution plans, and reuse of native Provider capabilities |
| 2026-11-13 (revised 2026-11-14) | [Native harness, PTC and LCU integration](2026-11-13-native-harness-capabilities.en.md) / [中文](2026-11-13-native-harness-capabilities.md) | Implemented six-repository design: native profiles, PTC/LCU, Agent/Team/Swarm/Workflow, Tower candidates, independent jobs, ownership, accounting and recovery |
| 2026-11-13 | [P5 ready queues and bounded runtime preparation](2026-11-13-p5-ready-queue-prewarm.md) (Chinese) | Optional admission queues, DSH initialization alongside verification, preserved delivery barriers, and existing-candidate assessment |
| 2026-11-13 | [JEV execution decisions and bounded diagnostics](2026-11-13-jev-execution-decisions.md) (Chinese) | Reproduction commands, HTTP evidence, failure actions, allowed model selection, and shared durable budgets |
| 2026-11-12 | [Unified JEV decisions](2026-11-12-unified-jev-decisions.md) (Chinese) | One master switch, actual investigation adoption, evidence-based deferral/report handoff, and durable recovery |
| 2026-11-12 | [Feedback-driven workflows and JEV](2026-11-12-feedback-driven-workflows.md) (Chinese; proposal with subsequent implementation) | Auto / Quick / Full, bounded investigation, fast feedback, model policy, and staged semantic decisions |
| 2026-11-10 | [Workbench redesign](2026-11-10-workbench-redesign.md) | Canvas, configuration, and run-history interaction |
| 2026-11-10 | [Task discovery](2026-11-10-task-discovery.md) | Session attribution and task selection |
| 2026-11-10 | [JEV shadow evaluation](2026-11-10-jev-shadow.md) | Optional routing observations without changing execution |
| 2026-11-08 | [Flow console](2026-11-08-flow-console.md) | Stage controls, sessions, scheduling, and delivery |
| 2026-11-07 | [Session lifecycle](2026-11-07-session-lifecycle.en.md) / [中文](2026-11-07-session-lifecycle.md) | Role reuse, durable recovery, and evaluation phases |
| 2026-11-06 | [Adaptive workflow](2026-11-06-adaptive-workflow.md) | Auto / Quick / Full |
| 2026-11-03 | [Codex Pro default route](2026-11-03-codex-pro-default.md) | Direct use of the public machine API |
| 2026-11-01 | [npm installation](2026-11-01-npm-installation.md) | One-command setup and six-package distribution |
| 2026-11-01 | [Model profiles and preflight](2026-11-01-model-profiles-preflight.md) | Bindings, configuration inspection, and local diagnostics |
| 2026-11-01 | [DSH integration](2026-11-01-dsh-integration.md) | Desktop control plane and independent workers |
