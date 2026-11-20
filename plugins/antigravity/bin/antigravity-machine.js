#!/usr/bin/env node
import { MACHINE_PROTOCOL_VERSION, MACHINE_CAPABILITIES, MACHINE_LIFECYCLE_CAPABILITIES, MACHINE_ACTIVITY_CAPABILITIES, runMachine, recoverMachine, doctor } from '../lib/machine.js'

const controller=new AbortController()
process.on('SIGTERM',()=>controller.abort());process.on('SIGINT',()=>controller.abort())
try {
  const flags=process.argv.slice(2)
  if(flags.length!==1||!['--capabilities','--doctor','--json','--recover'].includes(flags[0]))throw new Error('usage: antigravity-machine --capabilities | --doctor | --json | --recover; JSON request on stdin')
  let result
  if(flags[0]==='--capabilities')result={protocol:MACHINE_PROTOCOL_VERSION,capabilities:MACHINE_CAPABILITIES,lifecycle:MACHINE_LIFECYCLE_CAPABILITIES,activity:MACHINE_ACTIVITY_CAPABILITIES}
  else {
    let input=''
    for await(const chunk of process.stdin){input+=chunk;if(Buffer.byteLength(input)>2*1024*1024)throw new Error('JSON input exceeds 2 MiB')}
    const value=JSON.parse(input)
    result=flags[0]==='--recover'?await recoverMachine(value):flags[0]==='--doctor'?await doctor(value,{signal:controller.signal}):await runMachine(value,{signal:controller.signal})
  }
  process.stdout.write(JSON.stringify(result)+'\n')
  if(result.ok===false)process.exitCode=1
}catch(error){process.stdout.write(JSON.stringify({protocol:1,type:'error',error:{category:'invalid-request',detail:error.message}})+'\n');process.exitCode=2}
