#!/usr/bin/env node
// Implements the documented wire protocol for offline transport assertions.
import { appendFileSync, writeFileSync, readFileSync } from 'node:fs'
import { randomUUID } from 'node:crypto'
import { spawn } from 'node:child_process'
const args=process.argv.slice(2),arg=key=>args[args.indexOf(key)+1],scenario=process.env.FIXTURE_SCENARIO??'success'
if(process.env.FIXTURE_CALLS)appendFileSync(process.env.FIXTURE_CALLS,JSON.stringify({args,cwd:process.cwd(),pid:process.pid})+'\n')
if(args[0]==='--version'){console.log('agy '+(scenario==='old'?'1.5.6':'1.5.7'));process.exit()}
if(args[0]==='--help'){console.log('--input-format --output-format --model --effort --mode --conversation --print-timeout --disable-slash-commands --dangerously-skip-permissions --sandbox --json-schema --agent');process.exit()}
if(args[0]==='models'){console.log('gemini-4-hydrogen-high Gemini 4 Hydrogen (High)\ngemini-4-neon-high Gemini 4 Neon (High)\ngemini-4-carbon-high Gemini 4 Carbon (High)');process.exit()}
if(process.env.FIXTURE_PID)writeFileSync(process.env.FIXTURE_PID,String(process.pid))
const id=args.includes('--conversation')?arg('--conversation'):randomUUID(),agent=args.includes('--agent')?arg('--agent'):null
const emit=value=>process.stdout.write(JSON.stringify(value)+'\n')
const init={event:'init',conversation_id:id,init:{cwd:process.cwd(),model:scenario==='wrong-model'?'gemini-4-carbon-high':arg('--model'),permission_mode:args.includes('--dangerously-skip-permissions')?'always-proceed':'request-review',tools:agent&&scenario!=='tools-inherited'?[]:['run_command'],...(agent?{agent}:{}),...(args.includes('--json-schema')?{json_schema:JSON.parse(arg('--json-schema'))}:{})}}
if(scenario==='missing-agent')delete init.init.agent
if(scenario==='wrong-agent')init.init.agent='different-agent'
if(scenario==='early-error'){emit({event:'result',result:{conversation_id:'',status:'ERROR',response:'',error:'login required'}});process.exit(1)}
if(scenario!=='no-init')emit(init)
let raw=''
process.stdin.on('data',b=>{raw+=b;if(process.env.FIXTURE_INPUT)appendFileSync(process.env.FIXTURE_INPUT,b)})
process.stdin.on('end',async()=>{
  if(scenario==='no-init')return
  const message=JSON.parse(raw)
  if(message.event!=='user'||typeof message.message.content!=='string')process.exit(4)
  if(scenario==='grandchild'){
    const child=spawn(process.execPath,['-e','process.on("SIGTERM",()=>{});setInterval(()=>{},1000)'],{stdio:'ignore'})
    if(process.env.FIXTURE_CHILD_PID)writeFileSync(process.env.FIXTURE_CHILD_PID,String(child.pid))
  }
  if(['hang','grandchild'].includes(scenario)){setInterval(()=>{},1000);return}
  emit({event:'step_update',step_update:{conversation_id:id,step_index:0,state:'DONE',step_type:'user_input'}})
  if(scenario==='truncated'){process.stdout.write('{"event":');process.exit();return}
  if(scenario==='oversize'){process.stdout.write('x'.repeat(1024*1024));process.exit();return}
  if(scenario==='denied')process.stderr.write('Tool run_command was soft-denied: permission approval required\n')
  if(scenario==='drift')writeFileSync(process.env.HOME+'/.gemini/antigravity-cli/settings.json',JSON.stringify({useG1Credits:true}))
  const count=scenario==='long-stream'?2000:1
  for(let i=0;i<count;i++)emit({event:'step_update',step_update:{conversation_id:id,step_index:1,state:'ACTIVE',step_type:scenario==='illegal-tool'?'tool':'agent_response',text_delta:'progress '.repeat(scenario==='long-stream'?100:1)}})
  const result={event:'result',result:{conversation_id:id,status:scenario==='waiting'?'WAITING':scenario==='quota'?'ERROR':'SUCCESS',response:scenario==='quota'?'':'done',...(scenario==='quota'?{error:'quota exhausted'}:{}),duration_seconds:1,num_turns:1,usage:{input_tokens:100,output_tokens:20,thinking_tokens:3,cache_read_tokens:0,total_tokens:123}}}
  if(args.includes('--json-schema'))result.result.structured_output=scenario==='schema-bad'?{wrong:true}:{answer:'done'}
  emit(result)
  if(scenario==='duplicate')emit(result)
  if(scenario==='exit-fail')process.exitCode=1
})
if(scenario==='no-init')setInterval(()=>{},1000)
