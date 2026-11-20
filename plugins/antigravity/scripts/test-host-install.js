import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { mkdtempSync,readFileSync,existsSync,realpathSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join,resolve } from 'node:path'
const root=resolve('.'),candidate=JSON.parse(readFileSync(join(root,'candidate.json'),'utf8'))
const temp=realpathSync(mkdtempSync(join(tmpdir(),'antigravity-host-'))),dshHome=join(temp,'dsh'),profile=join(dshHome,'profiles','antigravity-candidate')
const child=spawn(process.execPath,[join(root,'node_modules/@deepseek-ai/dsh/lib/bin.js'),'plugin','--profile','antigravity-candidate','add',candidate.tarball,'--ignore-scripts'],{cwd:temp,env:{...process.env,DSH_HOME:dshHome,DSH_AGENTS_HOME:join(temp,'agents')},stdio:['ignore','pipe','pipe']})
let output='';child.stdout.on('data',b=>{output=(output+b).slice(-30000);process.stdout.write(b)});child.stderr.on('data',b=>{output=(output+b).slice(-30000);process.stderr.write(b)})
const timer=setTimeout(()=>child.kill('SIGTERM'),180000)
const code=await new Promise((resolve,reject)=>{child.once('close',resolve);child.once('error',reject)});clearTimeout(timer)
assert.equal(code,0,output)
const profilePackage=JSON.parse(readFileSync(join(profile,'package.json'),'utf8'))
assert.ok(profilePackage.dependencies['dsh-subagent-antigravity'])
const installed=join(profile,'node_modules/dsh-subagent-antigravity'),pkg=JSON.parse(readFileSync(join(installed,'package.json'),'utf8'))
assert.equal(pkg.version,candidate.version);assert.ok(existsSync(join(installed,pkg.dsh.bundle.patch)));assert.ok(existsSync(join(installed,pkg.exports['./client'])))
console.log('Real DSH 0.2.2-rc.3 disposable profile installation passed: '+profile)
