import { expect, test } from '@playwright/test'
import type { components } from '../src/api/schema.gen'
import { firstID } from './fixtures'
import { surveyFixture, surveyPath, managePath, privateText, questionID, textID } from './survey-workspace-fixtures'

test('mobile and keyboard completion explains visibility and saves only visible answers',async({page})=>{
 const f=await surveyFixture(page);await page.goto(surveyPath)
 await expect(page.getByRole('note')).toContainText('not anonymous')
 await page.getByLabel('Did you learn something? (required)').selectOption('yes')
 await page.getByLabel('Private explanation (optional)').fill(privateText)
 await page.getByLabel('Did you learn something? (required)').selectOption('no')
 await page.getByRole('button',{name:'Save response',exact:true}).focus();await page.keyboard.press('Enter')
 await expect(page.getByRole('status',{name:'Survey response status'})).toContainText('Response command confirmed at revision 1')
 expect(f.puts).toHaveLength(1);expect(f.puts[0].answers).toEqual({[questionID]:'no'})
 await expect(page.getByText(privateText,{exact:true})).toHaveCount(0)
 expect(await page.evaluate(()=>JSON.stringify(localStorage)+JSON.stringify(sessionStorage))).not.toContain(privateText)
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
})
test('uncertain response retries its identical command without creating a second response',async({page})=>{
 const f=await surveyFixture(page);f.state.dropResponse=1
 await page.goto(surveyPath);await page.getByLabel('Did you learn something? (required)').selectOption('yes');await page.getByLabel('Private explanation (optional)').fill(privateText)
 await page.getByRole('button',{name:'Save response',exact:true}).click()
 await expect(page.getByRole('status',{name:'Survey response status'})).toContainText('outcome is unknown')
 await page.getByRole('button',{name:'Retry the same survey response',exact:true}).click()
 await expect(page.getByRole('status',{name:'Survey response status'})).toContainText('Response command confirmed at revision 1')
 expect(f.puts).toHaveLength(2);expect(f.puts[0]).toEqual(f.puts[1]);expect(f.commands.size).toBe(1)
})
test('older retry receipt does not replace a later saved response',async({page})=>{
 const f=await surveyFixture(page);f.state.dropResponse=1
 await page.goto(surveyPath);await page.getByLabel('Did you learn something? (required)').selectOption('yes');await page.getByLabel('Private explanation (optional)').fill('COMMAND-A')
 await page.getByRole('button',{name:'Save response',exact:true}).click();await expect(page.getByRole('status',{name:'Survey response status'})).toContainText('outcome is unknown')
 f.state.response={revision:2,form_version:1,answers:{[questionID]:'yes',[textID]:'LATER-COMMAND-B'},submitted_at:'2027-06-22T10:00:00Z'}
 await page.getByRole('button',{name:'Retry the same survey response',exact:true}).click()
 await expect(page.getByRole('status',{name:'Survey response status'})).toContainText('confirmed at revision 1')
 await expect(page.getByRole('heading',{name:'Your saved response: revision 2',exact:false})).toBeVisible()
 await expect(page.getByText('LATER-COMMAND-B',{exact:true})).toBeVisible()
 await expect(page.getByRole('button',{name:'Save response',exact:true})).toBeDisabled()
 await page.getByRole('button',{name:'Load current saved response',exact:true}).click()
 await expect(page.getByLabel('Private explanation (optional)')).toHaveValue('LATER-COMMAND-B')
 expect(f.puts).toHaveLength(2)
})
test('close clears an uncertain local intent and retains authorised own history without retry',async({page})=>{
 const f=await surveyFixture(page);f.state.dropResponse=1;await page.clock.install()
 await page.goto(surveyPath);await page.getByLabel('Did you learn something? (required)').selectOption('yes');await page.getByLabel('Private explanation (optional)').fill(privateText)
 await page.getByRole('button',{name:'Save response',exact:true}).click();await expect(page.getByRole('status',{name:'Survey response status'})).toContainText('outcome is unknown')
 f.state.open=false;await page.clock.fastForward(16_000)
 await expect(page.getByText('Response editing is currently closed or unavailable.')).toBeVisible()
 await expect(page.getByRole('button',{name:'Retry the same survey response'})).toHaveCount(0)
 await expect(page.getByRole('heading',{name:'Your saved response: revision 1',exact:false})).toBeVisible();expect(f.puts).toHaveLength(1)
})
test('native access loss clears unsent text and stops all response effects',async({page})=>{
 const f=await surveyFixture(page);await page.clock.install();await page.goto(surveyPath)
 await page.getByLabel('Did you learn something? (required)').selectOption('yes');await page.getByLabel('Private explanation (optional)').fill(privateText)
 f.state.allowed=false;await page.clock.fastForward(16_000)
 await expect(page.getByRole('alert')).toBeVisible();await expect(page.getByLabel('Private explanation (optional)')).toHaveCount(0)
 expect(await page.evaluate(()=>JSON.stringify(localStorage)+JSON.stringify(sessionStorage))).not.toContain(privateText);expect(f.puts).toHaveLength(0)
})
test('small cohort report has no hidden cells or zero-as-unknown chart',async({page})=>{
 const f=await surveyFixture(page);f.state.small=true;await page.goto(surveyPath+'/results')
 await expect(page.getByText('Respondents: Suppressed;', {exact:false})).toBeVisible()
 await expect(page.getByText('Suppressed or text-only.',{exact:false})).toHaveCount(2)
 await expect(page.getByRole('table')).toHaveCount(0);await expect(page.getByText(privateText,{exact:true})).toHaveCount(0)
})
test('restricted staff text is absent from participant entry points',async({page})=>{
 await surveyFixture(page);await page.goto(managePath)
 await expect(page.getByRole('alert')).toBeVisible();await expect(page.getByText(privateText,{exact:true})).toHaveCount(0)
 await expect(page.getByRole('button',{name:'Restricted responses'})).toHaveCount(0)
 await page.goto(`/events/${firstID}/surveys`);await expect(page.getByRole('heading',{name:'Organiser survey administration'})).toHaveCount(0)
})

