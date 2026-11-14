'use strict';
const test=require('node:test');
const assert=require('node:assert/strict');
const {registerDesktopPackage}=require('../install.js');
const version=require('../../package.json').version;
const OLD='@ckanner/dsh-plugin-flywheel',NEW='@ckanner/flywheel';
function fixture(options={}) {
 const state=new Map(options.empty?[]:[[OLD,{version:'0.32.1',from:OLD}]]);
 if(options.newExists)state.set(NEW,{version});
 const calls=[],controller=new AbortController();
 const run=async (cmd,args)=>{
  assert.equal(cmd,'/desktop dsh');assert.deepEqual(args.slice(0,3),['plugin','--profile','desktop']);calls.push(args);
  const [action,name]=args.slice(3);
  if(action==='list')return {code:0,stdout:JSON.stringify(options.malformed??[{dependencies:state.has(name)?{[name]:state.get(name)}:{}}])};
  if(action==='add'){
   assert.equal(args.at(-1),NEW+'@'+version);assert.ok(args.includes('--ignore-scripts'));
   if(options.failAdd){if(options.partialAdd)state.set(NEW,{version});return {code:1,stderr:'offline'};}
   state.set(NEW,{version});return {code:0,stdout:''};
  }
  assert.equal(action,'remove');
  if(name===OLD&&options.cancelRemove){state.delete(OLD);controller.abort();return {code:143,signal:'SIGTERM'};}
  if(name===OLD&&options.replaceDuringRemove)state.set(NEW,{version,from:'file:/concurrent/install'});
  if(name===OLD&&options.failRemove||name===NEW&&options.failRollback)return {code:1,stderr:'profile locked'};
  state.delete(name);return {code:0,stdout:''};
 };
 const item={};return {state,calls,controller,item,run:()=>registerDesktopPackage(run,'/desktop dsh','desktop',item,controller.signal)};
}
test('desktop migration installs and verifies the complete package before removing the old identity',async()=>{
 const f=fixture();await f.run();assert.deepEqual([...f.state.keys()],[NEW]);assert.equal(f.item.migration.status,'migrated');
 const changes=f.calls.filter(a=>['add','remove'].includes(a[3]));assert.equal(changes.length,2);assert.equal(changes[0][3],'add');assert.equal(changes[1][4],OLD);
 await f.run();assert.equal(f.calls.filter(a=>a[3]==='remove').length,1,'repeat setup never removes another package');
});
test('a failed replacement download leaves the old desktop package available',async()=>{
 const f=fixture({failAdd:true});await assert.rejects(f.run(),/registration/);assert.deepEqual([...f.state.keys()],[OLD]);assert.equal(f.calls.some(a=>a[3]==='remove'),false);
});
test('failed legacy removal rolls back only a newly added replacement',async()=>{
 const f=fixture({failRemove:true});await assert.rejects(f.run(),/removal/);assert.deepEqual([...f.state.keys()],[OLD]);assert.equal(f.item.migration.status,'rolled-back');
 const existing=fixture({failRemove:true,newExists:true});await assert.rejects(existing.run());assert.ok(existing.state.has(NEW));assert.equal(existing.calls.some(a=>a[3]==='remove'&&a[4]===NEW),false);
});
test('rollback failure stays visible and never reports migration complete',async()=>{
 const f=fixture({failRemove:true,failRollback:true});await assert.rejects(f.run());assert.equal(f.item.migration.status,'incomplete');assert.match(f.item.migration.rollbackError,/rollback/);assert.equal(f.state.size,2);
});
test('cancellation does not spawn rollback work or turn an uncertain remove into success',async()=>{
 const f=fixture({cancelRemove:true});await assert.rejects(f.run());assert.equal(f.item.migration.status,'incomplete');assert.equal(f.calls.at(-1)[3],'remove');assert.equal(f.calls.at(-1)[4],OLD);
});
test('malformed lists and custom legacy sources fail before profile mutations',async()=>{
 const malformed=fixture({malformed:{}});await assert.rejects(malformed.run(),/project list/);assert.equal(malformed.calls.some(a=>a[3]==='add'),false);
 const custom=fixture();custom.state.get(OLD).from='file:/custom/plugin';await assert.rejects(custom.run(),/custom source/);assert.equal(custom.calls.some(a=>a[3]==='add'),false);
 const customResolved=fixture();customResolved.state.get(OLD).resolved='file:/custom/plugin';await assert.rejects(customResolved.run(),/custom source/);assert.equal(customResolved.calls.some(a=>a[3]==='add'),false);
 const invalid=fixture();invalid.state.get(OLD).version='../../other';await assert.rejects(invalid.run(),/identity/);assert.equal(invalid.calls.some(a=>a[3]==='add'),false);
});

test('invalid dependency records stop before any profile write',async()=>{
 for (const document of [[null],[{dependencies:null}],[{dependencies:{[OLD]:null}}]]) {
  const f=fixture({malformed:document});
  await assert.rejects(f.run(),/dependencies|identity/);
  assert.equal(f.calls.some(a=>a[3]==='add'||a[3]==='remove'),false);
 }
});
test('rollback never removes an unconfirmed or concurrently replaced package',async()=>{
 const partial=fixture({failAdd:true,partialAdd:true});await assert.rejects(partial.run());
 assert.ok(partial.state.has(OLD));assert.ok(partial.state.has(NEW));
 assert.equal(partial.calls.some(a=>a[3]==='remove'),false);
 const replaced=fixture({failRemove:true,replaceDuringRemove:true});await assert.rejects(replaced.run());
 assert.equal(replaced.state.get(NEW).from,'file:/concurrent/install');
 assert.equal(replaced.calls.some(a=>a[3]==='remove'&&a[4]===NEW),false);
 assert.equal(replaced.item.migration.status,'incomplete');
});
