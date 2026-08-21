# Drill: 缺陷注入真实仓库演练（§4.3，2026-08-20 首跑 / 2026-08-21 重审修订）

## 目的

验证缺陷注入抽检的全链路在**真实 git 命令**下成立：基线绿 → `git apply`
注入 → 测试真实变红 → `git apply -R` 反向还原（保留 executor 其他改动）。

## 方法（可重复）

`node --test tests/drill-defect-injection.test.js`（离线，仅需 git + node）：

1. /tmp 建真 git 仓库：`src/sum.js`（`return a + b`）+ `sum.test.js`
   （断言 `sum(2,3) === 5`），初始提交。
2. patch：`return a + b` → `return a - b`，提交进仓库。
3. `runDefectInjection({patchFile, testCmd: 'node --test sum.test.js', cwd})`
   —— 走生产默认 exec，不做任何替身。
4. 反例对照：注入 `a + b` → `(a + b)`（测试抓不住的装饰性变更），
   期望 `outcome: 'fake-tests'`。

## 实测结果（2026-08-21 语义）

- 正例：`{outcome:'pass', baselineGreen:true, testsWentRed:true, restored:true}`，
  还原后 `src/sum.js` 逐字节复原，**同目录下 executor 的未提交产物原样保留**
  （`git apply -R` 只移除注入的 hunks；旧的 `git checkout -- <files>` 会从
  index 还原、抹掉 executor 工作——已修，P0-1）。
- 反例：`outcome:'fake-tests'`，仍安全还原。

## 分案语义（P0-2）

| outcome | 含义 | 处置 |
|---|---|---|
| `pass` | 基线绿 + 注入变红 + 还原成功 | 结果喂 Review，正常继续 |
| `fake-tests` | 注入后测试仍绿 = 假测试实锤 | inbox + 停（DC3） |
| `uninjectable` | patch 读不到/碰账本路径/apply 失败 | inbox 请人刷新 patch，单元继续，不指控 |
| `baseline-red` | 注入前套件就红 | inbox（套件本身坏），单元继续 |
| `restore-failed` | `-R` 失败，工作区可能残留缺陷 | inbox + 停 |

`workspaceClean` 检查已删除：-R 后 touched 文件上 executor 的 diff 本来
就该在，它不是"脏"。patch 触及账本路径（state.json/reports/ 等，常量在
`core/ledger.js` 单一来源）直接拒绝注入。

## 附带发现（均已修复）

- `NODE_TEST_CONTEXT` 会沿 env 继承：嵌套 `node --test` 在 child 模式下
  失败也退出 0。编排器跑验收/测试命令前 scrub（`core/child-env.js` gateEnv，
  且整体已升级为白名单环境——见 403-fallback.md 同期记录，P0-4）。
- 演练仓库是 node 单文件项目；monorepo 下 `git apply` 路径前缀未演练。

## 残余风险

- 抽检触发策略（每 5 单元 / 测试占比加权）只有 fake-exec 单测与集成测试
  覆盖，未在真实任务流里跑过。
- 测试占比加权在 DC1（executor 不写测试）下基本不会触发——保留逻辑但
  视为低频路径。