test('dirty question tab switch requires explicit consent and clears only acknowledged unsent state',async({page})=>{
 const f=await surveyFixture(page,true);f.configuration.published_at=null;f.configuration.approved_at=null
 await page.goto(managePath);await page.getByRole('button',{name:'Questions',exact:true}).click()
 await page.getByLabel('Form name',{exact:true}).fill(privateText)
 await page.getByRole('button',{name:'Questions',exact:true}).click();await expect(page.getByLabel('Form name',{exact:true})).toHaveValue(privateText)
 let dialog=page.waitForEvent('dialog'),switching=page.getByRole('button',{name:'Overview',exact:true}).click();await(await dialog).dismiss();await switching
 await expect(page.getByLabel('Form name',{exact:true})).toHaveValue(privateText)
 dialog=page.waitForEvent('dialog');switching=page.getByRole('button',{name:'Overview',exact:true}).click();await(await dialog).accept();await switching
 await expect(page.getByRole('heading',{name:'Restricted participation and completion'})).toBeVisible()
 await page.getByRole('button',{name:'Questions',exact:true}).click();await expect(page.getByLabel('Form name',{exact:true})).toHaveValue('Post-event learning')
 expect(f.configurationWrites).toHaveLength(0)
})
test('organisers explicitly review authored text, export privately and publish only reviewed prose',async({page},info)=>{
 const f=await surveyFixture(page,true);await page.goto(managePath)
 await page.getByRole('button',{name:'Restricted responses',exact:true}).click();await expect(page.getByText(privateText,{exact:true})).toBeVisible()
 const download=page.waitForEvent('download');await page.getByRole('link',{name:'Download restricted responses CSV'}).click();expect((await download).suggestedFilename()).toBe('restricted-survey-responses.csv')
 await page.getByRole('button',{name:'Reviewed report',exact:true}).click();await expect(page.getByText(privateText,{exact:true})).toHaveCount(0)
 await page.getByLabel('Redacted summary for wider release').fill('Reviewed public learning themes.')
 await page.getByLabel('I reviewed and redacted this wording', {exact:false}).check()
 await page.getByRole('button',{name:'Freeze cohort and save reviewed summary'}).click();await expect(page.getByRole('status',{name:'Survey report status'})).toContainText('not yet published')
 expect(f.state.publishedReport).toBeNull()
 await page.getByRole('button',{name:'Approve exact reviewed summary'}).click();await expect(page.getByRole('status',{name:'Survey report status'})).toContainText('approved, not yet published')
 await page.getByRole('button',{name:'Publish reviewed report'}).click();await expect(page.getByRole('status',{name:'Survey report status'})).toContainText('published')
 expect(f.reportWrites.map(command=>command.action)).toEqual(['review_text','approve','publish']);expect(JSON.stringify(f.reportWrites)).not.toContain(privateText)
 await page.goto(surveyPath+'/results');await expect(page.getByText('Reviewed public learning themes.',{exact:true})).toBeVisible();await expect(page.getByRole('table')).toHaveCount(1)
 await expect(page.getByText(privateText,{exact:true})).toHaveCount(0);expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:info.outputPath('reviewed-survey-report.png'),fullPage:true})
})
test('uncertain reviewed summary keeps its exact command across a refused local tab leave',async({page})=>{
 const f=await surveyFixture(page,true);f.state.dropReport=1;await page.goto(managePath)
 await page.getByRole('button',{name:'Reviewed report',exact:true}).click();await page.getByLabel('Redacted summary for wider release').fill('Reviewed summary with retained identity.')
 await page.getByLabel('I reviewed and redacted this wording',{exact:false}).check();await page.getByRole('button',{name:'Freeze cohort and save reviewed summary'}).click();await expect(page.getByRole('status',{name:'Survey report status'})).toContainText('outcome is unknown')
 const dialog=page.waitForEvent('dialog'),switching=page.getByRole('button',{name:'Overview',exact:true}).click();await(await dialog).dismiss();await switching
 await expect(page.getByLabel('Redacted summary for wider release')).toHaveValue('Reviewed summary with retained identity.')
 await page.getByRole('button',{name:'Retry the same report command'}).click();await expect(page.getByRole('status',{name:'Survey report status'})).toContainText('not yet published')
 expect(f.reportWrites).toHaveLength(2);expect(f.reportWrites[0]).toEqual(f.reportWrites[1]);expect(f.state.review?.version).toBe(1)
})
test('archive clears an uncertain uncommitted summary and disables all report mutations',async({page})=>{
 const f=await surveyFixture(page,true);f.state.dropReport=1;f.state.commitDroppedReport=false;await page.clock.install();await page.goto(managePath)
 await page.getByRole('button',{name:'Reviewed report',exact:true}).click();await page.getByLabel('Redacted summary for wider release').fill(privateText)
 await page.getByLabel('I reviewed and redacted this wording',{exact:false}).check();await page.getByRole('button',{name:'Freeze cohort and save reviewed summary'}).click();await expect(page.getByRole('status',{name:'Survey report status'})).toContainText('outcome is unknown')
 f.state.writable=false;await page.clock.fastForward(16_000)
 await expect(page.getByLabel('Redacted summary for wider release')).toHaveValue('');await expect(page.getByLabel('Redacted summary for wider release')).toBeDisabled()
 await expect(page.getByRole('button',{name:'Retry the same report command'})).toHaveCount(0);expect(f.reportWrites).toHaveLength(1)
 expect(await page.evaluate(()=>JSON.stringify(localStorage)+JSON.stringify(sessionStorage))).not.toContain(privateText)
})
test('current organiser loss clears private response and review surfaces',async({page})=>{
 const f=await surveyFixture(page,true);await page.clock.install();await page.goto(managePath)
 await page.getByRole('button',{name:'Restricted responses',exact:true}).click();await expect(page.getByText(privateText,{exact:true})).toBeVisible()
 f.state.allowed=false;await page.clock.fastForward(16_000)
 await expect(page.getByRole('alert')).toBeVisible();await expect(page.getByText(privateText,{exact:true})).toHaveCount(0)
 await expect(page.getByRole('link',{name:'Download restricted responses CSV'})).toHaveCount(0)
})
for(const changed of[false,true])test(`edition comparison ${changed?'blocks changed':'shows equivalent'} definitions at separate retained versions`,async({page})=>{
 const f=await surveyFixture(page,true);f.state.otherChanged=changed;f.state.otherVersion=changed?3:2
 await page.goto(managePath);await page.getByRole('button',{name:'Compare editions',exact:true}).click()
 await page.getByLabel('Other edition you currently organise').selectOption('b9eb455e-e9c8-488f-b16a-7da3f0f25b30')
 await page.getByLabel('Published survey in that edition').selectOption('8d4158fd-3ef9-42eb-9c0c-a064f34db9cd')
 if(changed){await expect(page.getByText('These question definitions, versions or measure provenance are not equivalent.',{exact:false})).toBeVisible();await expect(page.getByRole('table')).toHaveCount(0)}
 else{await expect(page.getByText('Equivalent question definitions retained at separate versions 1 and 2.',{exact:false})).toBeVisible();await expect(page.getByRole('table',{name:'Separate edition counts for the equivalent question'})).toBeVisible()}
 await expect(page.getByText(privateText,{exact:true})).toHaveCount(0)
})


