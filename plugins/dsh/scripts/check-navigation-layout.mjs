// Optional real-browser visual regression check; no user profile or model calls.
// FLYWHEEL_PLAYWRIGHT_MODULE=/absolute/path/to/playwright/index.mjs node scripts/check-navigation-layout.mjs
import assert from 'node:assert/strict';
import { readFile, mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { pathToFileURL, fileURLToPath } from 'node:url';
import { transform } from 'lightningcss';
const { chromium } = await import(process.env.FLYWHEEL_PLAYWRIGHT_MODULE ? pathToFileURL(resolve(process.env.FLYWHEEL_PLAYWRIGHT_MODULE)).href : 'playwright');
const source = process.env.FLYWHEEL_LAYOUT_CSS || fileURLToPath(new URL('../src/client.module.css', import.meta.url));
const compiled = transform({ filename:'client.module.css', code:await readFile(source), cssModules:{pattern:'flywheel_[local]_[hash]'} });
const css=Object.fromEntries(Object.entries(compiled.exports).map(([key,value])=>[key,value.name]));
const output=resolve(process.env.FLYWHEEL_LAYOUT_OUTPUT || '/tmp/flywheel-navigation-layout');
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,...(process.env.FLYWHEEL_CHROME ? {executablePath:process.env.FLYWHEEL_CHROME}: {})});
const results=[];
try {
 for(const touch of [false,true]) for(const width of [320,800,1280]) {
  const context=await browser.newContext({viewport:{width,height:850},hasTouch:touch,isMobile:touch});
  const page=await context.newPage();
  const button=(id,text,selected=false,disabled=false)=>`<button id="${id}" class="${css.button} ${css.buttonSmall} ${css.buttonOutline}" aria-pressed="${selected}" ${disabled?'disabled':''}>${text}</button>`;
  await page.setContent(`<meta name="viewport" content="width=device-width,initial-scale=1"><style>:root{--dsw-alias-label-primary:#20242b;--dsw-alias-label-secondary:#68717d;--dsw-alias-border-l3:#e5e8ed;--dsw-alias-bg-layer-1:#fff;--dsw-alias-bg-layer-2:#f4f5f7;--dsw-alias-bg-module-platform:#f6f7f9;--dsw-alias-state-business-primary:#386ce8;--dsw-alias-interactive-bg-hover:#edf0f5}body{margin:0;background:white;font-family:system-ui}${compiled.code}</style><section class="${css.page}"><div class="${css.workbenchNav}"><nav class="${css.consoleTabs}">${button('flow','流程')}${button('history','记录',true)}</nav></div><div style="padding:16px"><div class="${css.segmented}">${button('canvas','画布',true)}${button('list','列表')}</div><div class="${css.inspectorTabs}">${button('detail','节点详情')}${button('evidence','证据',true)}</div><nav class="${css.configNav}" style="margin-top:16px">${button('settings','运行方式',true)}${button('models','模型与会话')}</nav>${button('disabled','不可操作',false,true)}</div></section>`);
  const measure=async id=>page.locator('#'+id).evaluate(el=>{const r=el.getBoundingClientRect(),s=getComputedStyle(el);return{width:r.width,height:r.height,padding:parseFloat(s.paddingLeft),radius:parseFloat(s.borderRadius),background:s.backgroundColor,outline:s.outlineStyle,marker:getComputedStyle(el,'::after').backgroundColor};});
  const coarse=await page.evaluate(()=>matchMedia('(pointer:coarse)').matches);
  for(const id of ['flow','history','canvas','list','detail','evidence','settings','models']) {
   await page.mouse.move(width-1,800);
   const before=await measure(id);
   await page.locator('#'+id).hover();
   const hover=await measure(id);
   assert.ok(before.padding>=8,`${id}: hover needs horizontal breathing room`);
   assert.ok(before.radius>=7,`${id}: hover must be rounded`);
   assert.equal(hover.height,before.height,`${id}: hover cannot resize a control`);
   assert.equal(hover.width,before.width,`${id}: hover cannot resize a control`);
   assert.ok(before.height >= (coarse?44:32) && before.height <= (coarse?44:36),`${id}: compact or touch target height, got ${before.height}`);
   if(!coarse && ['flow','detail','list'].includes(id)) assert.notEqual(hover.background,before.background,`${id}: hover must be visible`);
  }
  assert.notEqual((await measure('history')).marker,'rgba(0, 0, 0, 0)','selected tab keeps its marker');
  await page.locator('#flow').focus();
  assert.notEqual((await measure('flow')).outline,'none','keyboard focus remains visible');
  const disabled=await measure('disabled');await page.locator('#disabled').hover();assert.equal((await measure('disabled')).background,disabled.background,'disabled controls do not claim hover readiness');
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'no page overflow');
  await page.locator('#flow').hover();
  await page.screenshot({path:resolve(output,`${width}-${touch?'touch':'mouse'}.png`)});
  results.push({width,touch,coarse,pass:true});await context.close();
 }
 await writeFile(resolve(output,'report.json'),JSON.stringify(results,null,2)+'\n');console.log(JSON.stringify(results));
} finally { await browser.close(); }
