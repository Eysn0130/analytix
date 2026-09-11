async (page) => {
  const outputDir = __MERMAID_RENDER_OUT_DIR__
  const basename = __MERMAID_RENDER_BASENAME__
  if (!outputDir) throw new Error('MERMAID_RENDER_OUT_DIR is required')

  await page.setViewportSize({ width: 1800, height: 1600 })
  await page.waitForFunction(() => window.__MERMAID_RENDER_READY === true, null, { timeout: 45000 })
  await page.waitForTimeout(800)

  const cards = page.locator('.mermaid-card')
  const count = await cards.count()
  for (let index = 0; index < count; index += 1) {
    const card = cards.nth(index)
    await card.scrollIntoViewIfNeeded()
    await page.waitForTimeout(250)
    await card.screenshot({
      path: `${outputDir}/${basename}_${String(index + 1).padStart(2, '0')}.png`,
      animations: 'disabled'
    })
  }

  await page.screenshot({
    path: `${outputDir}/${basename}_总览.png`,
    fullPage: true,
    animations: 'disabled'
  })

  return { count }
}