test('transient response read failure preserves uncertain command until fresh authority succeeds',async({page})=>{
 const f=await surveyFixture(page);f.state.dropResponse=1;await page.clock.install();await page.goto(surveyPath)
 await page.getByLabel('Did you learn something? (required)').selectOption('yes');await page.getByLabel('Private explanation (optional)').fill(privateText)
 await page.getByRole('button',{name:'Save response',exact:true}).click();await expect(page.getByRole('status',{name:'Survey response status'})).toContainText('outcome is unknown')
 f.state.readFailure=true;await page.clock.fastForward(16_000)
 await expect(page.getByText('Current authority could not be refreshed.',{exact:false})).toBeVisible()
 await expect(page.getByLabel('Private explanation (optional)')).toHaveValue(privateText)
 await expect(page.getByRole('button',{name:'Retry the same survey response'})).toBeDisabled();expect(f.puts).toHaveLength(1)
 f.state.readFailure=false;await page.getByRole('button',{name:'Check saved response',exact:true}).click()
 await expect(page.getByRole('button',{name:'Retry the same survey response'})).toBeEnabled();await page.getByRole('button',{name:'Retry the same survey response'}).click()
 await expect(page.getByRole('status',{name:'Survey response status'})).toContainText('confirmed at revision 1');expect(f.puts).toHaveLength(2);expect(f.puts[1]).toEqual(f.puts[0])
})
test('transient organiser authority failure retains dirty text and resumes only after native refresh',async({page})=>{
 const f=await surveyFixture(page,true);f.configuration.published_at=null;f.configuration.approved_at=null;await page.clock.install();await page.goto(managePath)
 await page.getByRole('button',{name:'Questions',exact:true}).click();await page.getByLabel('Form name',{exact:true}).fill(privateText)
 f.state.readFailure=true;await page.clock.fastForward(16_000)
 await expect(page.getByText('Current survey authority could not be refreshed.',{exact:false})).toBeVisible();await expect(page.getByLabel('Form name',{exact:true})).toHaveValue(privateText)
 await expect(page.getByRole('button',{name:'Save question version',exact:true})).toBeDisabled();expect(f.configurationWrites).toHaveLength(0)
 f.state.readFailure=false;await page.getByRole('button',{name:'Refresh survey authority'}).click();await expect(page.getByLabel('Form name',{exact:true})).toBeEnabled();await expect(page.getByLabel('Form name',{exact:true})).toHaveValue(privateText)
})
test('transient report read failure retains unknown exact review without another write',async({page})=>{
 const f=await surveyFixture(page,true);f.state.dropReport=1;f.state.commitDroppedReport=false;await page.clock.install();await page.goto(managePath)
 await page.getByRole('button',{name:'Reviewed report',exact:true}).click();await page.getByLabel('Redacted summary for wider release').fill('Reviewed bounded themes.')
 await page.getByLabel('I reviewed and redacted this wording',{exact:false}).check();await page.getByRole('button',{name:'Freeze cohort and save reviewed summary'}).click()
 await expect(page.getByRole('status',{name:'Survey report status'})).toContainText('outcome is unknown')
 f.state.readFailure=true;await page.clock.fastForward(16_000)
 await expect(page.getByLabel('Redacted summary for wider release')).toHaveValue('Reviewed bounded themes.');await expect(page.getByRole('button',{name:'Retry the same report command'})).toBeDisabled();expect(f.reportWrites).toHaveLength(1)
 f.state.readFailure=false;await page.getByRole('button',{name:'Refresh survey authority'}).click();await page.getByRole('button',{name:'Check exact report operation'}).click()
 await expect(page.getByRole('button',{name:'Retry the same report command'})).toBeEnabled();await page.getByRole('button',{name:'Retry the same report command'}).click()
 await expect(page.getByRole('status',{name:'Survey report status'})).toContainText('not yet published');expect(f.reportWrites).toHaveLength(2);expect(f.reportWrites[1]).toEqual(f.reportWrites[0])
})


