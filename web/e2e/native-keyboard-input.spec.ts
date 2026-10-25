import { test, expect } from '@playwright/test'
import { activate, typeInto } from '../qualification-native/keyboard'

test('native text-entry helper observes a read-pause rejection before deliberately re-entering the same field', async ({ page }) => {
 await page.setContent('<fieldset><textarea aria-label="Synthetic current evidence">Earlier value</textarea></fieldset>')
 const control = page.getByLabel('Synthetic current evidence'), press = page.keyboard.press.bind(page.keyboard), insert = page.keyboard.insertText.bind(page.keyboard)
 let paused = false, inserts = 0
 page.keyboard.press = async (key, options) => { await press(key, options); if (key === 'ControlOrMeta+A' && !paused) { paused = true; await page.locator('fieldset').evaluate(fieldset => { (fieldset as HTMLFieldSetElement).disabled = true }) } }
 page.keyboard.insertText = async text => { inserts++; await insert(text); await page.locator('fieldset').evaluate(fieldset => { (fieldset as HTMLFieldSetElement).disabled = false }) }
 await typeInto(control, 'Deliberately entered full evidence')
 await expect(control).toHaveValue('Deliberately entered full evidence')
 expect(inserts).toBe(2)
})
test('native checkbox helper observes a read-pause rejection before deliberately toggling the same checkbox', async ({ page }) => {
 await page.setContent('<fieldset><label><input type="checkbox" checked>Pending observation</label></fieldset>')
 const control = page.getByRole('checkbox', { name: 'Pending observation' }), press = page.keyboard.press.bind(page.keyboard)
 let spaces = 0
 page.keyboard.press = async (key, options) => {
  if (key === 'Space') { spaces++; if (spaces === 1) await page.locator('fieldset').evaluate(fieldset => { (fieldset as HTMLFieldSetElement).disabled = true }) }
  await press(key, options)
  if (key === 'Space') await page.locator('fieldset').evaluate(fieldset => { (fieldset as HTMLFieldSetElement).disabled = false })
 }
 await activate(control, 'Space')
 await expect(control).not.toBeChecked()
 expect(spaces).toBe(2)
})

test('an unobserved failed input is not permission to repeat text entry', async ({ page }) => {
 await page.setContent('<textarea aria-label="Synthetic current evidence">Earlier value</textarea>')
 let inserts = 0
 page.keyboard.insertText = async () => { inserts++ }
 await expect(typeInto(page.getByLabel('Synthetic current evidence'), 'New observation')).rejects.toThrow('Native input did not accept its trusted keys')
 expect(inserts).toBe(1)
})
test('a paused replacement field cannot receive the old text entry', async ({ page }) => {
 await page.setContent('<fieldset><textarea aria-label="Synthetic current evidence">Earlier value</textarea></fieldset>')
 const control = page.getByLabel('Synthetic current evidence'), press = page.keyboard.press.bind(page.keyboard), insert = page.keyboard.insertText.bind(page.keyboard)
 let inserts = 0
 page.keyboard.press = async (key, options) => { await press(key, options); if (key === 'ControlOrMeta+A') await control.evaluate(element => { element.closest('fieldset')!.disabled = true; element.replaceWith(element.cloneNode(true)) }) }
 page.keyboard.insertText = async text => { inserts++; await insert(text); await page.locator('fieldset').evaluate(fieldset => { (fieldset as HTMLFieldSetElement).disabled = false }) }
 await expect(typeInto(control, 'Old private intended observation')).rejects.toThrow('Current native input was replaced')
 await expect(control).toHaveValue('Earlier value')
 expect(inserts).toBe(1)
})
test('command-button activation never uses local input re-entry', async ({ page }) => {
 await page.setContent('<fieldset><button>Confirm a command</button></fieldset>')
 const press = page.keyboard.press.bind(page.keyboard)
 let enters = 0
 page.keyboard.press = async (key, options) => { if (key === 'Enter') { enters++; await page.locator('fieldset').evaluate(fieldset => { (fieldset as HTMLFieldSetElement).disabled = true }) } await press(key, options); if (key === 'Enter') await page.locator('fieldset').evaluate(fieldset => { (fieldset as HTMLFieldSetElement).disabled = false }) }
 await activate(page.getByRole('button', { name: 'Confirm a command' }))
 expect(enters).toBe(1)
})
