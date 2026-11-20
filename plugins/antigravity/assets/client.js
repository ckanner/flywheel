// DSH's public lazy module, conversation view slot and settings form.
window.__ModuleLoader__.load({id:'dsh-subagent-antigravity',factory:function(require){
  const exports={},React=require('react'),h=React.createElement
  const zh=typeof navigator!=='undefined'&&navigator.language?.startsWith('zh')
  const t=zh?{title:'Antigravity 任务',prompt:'任务内容',label:'任务名称',cwd:'工作目录（留空沿用会话）',model:'模型',effort:'思考等级',mode:'执行模式',permission:'工具审批',sandbox:'请求原生终端沙箱',start:'开始任务',refresh:'刷新任务',cancel:'停止',result:'查看结果',doctor:'检查环境',resume:'继续会话',new:'新会话',settings:'Antigravity 默认配置',save:'保存',saved:'已保存，后续任务使用新默认值。',conflict:'保存冲突，草稿已保留。',hint:'任务直接调用官方 CLI；此面板不会调用父模型。任务在后台运行，切换视图不会取消。检查环境不发送推理请求。',scope:'仅分析给定证据（核验无工具）',unknown:'结果未知时先查看原调用，不要重复开始。',native:'原生权限配置',review:'需要审批的动作在无界面调用中会被拒绝',all:'预先批准全部工具动作',output:'结果 / 进度',loading:'处理中…'}:{title:'Antigravity tasks',prompt:'Task',label:'Task name',cwd:'Working directory (blank uses session)',model:'Model',effort:'Effort',mode:'Execution mode',permission:'Tool approval',sandbox:'Request native terminal sandbox',start:'Start task',refresh:'Refresh tasks',cancel:'Cancel',result:'View result',doctor:'Check environment',resume:'Resume conversation',new:'New conversation',settings:'Antigravity defaults',save:'Save',saved:'Saved. Future tasks use the new defaults.',conflict:'Save conflict. Your draft is preserved.',hint:'Tasks use the official CLI directly; this panel does not invoke the parent model. Tasks continue when you switch views. Environment checks send no inference request.',scope:'Analyze supplied evidence only (verified no tools)',unknown:'Inspect the original invocation before restarting an unknown task.',native:'Native permission configuration',review:'Actions needing approval are denied in headless mode',all:'Preapprove all tool actions',output:'Result / progress',loading:'Working…'}
  const defaults={command:'agy',model:'gemini-4-hydrogen-high',effort:'high',executionMode:'default',permissionMode:'request-review',sandbox:false,timeoutMs:600000,disposeGraceMs:2000,presets:['daily','heavy','cordis']}
  const models=['gemini-4-hydrogen-high','gemini-4-neon-high','gemini-4-carbon-high']
  const field=(label,element)=>h('label',{style:{display:'grid',gap:4}},label,element)
  const input=(label,value,onChange,type='text')=>field(label,h('input',{type,value,onChange:e=>onChange(type==='number'?Number(e.target.value):e.target.value)}))
  const select=(label,value,choices,onChange)=>field(label,h('select',{value,onChange:e=>onChange(e.target.value)},choices.map(option=>h('option',{key:typeof option==='string'?option:option[0],value:typeof option==='string'?option:option[0]},typeof option==='string'?option:option[1]))))
  function store(){
    const spec={init:()=>({initialized:false,prompt:'',label:'Antigravity task',cwd:'',model:defaults.model,effort:'high',executionMode:'default',permissionMode:'request-review',sandbox:false,toolPolicy:'native',sessionId:''}),actions:{set:(draft,key,value)=>{draft[key]=value}}}
    return {spec,create:()=>{let snapshot=spec.init();const listeners=new Set();return {getSnapshot:()=>snapshot,subscribe:fn=>{listeners.add(fn);return()=>listeners.delete(fn)},clearPersisted:()=>{},actions:{set:(key,value)=>{snapshot={...snapshot,[key]:value};for(const notify of listeners)notify()}}}}}
  }
  function TaskView({useStore,actions,execute}){
    const draft=useStore(v=>v),[busy,setBusy]=React.useState(false),[tasks,setTasks]=React.useState([]),[detail,setDetail]=React.useState(null),[message,setMessage]=React.useState(''),[catalog,setCatalog]=React.useState(models)
    const current=React.useRef(execute);current.current=execute
    const set=(key,value)=>actions.set(key,value)
    React.useEffect(()=>{
      let live=true;setTasks([]);setDetail(null);setMessage('');setBusy(false)
      if(!draft.initialized)void execute({version:1,action:'defaults'}).then(reply=>{
        if(!live)return
        for(const key of ['model','effort','executionMode','permissionMode','sandbox'])if(reply.defaults?.[key]!==undefined)actions.set(key,reply.defaults[key])
        if(reply.defaults?.defaultCwd)actions.set('cwd',reply.defaults.defaultCwd)
        actions.set('initialized',true)
      }).catch(error=>{if(live)setMessage(error.message)})
      return()=>{live=false;if(current.current===execute)current.current=null}
    },[execute])
    const call=async action=>{
      if(busy)return
      setBusy(true);setMessage('')
      try{
        const reply=await execute(action)
        if(current.current!==execute)return
        if(action.action==='list')setTasks(reply.tasks)
        else if(action.action==='doctor'){setDetail(reply);if(reply.cli?.models)setCatalog(reply.cli.models.filter(m=>/^gemini-4-(hydrogen|neon|carbon|argon)-(low|medium|high|xhigh)$/.test(m)))}
        else {setDetail(reply);setTasks(old=>[reply,...old.filter(item=>item.invocationId!==reply.invocationId)])}
      }catch(error){if(current.current===execute)setMessage(error.message+(action.action==='start'?' '+t.unknown:''))}finally{if(current.current===execute)setBusy(false)}
    }
    const start=()=>{
      const id=globalThis.crypto.randomUUID()
      call({version:1,action:'start',invocationId:id,prompt:draft.prompt,label:draft.label,...(draft.cwd?{cwd:draft.cwd}:{}),model:draft.model,effort:draft.effort,executionMode:draft.executionMode,permissionMode:draft.permissionMode,sandbox:draft.sandbox,toolPolicy:draft.toolPolicy,...(draft.sessionId?{sessionId:draft.sessionId}:{})})
    }
    return h('section',{style:{display:'grid',gap:12,padding:16,maxWidth:1100,margin:'auto',overflow:'auto'}},
      h('h2',null,t.title),h('p',null,t.hint),
      h('div',{style:{display:'grid',gridTemplateColumns:'repeat(auto-fit,minmax(220px,1fr))',gap:10}},input(t.label,draft.label,v=>set('label',v)),input(t.cwd,draft.cwd,v=>set('cwd',v)),select(t.model,draft.model,[...new Set([draft.model,...catalog])],v=>{set('model',v);set('effort',v.split('-').at(-1))}),select(t.effort,draft.effort,/carbon|argon/.test(draft.model)?['low','medium','high']:['low','medium','high','xhigh'],v=>{set('effort',v);set('model',draft.model.replace(/-(low|medium|high|xhigh)$/,'-'+v))}),select(t.mode,draft.executionMode,['default','accept-edits','plan'],v=>set('executionMode',v)),select(t.permission,draft.permissionMode,[['request-review',t.review],['always-proceed',t.all]],v=>set('permissionMode',v))),
      h('label',null,h('input',{type:'checkbox',checked:draft.sandbox,onChange:e=>set('sandbox',e.target.checked)}),' ',t.sandbox),
      h('label',null,h('input',{type:'checkbox',checked:draft.toolPolicy==='evidence-only',onChange:e=>set('toolPolicy',e.target.checked?'evidence-only':'native')}),' ',t.scope),
      draft.sessionId?h('p',null,t.resume+': '+draft.sessionId,' ',h('button',{type:'button',onClick:()=>set('sessionId','')},t.new)):null,
      field(t.prompt,h('textarea',{rows:7,value:draft.prompt,onChange:e=>set('prompt',e.target.value),style:{width:'100%',boxSizing:'border-box'}})),
      h('div',{style:{display:'flex',gap:8,flexWrap:'wrap'}},h('button',{type:'button',disabled:busy||!draft.prompt.trim(),onClick:start},t.start),h('button',{type:'button',disabled:busy,onClick:()=>call({version:1,action:'list'})},t.refresh),h('button',{type:'button',disabled:busy,onClick:()=>call({version:1,action:'doctor',...(draft.cwd?{cwd:draft.cwd}:{}),model:draft.model,effort:draft.effort})},t.doctor)),
      h('p',{role:'status','aria-live':'polite'},busy?t.loading:message),
      ...tasks.map(task=>h('article',{key:task.invocationId,style:{border:'1px solid currentColor',borderRadius:6,padding:10}},h('strong',null,task.label+' · '+task.status),h('p',null,task.requested?.model+' / '+task.requested?.effort+' · '+task.requested?.cwd),h('small',null,task.session?.id??task.invocationId),h('div',{style:{display:'flex',gap:8,marginTop:8}},h('button',{disabled:busy,onClick:()=>call({version:1,action:'result',invocationId:task.invocationId})},t.result),['running','cancelling'].includes(task.status)?h('button',{disabled:busy,onClick:()=>call({version:1,action:'cancel',invocationId:task.invocationId})},t.cancel):null,task.session?.resumable?h('button',{disabled:busy,onClick:()=>set('sessionId',task.session.id)},t.resume):null))),
      detail?h('details',{open:true},h('summary',null,t.output),h('pre',{style:{whiteSpace:'pre-wrap',overflowWrap:'anywhere'}},detail.output?.map(item=>item.text).join('\n')||detail.progress||JSON.stringify(detail,null,2)),h('details',null,h('summary',null,'Receipt'),h('pre',{style:{whiteSpace:'pre-wrap',overflowWrap:'anywhere'}},JSON.stringify(detail,null,2)))):null)
  }
  function Settings({view,form}){
    const state=form?.state,[draft,setDraft]=React.useState({...defaults,...state?.value}),[dirty,setDirty]=React.useState(false),[revision,setRevision]=React.useState(state?.revision),[busy,setBusy]=React.useState(false),[message,setMessage]=React.useState('')
    React.useEffect(()=>{if(!dirty&&!busy){setDraft({...defaults,...state?.value});setRevision(state?.revision)}},[state?.revision,dirty,busy])
    const set=(key,value)=>{if(!dirty)setRevision(state?.revision);setDraft({...draft,[key]:value});setDirty(true)}
    const enabled=state?.status==='ready'&&state.writable===true&&!busy
    const save=async()=>{setBusy(true);try{const value={...draft};if(!value.defaultCwd)delete value.defaultCwd;const operations=Object.keys(defaults).concat(['defaultCwd']).map(key=>value[key]===undefined?{op:'unset',path:[key]}:{op:'set',path:[key],value:value[key]});const accepted=await form.mutate(operations,revision);setMessage(accepted?t.saved:t.conflict);if(accepted)setDirty(false)}catch(error){setMessage(error.message)}finally{setBusy(false)}}
    if(view==='summary')return h('span',null,t.settings)
    return h('section',{style:{display:'grid',gap:10,maxWidth:800}},h('h3',null,t.settings),h('p',null,t.hint),input('agy command',draft.command,v=>set('command',v)),input(t.cwd,draft.defaultCwd??'',v=>set('defaultCwd',v)),input(t.model,draft.model,v=>set('model',v)),select(t.effort,draft.effort,['low','medium','high','xhigh'],v=>set('effort',v)),select(t.mode,draft.executionMode,['default','accept-edits','plan'],v=>set('executionMode',v)),select(t.permission,draft.permissionMode,['request-review','always-proceed'],v=>set('permissionMode',v)),h('label',null,h('input',{type:'checkbox',checked:draft.sandbox,onChange:e=>set('sandbox',e.target.checked)}),t.sandbox),input('Timeout (ms, 0 = unlimited)',draft.timeoutMs,v=>set('timeoutMs',v),'number'),input('Presets',draft.presets.join(', '),v=>set('presets',v.split(',').map(p=>p.trim()).filter(Boolean))),h('button',{disabled:!enabled||!dirty,onClick:save},t.save),h('p',{role:'status'},message))
  }
  exports.inject=['slots','remote','remote.commands']
  exports.apply=function(ctx){
    const bindings=new WeakMap()
    ctx.slots.inject('conversation.view',()=>ctx.slots.register({name:'conversation.view',id:'antigravity',order:1001,label:()=> 'Antigravity',store:store(),inject:(sessionId,actions)=>{
      let binding=bindings.get(actions)
      if(binding)return binding
      binding={execute:async request=>{
      const reply=await ctx.remote.commands.execute(sessionId,'/antigravity '+JSON.stringify(request),[])
      if(!reply?.ok)throw new Error(reply?.error?.message??'DSH command transport failed')
      const result=reply.value?.result
      if(result?.kind!=='success')throw new Error(result?.text??'Antigravity command unavailable')
      return JSON.parse(result.text)
      }};bindings.set(actions,binding);return binding
    }},TaskView))
    ctx.slots.inject('plugins.row.config',()=>ctx.slots.register({name:'plugins.row.config',key:'dsh-subagent-antigravity#antigravity'},Settings))
  }
  return exports
}})