test('uncertain question save reconciles exact native version before separate approval and publication',async({page})=>{
 const f=await surveyFixture(page,true);f.configuration.published_at=null;f.configuration.approved_at=null;f.state.dropConfiguration=1
 await page.goto(managePath);await page.getByRole('button',{name:'Questions',exact:true}).click();await page.getByLabel('Form name',{exact:true}).fill('Reviewed learning questions')
 await page.getByRole('button',{name:'Save question version'}).click();await expect(page.getByRole('status',{name:'Survey configuration status'})).toContainText('outcome is unknown');expect(f.configurationWrites).toHaveLength(1)
 await page.getByRole('button',{name:'Check exact saved operation'}).click();await expect(page.getByRole('status',{name:'Survey configuration status'})).toContainText('No second write was sent');expect(f.configurationWrites).toHaveLength(1)
 await page.getByRole('button',{name:'Approve exact question version'}).click();await expect(page.getByRole('status',{name:'Survey configuration status'})).toContainText('not yet published');expect(f.configuration.published_at).toBeNull()
 await page.getByRole('button',{name:'Publish fixed invitations'}).click();await expect(page.getByRole('status',{name:'Survey configuration status'})).toContainText('No message has been sent')
 expect(f.configurationWrites.map(c=>(c as {action:string}).action)).toEqual(['save','approve','publish']);expect(f.automationWrites).toHaveLength(0)
})
test('native wall-time resolution uses event timezone before saving exact approved dates',async({page})=>{
 const f=await surveyFixture(page,true);f.configuration.published_at=null;f.configuration.approved_at=null;const resolves:unknown[]=[]
 await page.route('**/api/v1/time/resolve',async route=>{const body=route.request().postDataJSON();resolves.push(body);return route.fulfill({json:{instant:new Date(new Date(body.wall_time+'Z').getTime()-12*3_600_000).toISOString()}})})
 await page.goto(managePath);await page.getByRole('button',{name:'Questions',exact:true}).click();await page.getByLabel('Opens in Pacific/Auckland',{exact:true}).fill('2027-06-22T21:00')
 await page.getByRole('button',{name:'Save question version'}).click();await expect(page.getByRole('status',{name:'Survey configuration status'})).toContainText('Saved question version 2')
 expect(resolves).toHaveLength(2);expect(resolves).toEqual(expect.arrayContaining([expect.objectContaining({timezone:'Pacific/Auckland',wall_time:'2027-06-22T21:00:00'})]))
 expect(f.configuration.definition.opens_at).toBe('2027-06-22T09:00:00.000Z')
})
test('invitation binding preserves exact source and approved campaign with separate native approval',async({page})=>{
 const f=await surveyFixture(page,true);await page.goto(managePath);await page.getByRole('button',{name:'Invitations',exact:true}).click()
 await expect(page.getByText('Survey publication does not send a message.',{exact:false})).toBeVisible();await page.getByLabel('Approved invitation campaign and exact version').selectOption(f.campaign.campaign.definition.id)
 await page.getByLabel('Reason for this invitation decision').fill('Reviewed exact opening invitation')
 await page.getByRole('button',{name:'Save exact survey invitation binding'}).click();await expect(page.getByRole('status',{name:'Survey invitation status'})).toContainText('operation save recorded')
 expect(f.automation.bindings[0].approved_at).toBeNull();expect(f.automationWrites[0].definition).toMatchObject({campaign_version:3,trigger:'survey.open',source_id:f.configuration.id,source_version:1,retain_after_archive:false})
 await page.getByLabel('Reason for this invitation decision').fill('Approve current exact binding');await page.getByRole('button',{name:'Approve exact invitation binding'}).click()
 await expect(page.getByRole('status',{name:'Survey invitation status'})).toContainText('operation approve recorded');expect(f.automationWrites.map(c=>c.action)).toEqual(['save','approve'])
})
test('unknown invitation operation reconciles durable binding without automatic new command',async({page})=>{
 const f=await surveyFixture(page,true);f.state.dropAutomation=1;await page.goto(managePath);await page.getByRole('button',{name:'Invitations',exact:true}).click()
 await page.getByLabel('Approved invitation campaign and exact version').selectOption(f.campaign.campaign.definition.id);await page.getByLabel('Reason for this invitation decision').fill('Explicit invitation review')
 await page.getByRole('button',{name:'Save exact survey invitation binding'}).click();await expect(page.getByRole('status',{name:'Survey invitation status'})).toContainText('outcome is unknown')
 await page.getByRole('button',{name:'Check exact invitation operation'}).click();await expect(page.getByRole('status',{name:'Survey invitation status'})).toContainText('No second operation was sent')
 expect(f.automationWrites).toHaveLength(1);expect(f.automation.bindings).toHaveLength(1);expect(f.automation.bindings[0].approved_at).toBeNull()
})


