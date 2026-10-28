// Native OIDC/portal/guide transport checkpoint only. No fulfilled application
// routes, injected sessions or application-authorization overrides belong here.
import assert from 'node:assert/strict'
import {createRequire} from 'node:module'
import {dirname,resolve,join} from 'node:path'
import {fileURLToPath} from 'node:url'
import {writeFileSync} from 'node:fs'
const root=resolve(dirname(fileURLToPath(import.meta.url)),'../../..')
const require=createRequire(join(root,'web/package.json'))
const {chromium,expect}=require('@playwright/test')
const output=process.env.HACKBOX_LIFECYCLE_BROWSER_ARTIFACTS
const source=process.env.HACKBOX_LIFECYCLE_BROWSER_SOURCE
assert.ok(output);assert.match(source??'',/^[a-f0-9]{40}$/)
const origins={admin:'http://admin.localhost:8080',public:'http://public.localhost:8080',guide:'http://styleguide.localhost:8080'}
const browser=await chromium.launch({headless:true})
try{
 const context=await browser.newContext({viewport:{width:1440,height:1000},proxy:{server:process.env.HACKBOX_LIFECYCLE_BROWSER_PROXY??'http://127.0.0.1:18088',bypass:'<-loopback>'}})
 const errors=[]
 async function signIn(origin){
  const page=await context.newPage();page.on('pageerror',error=>errors.push(error.message))
  await page.goto(origin+'/auth/login')
  await expect(page.getByRole('heading',{name:'Synthetic identity provider',exact:true})).toBeVisible()
  await page.getByLabel('Synthetic actor',{exact:true}).selectOption('owner')
  await page.getByRole('button',{name:'Continue through normal OIDC',exact:true}).click()
  await expect(page.getByRole('heading',{name:'Your events',exact:true})).toBeVisible()
  const response=await page.evaluate(async()=>{const result=await fetch('/api/v1/session',{credentials:'same-origin'});return {status:result.status,value:await result.json()}});assert.equal(response.status,200)
  const capabilities=await page.evaluate(()=>({secure_context:window.isSecureContext,random_uuid:typeof crypto.randomUUID,structured_clone:typeof structuredClone}))
  writeFileSync(join(output,origin.includes('admin')?'admin-capabilities.json':'public-capabilities.json'),JSON.stringify(capabilities,null,2)+'\n',{mode:0o600})
  assert.equal(capabilities.secure_context,true);assert.equal(capabilities.random_uuid,'function');assert.equal(capabilities.structured_clone,'function')
  return {page,session:response.value}
 }
 const admin=await signIn(origins.admin),publicPortal=await signIn(origins.public)
 assert.equal(admin.session.person.id,publicPortal.session.person.id)
 assert.ok(admin.session.csrf_token!==publicPortal.session.csrf_token,'Normal portal CSRF values must be independent')
 await admin.page.screenshot({path:join(output,'native-admin-index.png'),fullPage:true})
 await publicPortal.page.setViewportSize({width:390,height:844})
 await publicPortal.page.screenshot({path:join(output,'native-public-index-mobile.png'),fullPage:true})
 const guide=await context.newPage();guide.on('pageerror',error=>errors.push(error.message));await guide.goto(origins.guide)
 await expect(guide.getByRole('heading',{name:'Principles',exact:true})).toBeVisible()
 await publicPortal.page.getByRole('button',{name:publicPortal.session.person.display_name,exact:true}).click()
 const logout=publicPortal.page.waitForResponse(r=>new URL(r.url()).pathname==='/api/v1/logout')
 await publicPortal.page.getByRole('menuitem',{name:'Sign out',exact:true}).click();assert.equal((await logout).status(),204)
 await admin.page.reload();await expect(admin.page.getByRole('heading',{name:'Your events',exact:true})).toBeVisible()
 assert.deepEqual(errors,[])
 writeFileSync(join(output,'browser-bridge-proof.json'),JSON.stringify({source_revision:source,kind:'native-browser-bridge-only',complete_lifecycle:false,normal_oidc_both_portals:true,same_native_person:true,independent_csrf:true,separate_public_logout:true,admin_session_survives:true,independent_guide:true,fulfilled_application_routes:0,session_injections:0,page_errors:0},null,2)+'\n',{mode:0o600})
 console.log('Native browser bridge: normal OIDC, independent portals/logout and guide PASS; complete lifecycle remains open.')
}finally{await browser.close()}
