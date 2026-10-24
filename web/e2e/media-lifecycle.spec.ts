import { expect, test, type Page } from '@playwright/test'
import { firstID } from './fixtures'
import { mediaID, mockMedia, png, source, state } from './media-workspace-fixtures'
const gallery=`/events/${firstID}/media`
async function draft(page:Page,title='Synthetic private collection'){
 await page.getByRole('button',{name:'Add media',exact:true}).click()
 await page.getByLabel('Media title',{exact:true}).fill(title)
 await page.getByLabel('Submission reason',{exact:true}).fill('PRIVATE-REASON-CANARY')
 await page.getByLabel('Photograph (PNG or JPEG, up to 8 MiB)').setInputFiles({name:'synthetic.png',mimeType:'image/png',buffer:png})
}
async function submit(page:Page){await page.getByRole('button',{name:'Review private media draft',exact:true}).click();await page.getByRole('button',{name:'Confirm private media draft',exact:true}).click()}
async function backToMedia(page:Page){const related=page.locator('details').filter({has:page.locator('summary').filter({hasText:/^Related event workspaces/})});await related.locator('summary').click();await related.locator(`a[href="${gallery}"]`).click();await expect(page).toHaveURL(gallery)}
test('keyboard review/correction and local/route discard preserve exact private draft until deliberate discard',async({page})=>{
 const s=state();s.role='photographer';s.items=[];await mockMedia(page,s);await page.goto(gallery)
 const add=page.getByRole('button',{name:'Add media',exact:true});await add.focus();await expect(add).toBeFocused();await page.keyboard.press('Enter')
 await page.getByLabel('Media type',{exact:true}).selectOption('recording');await page.getByLabel('Media title',{exact:true}).fill('Private recording draft');await page.getByLabel('Recording HTTPS link').fill('https://synthetic.example.test/private-recording');await page.getByLabel('Submission reason').fill('PRIVATE-REASON-CANARY')
 const review=page.getByRole('button',{name:'Review private media draft',exact:true});await review.focus();await page.keyboard.press('Enter');await expect(page.getByRole('region',{name:'Review private recording submission'})).toContainText('Private recording draft');expect(s.writes).toHaveLength(0)
 await page.getByRole('button',{name:'Correct details',exact:true}).focus();await page.keyboard.press('Enter');await expect(review).toBeFocused();await expect(page.getByLabel('Recording HTTPS link')).toHaveValue('https://synthetic.example.test/private-recording')
 page.once('dialog',dialog=>void dialog.dismiss());await page.getByRole('button',{name:'Cancel media draft',exact:true}).click();await expect(page.getByLabel('Submission reason')).toHaveValue('PRIVATE-REASON-CANARY')
 page.once('dialog',dialog=>void dialog.dismiss());await page.getByRole('link',{name:'Back to event',exact:true}).click();await expect(page).toHaveURL(gallery);await expect(page.getByLabel('Media title',{exact:true})).toHaveValue('Private recording draft')
 page.once('dialog',dialog=>void dialog.accept());await page.getByRole('button',{name:'Cancel media draft',exact:true}).click();await expect(add).toBeFocused();expect(s.writes).toHaveLength(0)
})
test('an unknown upload survives actual SPA event navigation without private payload resurrection or replacement',async({page})=>{
 const s=state();s.role='photographer';s.items=[];s.lostUpload=true;await mockMedia(page,s);await page.goto(gallery);await page.evaluate(()=>Object.assign(window,{mediaDocumentMarker:'same-document'}));await draft(page,'PRIVATE-TITLE-CANARY');await submit(page)
 await expect(page.getByText(/The response was lost/)).toBeVisible();expect(s.uploads).toHaveLength(1)
 page.once('dialog',dialog=>void dialog.accept());await page.getByRole('link',{name:'Back to event',exact:true}).click();await expect(page).toHaveURL(`/events/${firstID}`);await backToMedia(page)
 expect(await page.evaluate(()=>(window as unknown as {mediaDocumentMarker?:string}).mediaDocumentMarker)).toBe('same-document')
 await expect(page.getByRole('button',{name:'Add media',exact:true})).toBeDisabled();await expect(page.getByRole('complementary',{name:'Unconfirmed command history'})).toContainText('Original outcome remains unconfirmed');await expect(page.getByLabel('Submission reason')).toHaveCount(0)
 expect(s.uploads).toHaveLength(1);const stored=await page.evaluate(()=>JSON.stringify([Object.entries(localStorage),Object.entries(sessionStorage)]));expect(stored).not.toContain('PRIVATE-TITLE-CANARY');expect(stored).not.toContain('PRIVATE-REASON-CANARY')
})
for(const response of ['acknowledged','lost'] as const)test(`held upload ${response} after capability teardown settles only actual original acknowledgement across SPA routes`,async({page})=>{
 const s=state();s.role='photographer';s.items=[];await mockMedia(page,s);let release!:()=>void,started=false;const gate=new Promise<void>(resolve=>{release=resolve})
 await page.route('**/media/upload',async route=>{started=true;await gate;const input=route.request().postDataJSON();s.uploads.push({key:route.request().headers()['idempotency-key'],input});const item={...source(),id:input.id,title:'Current safe synthetic record',can_edit:true};s.items=[item];return response==='acknowledged'?route.fulfill({json:{...item,acknowledged_version:1}}):route.abort('failed')})
 await page.goto(gallery);await draft(page);await submit(page);await expect.poll(()=>started).toBe(true)
 s.role='attendee';await page.evaluate(()=>window.dispatchEvent(new Event('visibilitychange')));await expect(page.getByRole('heading',{name:'Reviewed gallery',exact:true})).toBeVisible();await expect(page.getByText('PRIVATE-REASON-CANARY',{exact:true})).toHaveCount(0)
 await page.getByRole('link',{name:'Back to event',exact:true}).click();await expect(page).toHaveURL(`/events/${firstID}`);s.role='photographer';await backToMedia(page)
 await expect(page.getByRole('button',{name:'Add media',exact:true})).toBeDisabled();await expect(page.getByText('Waiting for the earlier media change to finish. A replacement cannot be started.',{exact:true})).toBeVisible();release()
 if(response==='acknowledged'){await expect(page.getByRole('button',{name:'Add media',exact:true})).toBeEnabled();await expect(page.getByText(/Original intent accepted at media version/)).toHaveCount(0)}
 else {await expect(page.getByRole('complementary',{name:'Unconfirmed command history'})).toContainText('Original outcome remains unconfirmed');await expect(page.getByRole('button',{name:'Add media',exact:true})).toBeDisabled()}
 expect(s.uploads).toHaveLength(1)
})
test('own-consent lost response cannot be retried or settled by reading the current choice',async({page})=>{
 const s=state();await mockMedia(page,s);let writes=0
 await page.route('**/photo-consent',route=>{if(route.request().method()==='PUT'){writes++;s.consent='no';s.choiceRevision++;return route.abort('failed')}return route.fulfill({json:{choice:s.consent,revision:s.choiceRevision,can_opt_in:true,can_revoke:true}})})
 await page.goto(gallery);await page.getByRole('button',{name:'Do not allow identifiable media',exact:true}).click();expect(writes).toBe(0);await page.getByRole('button',{name:'Confirm photo consent choice',exact:true}).click();await expect(page.getByText(/The original outcome is unknown/)).toBeVisible()
 await page.getByRole('button',{name:'Check current photo consent',exact:true}).click();await expect(page.getByText('Current choice:',{exact:false})).toContainText('no');await expect(page.getByRole('button',{name:'Confirm photo consent choice',exact:true})).toBeDisabled();await page.getByRole('button',{name:'Correct details',exact:true}).click();await expect(page.getByRole('button',{name:'Allow identifiable event media',exact:true})).toBeDisabled();expect(writes).toBe(1)
 page.once('dialog',dialog=>void dialog.accept());await page.getByRole('link',{name:'Back to event',exact:true}).click();await backToMedia(page);await expect(page.getByRole('button',{name:'Do not allow identifiable media',exact:true})).toBeDisabled();expect(writes).toBe(1)
})
test('a held protected thumbnail cannot create an object URL after current consent removes its record',async({page})=>{
 const s=state();await mockMedia(page,s);let release!:()=>void,started=false;const gate=new Promise<void>(resolve=>{release=resolve})
 await page.addInitScript(()=>{const original=URL.createObjectURL;Object.assign(window,{mediaObjectURLs:0});URL.createObjectURL=function(value){(window as unknown as {mediaObjectURLs:number}).mediaObjectURLs++;return original.call(this,value)}})
 await page.route('**/thumbnail',async route=>{started=true;await gate;return route.fulfill({contentType:'image/png',body:png})})
 await page.goto(gallery);await page.getByRole('button',{name:'View protected photo',exact:true}).click();await expect.poll(()=>started).toBe(true);s.consent='no';await page.evaluate(()=>window.dispatchEvent(new Event('visibilitychange')));await expect(page.getByText('No media is available in this view.')).toBeVisible();release();await page.waitForTimeout(100)
 expect(await page.evaluate(()=>(window as unknown as {mediaObjectURLs:number}).mediaObjectURLs)).toBe(0);await expect(page.locator('.media-photo img')).toHaveCount(0)
})
test('withheld recording URL at fresh200 fences an earlier held publish continuation even while can_edit remains true',async({page})=>{
 const s=state();s.role='organiser';s.items=[{...source(),kind:'recording',recording_url:'https://synthetic.example.test/PRIVATE-DESTINATION',can_edit:true,can_review:true,can_publish:true,can_withdraw:true}];await mockMedia(page,s);let release!:()=>void;const gate=new Promise<void>(resolve=>{release=resolve})
 await page.goto(gallery);await page.getByRole('button',{name:'Open private media review',exact:true}).click();await page.getByLabel('Reason for this action').fill('PRIVATE-REASON-CANARY');await page.getByRole('button',{name:'Review gallery release',exact:true}).click();s.deferSession=()=>gate;await page.getByRole('button',{name:'Confirm explicit gallery release',exact:true}).click();await expect(page.getByText('Checking current authority for this exact reviewed intent…')).toBeVisible()
 s.items=[{...s.items[0],recording_url:'',can_publish:false}];await page.evaluate(()=>window.dispatchEvent(new Event('visibilitychange')));await expect(page.getByLabel('Recording HTTPS link')).toHaveCount(0);await expect(page.getByText('PRIVATE-REASON-CANARY',{exact:true})).toHaveCount(0);release();await page.waitForTimeout(100);expect(s.writes).toHaveLength(0)
})
test('parent denial and only a restored empty list cannot resurrect private detail, editor or old protected URL',async({page})=>{
 const s=state();s.role='photographer';s.items=[{...source(),title:'PRIVATE-RECORD-CANARY',can_edit:true,can_read_history:true}];await mockMedia(page,s);await page.goto(`${gallery}/${mediaID}`);await page.getByRole('button',{name:'Open private media review',exact:true}).click();await page.getByLabel('Reason for this action').fill('PRIVATE-REASON-CANARY');await page.getByRole('button',{name:'View protected photo',exact:true}).click();await expect(page.locator('.media-photo img')).toBeVisible()
 s.denied=true;await page.evaluate(()=>window.dispatchEvent(new Event('visibilitychange')));await expect(page.getByText(/Current media access could not be verified/)).toBeVisible();s.denied=false;s.items=[];await page.getByRole('button',{name:'Try again',exact:true}).click();await expect(page.getByText('PRIVATE-RECORD-CANARY',{exact:true})).toHaveCount(0);await expect(page.getByText('PRIVATE-REASON-CANARY',{exact:true})).toHaveCount(0);await expect(page.locator('.media-photo img')).toHaveCount(0)
 await page.clock.install();await page.clock.fastForward(16_000);await expect(page.getByRole('button',{name:'Open private media review',exact:true})).toHaveCount(0);await expect(page.getByRole('button',{name:'Read media decision history',exact:true})).toHaveCount(0)
})
test('unsafe file metadata and UTF8 title limits fail before any native upload or recording write',async({page})=>{
 const s=state();s.role='photographer';s.items=[];await mockMedia(page,s);await page.goto(gallery);await draft(page)
 const file=page.getByLabel('Photograph (PNG or JPEG, up to 8 MiB)'),title=page.getByLabel('Media title',{exact:true}),reason=page.getByLabel('Submission reason',{exact:true}),review=page.getByRole('button',{name:'Review private media draft',exact:true})
 await file.setInputFiles({name:'not-a-photo.svg',mimeType:'image/svg+xml',buffer:Buffer.from('<svg/>')});await review.click()
 await expect(file).toBeFocused();await expect(file).toHaveAttribute('aria-invalid','true');await expect(file).toHaveAttribute('aria-describedby',/media-file-error/);await expect(file).toHaveAccessibleDescription(/Choose a PNG or JPEG photograph up to 8 MiB/)
 await file.setInputFiles({name:'synthetic.png',mimeType:'image/png',buffer:png});await title.fill('😀'.repeat(60));await review.click()
 await expect(title).toBeFocused();await expect(title).toHaveAttribute('aria-invalid','true');await expect(title).toHaveAttribute('aria-describedby',/new-media-title-error/);await expect(title).toHaveAccessibleDescription(/Provide a media title up to 200 UTF-8 bytes/)
 await title.fill('Valid synthetic title');await reason.fill('');await review.click();await expect(reason).toBeFocused();await expect(reason).toHaveAttribute('aria-invalid','true');await expect(reason).toHaveAccessibleDescription(/Provide a reason up to 2000 UTF-8 bytes/)
 await reason.fill('Synthetic recording review');await page.getByLabel('Media type',{exact:true}).selectOption('recording');const url=page.getByLabel('Recording HTTPS link',{exact:true});await url.fill('http://synthetic.example.test/recording');await review.click();await expect(url).toBeFocused();await expect(url).toHaveAttribute('aria-invalid','true');await expect(url).toHaveAccessibleDescription(/Provide a valid HTTPS recording destination/)
 expect(s.uploads).toHaveLength(0);expect(s.writes).toHaveLength(0)
 page.once('dialog',dialog=>void dialog.accept());await page.getByRole('button',{name:'Cancel media draft',exact:true}).click();s.items=[{...source(),can_edit:true}];await page.evaluate(()=>window.dispatchEvent(new Event('visibilitychange')))
 await page.getByRole('button',{name:'Open private media review',exact:true}).click();const correction=page.getByRole('button',{name:'Review metadata correction',exact:true}),currentTitle=page.getByLabel('Media title',{exact:true}),currentReason=page.getByLabel('Reason for this action',{exact:true})
 await currentTitle.fill('😀'.repeat(60));await currentReason.fill('Synthetic metadata correction');await correction.click();await expect(currentTitle).toBeFocused();await expect(currentTitle).toHaveAttribute('aria-invalid','true');await expect(currentTitle).toHaveAccessibleDescription(/Provide a media title up to 200 UTF-8 bytes/)
 await currentTitle.fill('Corrected synthetic title');await currentReason.fill('');await correction.click();await expect(currentReason).toBeFocused();await expect(currentReason).toHaveAttribute('aria-invalid','true');await expect(currentReason).toHaveAccessibleDescription(/Provide a reason up to 2000 UTF-8 bytes/)
 expect(s.uploads).toHaveLength(0);expect(s.writes).toHaveLength(0)
})