function observation(basis:components['schemas']['OutcomeEvidence']['basis'],version:number,value:string|null):components['schemas']['OutcomeObservation']{
 const reference={objective:{id:'01c8a898-5703-4231-8c6e-95e1e062fcbd',version},measure_id:'339c1a9a-7dd7-4f3a-b550-bad4cab883ce'}
 return{outcome_id:'638c35ca-56f0-451c-b213-7aa0ee2d49e2',outcome_version:version,evidence:{id:`41d2ae8d-8d97-4ccd-822f-${String(version).padStart(12,'0')}`,basis,claim:`Retained ${basis} observation v${version}`,value,denominator:null,source:'Private organiser observation source',observed_at:basis==='demonstrated_adoption'?'2027-06-28T09:00:00Z':null,measures:[reference]},recorded_at:'2027-06-29T09:00:00Z',measures:[{reference,definition:{id:reference.measure_id,description:`Native retained definition v${version}`,unit:version===1?'people':'teams',method:'Retained native measurement method',evidence_source:'Configured review evidence source',baseline:null,target:version===1?'10':'20',review_horizon:'One month'}}]}
}
test('native observations keep estimates, self-reports and demonstrated records separate with exact paged provenance',async({page},info)=>{
 const f=await surveyFixture(page,true);f.state.observationPages=[[observation('estimate',1,null),observation('self_report',2,'3')],[observation('demonstrated_adoption',3,'0')]]
 await page.goto(managePath)
 await expect(page.getByRole('region',{name:'Estimates',exact:true})).toContainText('Unknown / not recorded')
 await expect(page.getByRole('region',{name:'Self-reported follow-through',exact:true})).toContainText('Native retained definition v2')
 await expect(page.getByRole('region',{name:'Demonstrated adoption records',exact:true})).toHaveCount(0)
 await page.getByRole('button',{name:'More native follow-through observations'}).click()
 const demonstrated=page.getByRole('region',{name:'Demonstrated adoption records',exact:true});await expect(demonstrated).toBeVisible();await expect(demonstrated.getByRole('cell',{name:'0',exact:true})).toBeVisible()
 await expect(demonstrated).toContainText('Unknown / not recorded');await expect(demonstrated).toContainText('Exact retained objective version 3');await expect(demonstrated).toContainText('01c8a898-5703-4231-8c6e-95e1e062fcbd')
 await expect(page.getByRole('region',{name:'Estimates',exact:true})).toContainText('people');await expect(demonstrated).toContainText('teams')
 await demonstrated.screenshot({path:info.outputPath('native-observations.png')})
 expect(f.observationCursors).toEqual([null,'synthetic-native-page-1'])
 expect(await page.evaluate(()=>JSON.stringify(localStorage)+JSON.stringify(sessionStorage))).not.toContain('Private organiser observation source')
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
})
test('pending follow-through has no fabricated zero and native resource loss clears restricted observations',async({page})=>{
 const f=await surveyFixture(page,true);await page.clock.install();await page.goto(managePath)
 await expect(page.getByText('No retained follow-through observations are available.',{exact:false})).toBeVisible()
 f.state.observationPages=[[observation('self_report',1,'3')]];await page.clock.fastForward(16_000)
 await expect(page.getByText('Private organiser observation source',{exact:true})).toBeVisible()
 f.state.observationDenied=true;await page.clock.fastForward(16_000)
 await expect(page.getByRole('alert')).toBeVisible();await expect(page.getByText('Private organiser observation source',{exact:true})).toHaveCount(0)
 await expect(page.getByRole('heading',{name:'Restricted participation and completion'})).toHaveCount(0)
})
test('archived organiser dashboard retains only native-authorised historical follow-through',async({page})=>{
 const f=await surveyFixture(page,true);f.state.writable=false;f.state.observationPages=[[observation('demonstrated_adoption',1,'2')]]
 await page.goto(managePath);await expect(page.getByText('This event is read-only.',{exact:false})).toBeVisible();await expect(page.getByRole('region',{name:'Demonstrated adoption records',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'Reviewed report',exact:true}).click();await expect(page.getByRole('button',{name:'Freeze cohort and save reviewed summary'})).toBeDisabled()
 expect(f.reportWrites).toHaveLength(0)
})
