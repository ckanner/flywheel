import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import { webcrypto } from 'node:crypto'
import React from 'react'
import TestRenderer,{act} from 'react-test-renderer'

test('public client launches a direct owner-bound command, loads defaults without inference, and saves revisioned settings',async()=>{
  const slots=[],calls=[];let module
  vm.runInNewContext(readFileSync(new URL('../assets/client.js',import.meta.url),'utf8'),{window:{__ModuleLoader__:{load:entry=>{module=entry.factory(name=>{assert.equal(name,'react');return React})}}},navigator:{language:'en'},crypto:webcrypto,console})
  const ctx={slots:{inject:(_name,fn)=>fn(),register:(definition,component)=>{slots.push({definition,component})}},remote:{commands:{execute:async(session,command,attachments)=>{
    calls.push({session,command,attachments});const request=JSON.parse(command.slice('/antigravity '.length))
    const value=request.action==='defaults'?{version:1,defaults:{model:'gemini-4-neon-high',effort:'high'}}:{version:1,invocationId:request.invocationId,label:'test',status:'running',requested:{model:request.model,effort:request.effort,native:request.native,cwd:'/workspace'},...(request.native?{native:{mode:request.native.mode,observedAgents:1,limit:{maxObservedAgents:request.native.maxObservedAgents??null,hard:false,exceeded:false},completion:{state:'unknown'},children:[{id:'aaaaaaaa-1111-2222-3333-444444444444',kind:'worker',role:'Implement bounded task',state:'unknown'}]}}:{})}
    return {ok:true,value:{result:{kind:'success',text:JSON.stringify(value)}}}
  }}}}
  module.apply(ctx)
  const view=slots.find(s=>s.definition.name==='conversation.view'),instance=view.definition.store.create();instance.actions.set('prompt','Inspect task')
  const binding=view.definition.inject('session-a',instance.actions)
  assert.equal(view.definition.inject('session-a',instance.actions),binding)
  let renderer
  const App=()=>{const snapshot=React.useSyncExternalStore(instance.subscribe,instance.getSnapshot);return React.createElement(view.component,{...binding,actions:instance.actions,useStore:selector=>selector(snapshot)})}
  await act(async()=>{renderer=TestRenderer.create(React.createElement(App))})
  assert.equal(calls.length,1);assert.match(calls[0].command,/'?defaults/);assert.equal(instance.getSnapshot().model,'gemini-4-neon-high')
  const button=renderer.root.findAllByType('button').find(b=>b.children.includes('Start task'))
  await act(async()=>{await button.props.onClick()})
  const start=JSON.parse(calls[1].command.slice('/antigravity '.length));assert.equal(start.action,'start');assert.equal(start.model,'gemini-4-neon-high');assert.equal(calls[1].session,'session-a');assert.equal(calls[1].attachments.length,0)
  await act(async()=>{renderer.root.findAllByType('select').find(s=>s.props.value==='gemini-4-neon-high').props.onChange({target:{value:'gemini-pro-orchestrator'}})})
  const effort=renderer.root.findAllByType('select').find(s=>s.props.disabled&&s.props.value==='');assert.ok(effort)
  await act(async()=>{renderer.root.findAllByType('select').find(s=>s.props.value==='native').props.onChange({target:{value:'swarm'}})})
  await act(async()=>{renderer.root.findAllByType('input').find(i=>i.props.min===1).props.onChange({target:{value:'4'}})})
  await act(async()=>{await renderer.root.findAllByType('button').find(b=>b.children.includes('Start task')).props.onClick()})
  const swarm=JSON.parse(calls.at(-1).command.slice('/antigravity '.length));assert.equal(swarm.model,'gemini-pro-orchestrator');assert.equal(swarm.effort,'')
  assert.deepEqual(swarm.native,{version:1,mode:'swarm',maxObservedAgents:4});assert.ok(renderer.root.findAllByType('td').some(cell=>cell.children.includes('Unknown')))
  await act(async()=>{renderer.root.findAllByType('input').find(i=>i.props.type==='checkbox'&&i.parent.children.some(child=>typeof child==='string'&&child.includes('Analyze supplied evidence'))).props.onChange({target:{checked:true}})})
  assert.equal(renderer.root.findAllByType('button').find(b=>b.children.includes('Start task')).props.disabled,true)
  const settings=slots.find(s=>s.definition.name==='plugins.row.config');let operations,revision
  const form={state:{status:'ready',writable:true,revision:7,value:{}},mutate:async(ops,rev)=>{operations=ops;revision=rev;return true}}
  let config
  await act(async()=>{config=TestRenderer.create(React.createElement(settings.component,{view:'edit',form}))})
  await act(async()=>{config.root.findAllByType('input')[0].props.onChange({target:{value:'/opt/agy'}})})
  await act(async()=>{config.root.findAllByType('input')[2].props.onChange({target:{value:'gemini-pro-orchestrator-neon'}})})
  await act(async()=>{config.root.findAllByType('select').find(s=>s.props.value==='native').props.onChange({target:{value:'subagents'}})})
  await act(async()=>{await config.root.findAllByType('button').find(b=>b.children.includes('Save')).props.onClick()})
  assert.equal(revision,7);assert.equal(operations.find(o=>o.path[0]==='command').value,'/opt/agy')
  assert.equal(operations.find(o=>o.path[0]==='model').value,'gemini-pro-orchestrator-neon');assert.equal(operations.find(o=>o.path[0]==='effort').value,'')
  assert.equal(operations.find(o=>o.path[0]==='native').value.mode,'subagents')
  await act(async()=>{renderer.unmount();config.unmount()})
})
