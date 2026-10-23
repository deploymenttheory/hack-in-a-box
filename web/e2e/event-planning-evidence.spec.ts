import { test, expect } from '@playwright/test'
import { mockWorkspace, firstID } from './fixtures'
test('synthetic event planning desktop and mobile visual evidence',async({page},info)=>{
 await mockWorkspace(page);await page.goto('/events/'+firstID)
 await expect(page.getByRole('heading',{name:'North Star Hack',exact:true})).toBeVisible()
 await page.evaluate(()=>document.fonts.ready)
 await page.screenshot({path:info.outputPath((process.env.PLANNING_BASELINE?'before':'after')+'-overview.png'),fullPage:true})
 await page.getByRole('button',{name:'Setup',exact:true}).click()
 await expect(page.getByLabel('Event name',{exact:true})).toBeVisible()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:info.outputPath((process.env.PLANNING_BASELINE?'before':'after')+'-setup.png'),fullPage:true})
})
