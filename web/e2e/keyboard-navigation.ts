import { expect, type Locator, type Page } from '@playwright/test'
// Traverse the real focus order. Never focus/select/click the target directly;
// an unreachable or trapped control fails this finite keyboard journey.
export async function tabTo(page:Page,target:Locator,backward=false,maxTabs=180){
 await expect(target).toBeVisible()
 for(let index=0;index<maxTabs;index++){
  if(await target.evaluate(element=>element===element.ownerDocument.activeElement))return
  await page.keyboard.press(backward?'Shift+Tab':'Tab')
 }
 throw new Error(`Keyboard focus could not reach ${target.toString()} after ${maxTabs} native tabs`)
}
export async function keyboardEventSection(page:Page,label:string,value:string){
 const nav=page.getByRole('navigation',{name:'Event sections',exact:true}),select=nav.getByRole('combobox',{name:'Event section',exact:true})
 await expect(nav).toBeVisible()
 if(await select.isVisible()){
  await tabTo(page,select);await page.keyboard.press('Home')
  const index=await select.evaluate((element,requested)=>[...(element as HTMLSelectElement).options].findIndex(option=>option.value===requested),value)
  expect(index).toBeGreaterThanOrEqual(0)
  for(let step=0;step<index;step++)await page.keyboard.press('ArrowDown')
  await page.keyboard.press('Enter');await expect(select).toHaveValue(value)
 }else{await tabTo(page,nav.getByRole('button',{name:label,exact:true}));await page.keyboard.press('Enter')}
}
export async function keyboardActivate(page:Page,target:Locator,key:'Enter'|'Space'='Enter'){
 await tabTo(page,target);await page.keyboard.press(key)
}
export async function keyboardConfirm(page:Page){
 const dialog=page.getByRole('alertdialog');await expect(dialog).toBeVisible();await expect(dialog.getByRole('button',{name:'Cancel',exact:true})).toBeFocused();await page.keyboard.press('Tab');await expect(dialog.getByRole('button',{name:'Confirm reviewed change'})).toBeFocused();await page.keyboard.press('Enter')
}
export async function keyboardCorrectReview(page:Page,prepare:Locator){
 await keyboardActivate(page,prepare,'Space');const dialog=page.getByRole('alertdialog');await expect(dialog.getByRole('button',{name:'Cancel',exact:true})).toBeFocused();await page.keyboard.press('Shift+Tab');await expect(dialog.getByRole('button',{name:'Close confirmation'})).toBeFocused();await page.keyboard.press('Tab');await expect(dialog.getByRole('button',{name:'Cancel',exact:true})).toBeFocused();await page.keyboard.press('Space');await expect(prepare).toBeFocused()
}
