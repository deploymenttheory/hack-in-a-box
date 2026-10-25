import {readFileSync} from 'node:fs'
import {request} from 'node:http'
import type {Browser} from '@playwright/test'
const path=process.env.HACKBOX_FREEZE_MANIFEST
if(!path)throw new Error('Run through TestSubmissionFreezeBrowsers with native PostgreSQL/runtime')
export const native=JSON.parse(readFileSync(path,'utf8')) as {event_id:string;team_id:string;admin:string;public:string;socket:string;viewport:'desktop'|'mobile'}
export function control<T=unknown>(action:string,extra:Record<string,string>={}){return new Promise<T>((resolve,reject)=>{const body=JSON.stringify({action,...extra}),r=request({socketPath:native.socket,path:'/',method:'POST',headers:{'Content-Type':'application/json','Content-Length':Buffer.byteLength(body)}},response=>{let text='';response.setEncoding('utf8').on('data',s=>text+=s);response.on('end',()=>{if(!response.statusCode||response.statusCode>=300)reject(new Error(`Native fixture control ${action}: ${response.statusCode} ${text}`));else resolve((text?JSON.parse(text):undefined) as T)})});r.on('error',reject);r.end(body)})}
export async function actor(browser:Browser,surface:'admin'|'public',role:'owner'|'mentor'|'competitor'){const cookie=await control<{Name:string;Value:string}>('session',{surface,actor:role}),context=await browser.newContext({baseURL:native[surface],viewport:native.viewport==='mobile'?{width:390,height:844}:{width:1440,height:1000}});await context.addCookies([{name:cookie.Name,value:cookie.Value,url:native[surface],httpOnly:true,sameSite:'Lax'}]);return context}
