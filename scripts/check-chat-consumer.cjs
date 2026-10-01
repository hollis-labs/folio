// Optional browser check for the generated chat-app, served by its real Go binary.
const assert = require('node:assert/strict')
const fs = require('node:fs/promises')
const path = require('node:path')
const { chromium } = require(process.env.FOLIO_PLAYWRIGHT_MODULE || 'playwright')

async function main() {
  const url = process.argv[2]
  assert.ok(url, 'pass the generated app URL, including /chat/')
  const deadline = Date.now() + 15000
  while (true) {
    try {
      const response = await fetch(new URL('/api/health', url))
      if (response.ok) break
    } catch {}
    assert.ok(Date.now() < deadline, 'generated Go server did not start')
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
  const browser = await chromium.launch({
    headless: true,
    executablePath: process.env.FOLIO_CHROMIUM_EXECUTABLE || undefined,
  })
  try {
    const page = await browser.newPage()
    const errors = []
    page.on('pageerror', (error) => errors.push(error.message))
    const samples = []
    for (const width of [1024, 390]) {
      await page.setViewportSize({ width, height: 800 })
      await page.goto(url)
      const input = page.getByLabel('Message', { exact: true })
      await input.waitFor()
      const send = page.getByRole('button', { name: /^Send(?: message)?$/ })
      assert.equal(await send.count(), 1, 'ChatInput must supply exactly one Send control')
      await input.fill(`Hello at ${width}`)
      await send.click()
      await page.getByText(`Echo: Hello at ${width}`, { exact: true }).waitFor()
      assert.equal(await input.inputValue(), '', 'submit must clear the draft')
      const sample = await page.evaluate(() => {
        const row = document.querySelector('[data-slot="chat-stream-item"] > div')
        const bubble = row.lastElementChild
        const input = document.querySelector('textarea')
        const viewport = document.querySelector('[aria-label="Conversation"]')
        const probe = document.createElement('div')
        probe.style.fontSize = 'var(--text-control)'
        probe.style.borderRadius = 'var(--radius-panel)'
        document.body.append(probe)
        const expected = getComputedStyle(probe)
        const expectedFont = expected.fontSize
        const expectedRadius = expected.borderRadius
        probe.remove()
        const bubbleStyle = getComputedStyle(bubble)
        const inputStyle = getComputedStyle(input)
        const composer = document.querySelector('[data-slot="chat-input"]')
        const composerStyle = getComputedStyle(composer)
        const streamStyle = getComputedStyle(viewport)
        const selectors = []
        function walk(rules) {
          for (const rule of rules) {
            if (rule.selectorText) selectors.push(rule.selectorText)
            if (rule.cssRules) walk(rule.cssRules)
          }
        }
        for (const sheet of document.styleSheets) walk(sheet.cssRules)
        return {
          viewport: window.innerWidth,
          rowAlignment: getComputedStyle(row).alignItems,
          bubble: { fontSize: bubbleStyle.fontSize, radius: bubbleStyle.borderRadius,
            background: bubbleStyle.backgroundColor, maxWidth: bubbleStyle.maxWidth },
          composer: { fontSize: inputStyle.fontSize, borderWidth: composerStyle.borderTopWidth,
            radius: composerStyle.borderRadius, width: composer.getBoundingClientRect().width },
          stream: { overflow: streamStyle.overflowY, paddingLeft: streamStyle.paddingLeft,
            paddingTop: streamStyle.paddingTop },
          tokens: { controlFont: expectedFont, panelRadius: expectedRadius },
          typography: { bubbleMatchesControlToken: bubbleStyle.fontSize === expectedFont,
            composerMatchesControlToken: inputStyle.fontSize === expectedFont },
          emitted: { controlFont: selectors.includes('.text-control'),
            userAlignment: selectors.includes('.items-end'), maxWidth: selectors.includes('.max-w-lg') },
          horizontalOverflow: document.documentElement.scrollWidth > window.innerWidth,
        }
      })
      sample.sendControls = await send.count()
      assert.ok(Object.values(sample.typography).every(Boolean), 'bubble and composer must match the control font token')
      assert.equal(sample.rowAlignment, 'flex-end', 'kit source must style the user row')
      assert.equal(sample.bubble.radius, sample.tokens.panelRadius)
      assert.notEqual(sample.bubble.radius, '0px')
      assert.notEqual(sample.bubble.background, 'rgba(0, 0, 0, 0)')
      assert.notEqual(sample.bubble.maxWidth, 'none')
      assert.ok(parseFloat(sample.composer.borderWidth) > 0, 'composer border must be styled')
      assert.ok(parseFloat(sample.composer.radius) > 0, 'composer radius must be styled')
      assert.ok(sample.composer.width > 200, 'composer must remain usable at narrow widths')
      assert.equal(sample.stream.overflow, 'auto')
      assert.ok(parseFloat(sample.stream.paddingTop) > 0)
      assert.ok(Object.values(sample.emitted).every(Boolean), 'chat utilities must be emitted')
      assert.equal(sample.horizontalOverflow, false)
      samples.push(sample)
      if (process.env.FOLIO_SCREENSHOT_DIR) {
        await fs.mkdir(process.env.FOLIO_SCREENSHOT_DIR, { recursive: true })
        await page.screenshot({ path: path.join(process.env.FOLIO_SCREENSHOT_DIR, `chat-app-${width}.png`) })
      }
      await page.getByRole('button', { name: 'New conversation', exact: true }).click()
      await page.getByText('Start a conversation', { exact: true }).waitFor()
    }
    assert.deepEqual(errors, [], 'browser must have no uncaught page errors')
    console.log(JSON.stringify({ result: 'PASS', scope: 'source emission, styled layout, control-token typography, one Send control, and interaction', pageErrors: errors, samples }, null, 2))
  } finally {
    await browser.close()
  }
}
main().catch((error) => { console.error(error); process.exitCode = 1 })
