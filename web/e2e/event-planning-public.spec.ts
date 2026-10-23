import { openEventSection, expectEventSection } from './event-navigation-fixture'
import { expect, test } from '@playwright/test'
import { mockWorkspace, firstID, secondID } from './fixtures'
import { contrastOf } from './design-system-contrast'

test('participant event chrome keeps only native reader workspaces and scoped private snapshot links',async({page},info)=>{
 await mockWorkspace(page);const writes:string[]=[]
 page.on('request',request=>{if(new URL(request.url()).pathname.startsWith('/api/')&&!['GET','HEAD'].includes(request.method()))writes.push(request.url())})
 await page.goto('/events/'+firstID)
 const navigation=page.getByRole('navigation',{name:'Event sections',exact:true})
 for(const label of ['Setup','Staff','Rules','Applicants','Attendance','Inventory'])await expectEventSection(page,label,false)
 await expect(page.getByRole('heading',{name:'Event dates',exact:true})).toBeVisible()
 const snapshot=page.getByRole('region',{name:'participant downloads',exact:true})
 await expect(snapshot.getByRole('link',{name:'Readiness CSV',exact:true})).toHaveAttribute('href',`/api/v1/events/${firstID}/exports/readiness.csv?audience=participant`)
 if(info.project.name.endsWith('mobile')){await navigation.getByRole('combobox',{name:'Event section',exact:true}).focus();await page.keyboard.press('t');await page.keyboard.press('Enter');await expect(navigation.getByRole('combobox',{name:'Event section',exact:true})).toHaveValue('today')}else{await navigation.getByRole('button',{name:'Today',exact:true}).focus();await page.keyboard.press('Enter')};await expect(page.getByRole('heading',{name:'Today',exact:true})).toBeVisible()
 await page.getByRole('link',{name:'← Switch event',exact:true}).click();await page.getByRole('link',{name:'Southern Lights Hack',exact:true}).click()
 await expect(page.getByRole('heading',{name:'Southern Lights Hack',exact:true})).toBeVisible();await expect(page.getByRole('heading',{name:'North Star Hack',exact:true})).toHaveCount(0)
 await expect(page.getByRole('link',{name:'Readiness CSV',exact:true})).toHaveAttribute('href',`/api/v1/events/${secondID}/exports/readiness.csv?audience=participant`)
 expect(writes).toEqual([]);expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:info.outputPath('participant-event-after.png'),fullPage:true})
})

test('participant custom event keeps native contrast and focus in a system-dark portal',async({page})=>{
 await mockWorkspace(page);await page.emulateMedia({colorScheme:'dark',reducedMotion:'reduce'})
 await page.route(`**/events/${firstID}/branding`,route=>route.fulfill({json:{revision:1,branding:{primary:'#747474',background:'#ffffff',foreground:'#747474',accent:'#efca63',assets:[]}}}))
 await page.goto('/events/'+firstID)
 const heading=page.getByRole('heading',{name:'North Star Hack',exact:true});await expect(heading).toBeVisible();expect(await contrastOf(heading)).toBeGreaterThanOrEqual(4.5)
 const nav=page.getByRole('navigation',{name:'Event sections',exact:true}),today=await nav.getByRole('combobox',{name:'Event section',exact:true}).isVisible()?nav.getByRole('combobox',{name:'Event section',exact:true}):nav.getByRole('button',{name:'Today',exact:true})
 await today.focus();expect(await today.evaluate(element=>getComputedStyle(element).outlineStyle)).not.toBe('none');expect(await contrastOf(today)).toBeGreaterThanOrEqual(4.5)
 await openEventSection(page,'Today');await expect(page.getByRole('heading',{name:'Today',exact:true})).toBeVisible()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
})
