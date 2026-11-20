import { spawn } from 'node:child_process'
import { command, directory, modelBinding } from './input.js'
import { runtimeEnvironment, inspectSubscription, redact } from './environment.js'

export async function probeCommand(program,args,{cwd,env,signal,timeoutMs=10000}={}) {
  return new Promise(resolve=>{
    let out='',err='',settled=false,overflow=false
    const child=spawn(program,args,{cwd,env,windowsHide:true,shell:false,detached:process.platform!=='win32',stdio:['ignore','pipe','pipe']})
    const kill=()=>{try{process.kill(-child.pid,'SIGKILL')}catch{child.kill('SIGKILL')}}
    const timeout=setTimeout(kill,timeoutMs)
    const abort=()=>kill()
    signal?.addEventListener('abort',abort,{once:true});if(signal?.aborted)abort()
    const add=(target,chunk)=>{
      if(Buffer.byteLength(target)+chunk.length>1024*1024){overflow=true;kill();return target}
      return target+chunk.toString('utf8')
    }
    child.stdout.on('data',b=>{out=add(out,b)});child.stderr.on('data',b=>{err=add(err,b)})
    const finish=value=>{if(settled)return;settled=true;clearTimeout(timeout);signal?.removeEventListener('abort',abort);resolve(value)}
    child.on('error',e=>finish({ok:false,output:'',error:redact(e.message)}))
    child.on('close',(code,sig)=>finish({ok:code===0&&!overflow,output:out,error:redact(err),exitCode:code,signal:sig,overflow}))
  })
}
export async function inspectCLI(request,env,signal) {
  const v=await probeCommand(request.command,['--version'],{cwd:request.cwd,env,signal})
  const match=/\b(\d+)\.(\d+)\.(\d+)\b/.exec(v.output)
  if(!v.ok||!match||Number(match[1])<1||Number(match[1])===1&&(Number(match[2])<5||Number(match[2])===5&&Number(match[3])<7))throw new Error('agy 1.5.7 or newer is required; --version was not confirmed')
  const help=await probeCommand(request.command,['--help'],{cwd:request.cwd,env,signal})
  const required=['--input-format','--output-format','--model','--effort','--conversation','--mode','--print-timeout','--disable-slash-commands','--dangerously-skip-permissions']
  if(request.outputSchema)required.push('--json-schema')
  if(request.toolPolicy==='evidence-only')required.push('--agent')
  if(request.sandbox)required.push('--sandbox')
  if(!help.ok||required.some(flag=>!help.output.includes(flag)))throw new Error('agy does not advertise the required headless capabilities')
  const models=await probeCommand(request.command,['models'],{cwd:request.cwd,env,signal})
  if(!models.ok)throw new Error('agy models failed: '+models.error)
  const slugs=models.output.split(/\r?\n/).map(l=>l.trim().split(/\s+/)[0]).filter(s=>/^[a-z][a-z0-9._-]+$/.test(s))
  if(!slugs.includes(request.model))throw new Error('requested model is absent from the native agy models catalog')
  return {version:match[0],models:slugs,source:'agy --version / --help / models'}
}
export async function doctor(input,runtime={}) {
  try {
    const request={command:command(input.command),cwd:directory(input.cwd),...(input.model?modelBinding(input.model,input.effort):{model:'gemini-4-hydrogen-high',effort:'high'})}
    const env=runtimeEnvironment(runtime),subscription=inspectSubscription(env)
    const cli=await inspectCLI(request,env,runtime.signal)
    return {protocol:1,type:'doctor',ok:true,cli,subscription,authentication:'unknown',remainingQuota:'unknown',inferencePerformed:false}
  } catch(error){return {protocol:1,type:'doctor',ok:false,error:redact(error.message),inferencePerformed:false}}
}
