// Render an actual authorized HTTP export; run from the repository root.
const {readFileSync,writeFileSync}=require('node:fs')
const {createServer}=require('node:http')
const {createRequire}=require('node:module')
const {createHash}=require('node:crypto')
const requireWeb=createRequire(require('node:path').resolve('web/package.json'))
const {chromium}=requireWeb('@playwright/test'),AxeBuilder=requireWeb('@axe-core/playwright').default
const base=require('node:path').resolve(process.argv[2] || '')+'/'
if(!process.argv[2])throw new Error('Usage: node docs/verification/certificates/print-browser.cjs <private-export-directory>')
const html=readFileSync(base+'certificate.html'),headers=JSON.parse(readFileSync(base+'certificate.html.headers.json','utf8'))
const assert=require('node:assert/strict')
let requests=0
const server=createServer((request,response)=>{requests++;response.writeHead(200,{'Content-Type':'text/html; charset=utf-8','Content-Security-Policy':headers.content_security_policy,'Cache-Control':headers.cache_control,'X-Content-Type-Options':'nosniff'});response.end(html)})
;(async()=>{
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve))
 const browser=await chromium.launch({headless:true}),cases=[]
 try{
  for(const width of [1440,390])for(const media of ['screen','print']){
   const context=await browser.newContext({viewport:{width,height:1000}}),page=await context.newPage();let external=0
   page.on('request',r=>{if(new URL(r.url()).hostname!=='127.0.0.1')external++})
   await page.emulateMedia({media});await page.goto(`http://127.0.0.1:${server.address().port}/certificate`)
   await page.getByRole('heading',{name:'Certificate of attendance',exact:true}).waitFor()
   assert.equal(await page.locator('html').getAttribute('lang'),'en')
   assert.equal(await page.getByRole('main').count(),1)
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
   assert.equal(await page.locator('script').count(),0)
   const fences=await page.evaluate(async()=>{
    const failures=[];document.addEventListener('securitypolicyviolation',event=>failures.push(event.effectiveDirective))
    const script=document.createElement('script');script.textContent='window.certificateUnexpectedScript=true';document.body.append(script)
    const image=new Image();image.alt='';image.src='/should-never-fetch-image';document.body.append(image)
    await new Promise(resolve=>setTimeout(resolve,50))
    const result={scriptExecuted:!!window.certificateUnexpectedScript,scriptsBlocked:failures.some(value=>value.startsWith('script-src')),imagesBlocked:failures.includes('img-src')};script.remove();image.remove();return result
   })
   assert.deepEqual(fences,{scriptExecuted:false,scriptsBlocked:true,imagesBlocked:true})
   const axe=await new AxeBuilder({page}).analyze();assert.deepEqual(axe.violations.filter(v=>['serious','critical'].includes(v.impact)).map(v=>v.id),[])
   assert.equal(external,0)
   cases.push({width,media,overflow:false,serious_critical_axe:0,external_requests:external,inline_scripts_blocked:true,image_fetch_blocked:true})
   await context.close()
  }
  const result={kind:'Actual native HTTP exported certificate HTML rendered in isolated Chromium; not app/OIDC or UI165 proof',html_sha256:createHash('sha256').update(html).digest('hex'),csp:headers.content_security_policy,cases,requests}
  writeFileSync(base+'browser-print.json',JSON.stringify(result,null,2)+'\n',{mode:0o600});console.log(JSON.stringify({passed:cases.length,html_sha256:result.html_sha256}))
 }finally{await browser.close();await new Promise(resolve=>server.close(resolve))}
})().catch(error=>{console.error(error);server.close();process.exitCode=1})
