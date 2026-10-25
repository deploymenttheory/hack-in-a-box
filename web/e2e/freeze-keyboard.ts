import {expect,type Page,type Locator} from '@playwright/test'
// Exercise actual native traversal. No HTMLElement.focus, locator.focus, forced
// click or DOM value mutation; the bounded loop fails an unreachable control.
export async function tabToFreezeControl(page:Page,target:Locator){
 await expect(target).toBeVisible();await expect(target).toBeEnabled()
 for(let step=0;step<120;step++){if(await target.evaluate(element=>element===document.activeElement))return;await page.keyboard.press('Tab')}
 throw new Error('Code evidence control was not reachable through keyboard traversal')
}
export async function activateFreezeControl(page:Page,target:Locator){await tabToFreezeControl(page,target);await page.keyboard.press('Enter')}
export async function typeFreezeField(page:Page,target:Locator,value:string){await tabToFreezeControl(page,target);await page.keyboard.press('ControlOrMeta+A');await page.keyboard.insertText(value)}
