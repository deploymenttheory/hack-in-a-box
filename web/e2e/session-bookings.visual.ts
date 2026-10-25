import { test, expect } from '@playwright/test'
import { sessionsFixture } from './session-bookings-fixtures'
import { firstID } from './fixtures'
import { expectEventSection } from './event-navigation-fixture'
test('reviewed main has no Sessions UI in the adjacent native event workspace',async({page},info)=>{
 await sessionsFixture(page);await page.goto(`/events/${firstID}`);await expectEventSection(page,'Sessions',false);await expect(page.getByRole('heading',{name:'Sessions and personal agenda'})).toHaveCount(0);await expect(page.getByRole('heading',{name:'Synthetic session edition',exact:true})).toBeVisible();await page.screenshot({path:info.outputPath('before-sessions-ui-absent.png'),fullPage:true})
})
