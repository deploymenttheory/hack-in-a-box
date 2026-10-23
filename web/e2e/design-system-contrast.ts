import type { Locator } from '@playwright/test'

export async function contrastOf(locator: Locator, foreground = 'color') {
  return locator.evaluate((element, property) => {
    const canvas = document.createElement('canvas'); canvas.width = canvas.height = 1
    const ctx = canvas.getContext('2d')!
    function rgb(value: string) { ctx.clearRect(0, 0, 1, 1); ctx.fillStyle = value; ctx.fillRect(0, 0, 1, 1); return [...ctx.getImageData(0, 0, 1, 1).data] }
    let background = [255, 255, 255, 255]
    const ancestors: Element[] = []; for (let node: Element | null = element; node; node = node.parentElement) ancestors.unshift(node)
    for (const node of ancestors) { const color = rgb(getComputedStyle(node).backgroundColor), alpha = color[3] / 255; background = background.map((value, i) => i < 3 ? color[i] * alpha + value * (1 - alpha) : 255) }
    function luminance(channels: number[]) { return channels.slice(0, 3).reduce((total, value, i) => { const c = value / 255; return total + [0.2126, 0.7152, 0.0722][i] * (c <= .04045 ? c / 12.92 : ((c + .055) / 1.055) ** 2.4) }, 0) }
    const a = luminance(rgb(getComputedStyle(element).getPropertyValue(property))), b = luminance(background)
    return (Math.max(a, b) + .05) / (Math.min(a, b) + .05)
  }, foreground)
}
