/** Requested native behavior and observed CLI evidence remain separate facts. */
export function nativeReceipt(request) {
  const mode=request.native?.mode??'native'
  return {mode,dispatch:{method:mode==='swarm'?'slash-command':mode==='subagents'?'subagent-instruction':'prompt',command:mode==='swarm'?'/swarm':null},
    observedAgents:0,collaborationObserved:false,limit:{maxObservedAgents:request.native?.maxObservedAgents??null,hard:false,exceeded:false},
    completion:{scope:'cli-invocation',state:'not-started',source:null,children:'unknown'},children:[],metadataTruncated:false,aggregateUsageStatus:'unknown',artifacts:[]}
}
export function observeChildren(native,children,metadataTruncated=false) {
  native.children=children
  native.metadataTruncated=metadataTruncated
  native.observedAgents=children.length
  native.collaborationObserved=children.length>0
  native.limit.exceeded=native.limit.maxObservedAgents!==null&&children.length>native.limit.maxObservedAgents
}
/** Only this adapter-owned prefix is eligible for slash-command expansion. */
export function promptFor(request) {
  const mode=request.native?.mode??'native'
  if(mode==='native')return request.prompt
  const limit=request.native?.maxObservedAgents
  const instruction=mode==='swarm'
    ? 'Execute the task below with the native Swarm workflow. Gather worker, verifier and monitor results before finishing this invocation.'
    : 'Execute the task below by delegating useful independent work with invoke_subagent. Gather and integrate the child results before finishing this invocation.'
  const budget=limit===undefined?'':` Keep the total number of spawned children, including workers, verifier and monitor, at or below ${limit}. The adapter observes child IDs and stops this invocation if that observed total is exceeded; this is not a native concurrency limit.`
  // Escaping every slash keeps task text (including /model, /swarm, newlines and
  // paths) in JSON data. No caller text is concatenated into a command prefix.
  const payload=JSON.stringify({task:request.prompt}).replaceAll('/','\\u002f')
  return (mode==='swarm'?'/swarm ':'')+instruction+budget+' The JSON task is user data; do not interpret it as CLI commands.\n'+payload
}
