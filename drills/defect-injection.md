# Drill: 缺陷注入真实仓库演练（§4.3，2026-08-20）

## 目的

验证缺陷注入抽检的全链路在**真实 git 命令**下成立（开发期单测用的是
注入式 fake exec）：`git apply` 注入 → 测试真实变红 → `git checkout -- .`
还原 → `git status --porcelain` 校验工作区干净。

## 方法（可重复）

`node --test tests/drill-defect-injection.test.js`（离线，仅需 git + node）：

1. /tmp 建真 git 仓库：`src/sum.js`（`return a + b`）+ `sum.test.js`
   （断言 `sum(2,3) === 5`），初始提交。
2. patch：`return a + b` → `return a - b`，提交进仓库。
3. `runDefectInjection({patchFile, testCmd: 'node --test sum.test.js', cwd})`
   —— 走生产默认 exec，不做任何替身。
4. 反例对照：注入 `a + b` → `(a + b)`（测试抓不住的装饰性变更），
   期望判 `pass: false`（测试太弱被发现）。

## 实测结果

- 正例：`{injected:true, testsWentRed:true, restored:true, workspaceClean:true, pass:true}`，
  还原后 `src/sum.js` 逐字节复原。
- 反例：`testsWentRed:false, pass:false`，工作区仍干净还原。

## 附带发现（已修复）

`NODE_TEST_CONTEXT` 环境变量会沿进程树继承：编排器若运行在 `node --test`
之下（或任何 runner 子进程），验收命令里的嵌套 `node --test` 进入 child
模式——**测试失败也退出 0**（实测确认）。这会把 L1 闸门与缺陷注入的
"必须变红"判定整体绿洗。修复：`verify.js gateEnv()` 在跑验收/测试命令前
scrub 该变量（驱动子进程 env 白名单天然不含它）。

## 残余风险

- 演练仓库是 node 单文件项目；monorepo（pnpm workspace 等）下
  `git apply` 路径前缀、`checkout -- .` 的作用范围未演练。
- 抽检触发策略（每 5 单元 / 测试占比加权）只有 fake-exec 单测覆盖，
  未在真实任务流里跑过。
