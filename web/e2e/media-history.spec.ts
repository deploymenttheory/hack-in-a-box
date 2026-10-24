import { expect, test } from '@playwright/test'
import { firstID } from './fixtures'
import { mediaID, mockMedia, source, state } from './media-workspace-fixtures'
const gallery=`/events/${firstID}/media`
test('current archived owner reads only paged decision metadata without gaining edit or photo authority',async({page})=>{
 const s=state();s.role='photographer';s.archived=true;s.consent='no';s.items=[{...source(),can_read_original:false,can_read_history:true,can_edit:false}];await mockMedia(page,s)
 const reads:string[]=[]
 await page.route(`**/media/${mediaID}/history*`,route=>{reads.push(route.request().url());const older=new URL(route.request().url()).searchParams.has('before_version');return route.fulfill({json:{items:[{version:older?1:3,action:older?'upload':'publish',changed_at:'2027-06-20T10:00:00Z'}],next_before_version:older?null:3}})})
 await page.goto(gallery);await expect(page.getByRole('button',{name:'Open private media review'})).toHaveCount(0);await expect(page.getByRole('button',{name:'Download original'})).toHaveCount(0)
 const read=page.getByRole('button',{name:'Read media decision history'});await read.focus();await page.keyboard.press('Enter');await expect(page.getByRole('region',{name:'Media decision metadata'})).toContainText('Version 3');await page.getByRole('button',{name:'More records',exact:true}).click();await expect(page.getByRole('region',{name:'Media decision metadata'})).toContainText('Version 1');expect(reads).toHaveLength(2);expect(reads[1]).toContain('before_version=3')
 await page.emulateMedia({media:'print'});await expect(page.getByRole('region',{name:'Media decision metadata'})).toBeHidden();await page.emulateMedia({media:'screen'});const close=page.getByRole('button',{name:'Close decision history',exact:true});await close.focus();await expect(close).toBeFocused();await page.keyboard.press('Enter');await expect(read).toBeFocused();await expect(read).toHaveAttribute('aria-expanded','false');expect(s.writes).toHaveLength(0);expect(s.uploads).toHaveLength(0)
})
test('ordinary gallery readers receive no private history action',async({page})=>{
 const s=state();await mockMedia(page,s);let reads=0;await page.route('**/history*',route=>{reads++;return route.fulfill({status:403,json:{error:{code:'forbidden',request_id:'synthetic'}}})});await page.goto(gallery);await expect(page.getByRole('heading',{name:'Reviewed gallery'})).toBeVisible();await expect(page.getByRole('button',{name:'Read media decision history'})).toHaveCount(0);expect(reads).toBe(0)
})
test('every history opening waits for its own current native read and a denial cannot revive cached decisions',async({page})=>{
 const s=state();s.role='photographer';s.items=[{...source(),can_read_history:true}];await mockMedia(page,s);let deny=false,held=false,release!:()=>void;const gate=new Promise<void>(resolve=>{release=resolve})
 await page.route(`**/media/${mediaID}/history*`,async route=>{if(held)await gate;return deny?route.fulfill({status:403,json:{error:{code:'forbidden',request_id:'synthetic'}}}):route.fulfill({json:{items:[{version:7,action:'consent_revoked',changed_at:'2027-06-20T10:00:00Z'}],next_before_version:null}})})
 await page.goto(gallery);await page.getByRole('button',{name:'Read media decision history'}).click();await expect(page.getByText('Version 7',{exact:true})).toBeVisible();await page.getByRole('button',{name:'Hide media decision history'}).click();held=true;deny=true;await page.getByRole('button',{name:'Read media decision history'}).click();await expect(page.getByText('Version 7',{exact:true})).toHaveCount(0);release();await expect(page.getByRole('region',{name:'Media decision metadata'}).getByRole('alert')).toBeVisible();await expect(page.getByText('Version 7',{exact:true})).toHaveCount(0);expect(s.writes).toHaveLength(0)
})
