import { expect, type Page } from '@playwright/test'
// Exercise the actual responsive native control. The owning capability registry
// supplies both the desktop buttons and narrow select options.
export async function openEventSection(page: Page, label: string) {
 const nav=page.getByRole('navigation',{name:'Event sections',exact:true}),select=nav.getByRole('combobox',{name:'Event section',exact:true})
 await nav.waitFor({state:'visible'})
 if(await select.isVisible())await select.selectOption({label})
 else await nav.getByRole('button',{name:label,exact:true}).click()
}

export async function expectEventSection(page: Page, label: string, permitted = true) {
 const nav=page.getByRole('navigation',{name:'Event sections',exact:true}),select=nav.getByRole('combobox',{name:'Event section',exact:true})
 await nav.waitFor({state:'visible'})
 if(await select.isVisible())await expect(select.locator('option').filter({hasText:new RegExp('^'+label.replace(/[.*+?^${}()|[\]\\]/g,'\\$&')+'$')})).toHaveCount(permitted?1:0)
 else if(permitted)await expect(nav.getByRole('button',{name:label,exact:true})).toBeVisible()
 else await expect(nav.getByRole('button',{name:label,exact:true})).toHaveCount(0)
}
