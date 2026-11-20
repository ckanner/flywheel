# Validation and release evidence

This candidate was implemented on Linux with Node 24.22.3/npm 11.19.1. No user account, credentials, billing setting or npm release has been changed.

## Reproducible offline gates

```sh
npm ci --ignore-scripts
npm test
npm run test:package
```

The test suite uses `test/fixtures/fake-agy.js`, an executable implementation of the documented wire envelopes. It verifies observable admission and failure boundaries, not a real Gemini answer.

| Layer | Checked behavior |
| --- | --- |
| Native wire | Successful init/steps/result, exact resume, early native error, truncated/duplicate/missing/nonterminal result, wrong model, schema validation, soft denial despite exit zero, nonzero exit, long progress separate from retained output. |
| Subscription | API/GCP/credits/permission conflicts rejected before prompt; filtered key and Node injection environment; settings drift rejected; no fallback. |
| Evidence-only | Exact native agent plus empty tools before stdin; inherited tools and absent init block prompt; tool step fails; managed agent cleanup. |
| Lifecycle | Caller acknowledgement before stdin, rejection blocks dispatch, abort, total deadline, parent death, original receipt recovery, stubborn descendants, unknown native state. |
| Sessions | Same binding exact resume; changed model rejected; cross-process concurrent/unresolved lease blocked; recovery is read-only. |
| DSH backend | Direct command uses the same runner, owner/workspace isolation, duplicate invocation idempotency, fresh config snapshots, committed preset checks, jobs and cancellation entry. |
| DSH schema/PTC | Actual published `@deepseek-ai/dsh-tools` and `dsh-tool-executor-ptc` 0.2.2-rc.3 validate schema and generate named input/output SDK types. |
| Desktop client | Public ModuleLoader/slots/remote.commands contract, real React render, direct owner-bound start, defaults load with no inference, revisioned settings mutation. |
| Packed artifact | Real npm tarball installed outside the source tree without DSH peers; exported API/types/bin/client/patch/supervisor present; new turn, evidence-only admission and read-only recovery through installed package. |

`scripts/test-package.js` prints exact tarball integrity and writes ignored `candidate.json` for a local acceptance run. The tarball is not automatically published or installed into an existing user profile.

## Host installation

The package follows the same public DSH bundle shape as the maintained Claude/Codex plugins: root plugin export, `dsh.bundle.patch`, `cordis.patch.yml`, and lazy `./client`. The root plugin checks commands/workingDirectory/lifecycle services. Optional jobs are checked when a background delegation is requested.

Candidate installation must use the real DSH CLI in a disposable profile:

```sh
dsh plugin --profile <disposable-profile> add /absolute/candidate.tgz
```

Use isolated `DSH_HOME` and `DSH_AGENTS_HOME` for this gate. `npm run test:host-install` performs it against the candidate recorded by `test:package`. The real `@deepseek-ai/dsh` 0.2.2-rc.3 CLI successfully installed this candidate into an isolated profile on 2026-11-20, resolving the six production packages through pnpm and validating package/bundle/client presence. The CLI's published help confirms `plugin --profile` initializes a profile and forwards `add` to pnpm. This does not exercise the user's graphical Desktop or native account.

## Required real-machine gates before claiming deployment

On the target Mac and its existing official login:

1. Run inference-free doctor in the exact Desktop cwd/HOME/command environment; inspect real `agy` version/catalog and local subscription controls.
2. Verify the account has the intended model entitlement and credits fallback remains disabled in official settings. Do not replace this with a token estimate.
3. Install the exact candidate and integrity into a test Desktop profile. Confirm the Antigravity view, settings, direct launch without parent-model traffic, allowed preset/PTC tool registration, jobs output and cancellation.
4. Perform one explicitly selected normal Gemini task, a same-binding repair, and cancellation. Record requested/observed model, native session, usage scope, output, local cleanup and remaining unknown fields.
5. Exercise evidence-only admission with the real CLI. It must return the selected custom agent and empty tools **before stdin**. A CLI that does not do so must be reported incompatible; never bypass the guard.
6. Check a quota/auth boundary without enabling another billing route. Native child/browser features need separate observed tests; this candidate does not claim image attachment transport or whole-tree budget enforcement.

No claim of latency improvement, model superiority, zero additional bill, exact remaining quota or Mac compatibility follows from offline fixture success. Current unknown native/remote work remains blocked for automatic replay.

## Release procedure

Review the full diff and run both offline gates. Perform disposable-profile installation and the real-machine checks above. Freeze the candidate version and integrity; publish only that reviewed package after separate authorization. Do not regenerate the tarball after approval without repeating integrity/installation checks. Updating a DSH profile or Flywheel default is an explicit subsequent action, not an npm postinstall side effect.

中文说明：上述离线测试能证明协议、准入、守护、只读恢复、DSH 工具/面板契约和安装包内容；不能代替用户 Mac、官方账号、套餐权益或真实模型质量验收。发布、用户 profile 安装和默认配置迁移仍须针对已审查候选进行，不在测试或安装脚本中自动发生。
