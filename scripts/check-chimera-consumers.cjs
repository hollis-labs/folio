// Explicit network/build/native opt-in proof. Folio itself remains a renderer.
const assert = require('node:assert/strict')
const { spawn } = require('node:child_process')
const fs = require('node:fs/promises')
const path = require('node:path')
const { createHash } = require('node:crypto')
const { chromium, expect } = require(process.env.FOLIO_PLAYWRIGHT_MODULE || '@playwright/test')
const root = path.resolve(__dirname, '..')
const scratch = path.join(root, '.scratch')
const tag = `chimera-consumers-${Date.now()}`
const output = path.join(scratch, tag)
const env = { ...process.env, TMPDIR: path.join(scratch, 'tmp'), GOCACHE: path.join(scratch, 'gocache'), GOMODCACHE: path.join(scratch, 'gomodcache'), GOWORK: 'off', npm_config_cache: path.join(scratch, 'npmcache') }
const sha = bytes => createHash('sha256').update(bytes).digest('hex')
async function run(command, args, cwd, logname) {
  const child = spawn(command, args, { cwd, env })
  let text = ''
  child.stdout.on('data', b => { text += b })
  child.stderr.on('data', b => { text += b })
  const code = await new Promise((resolve, reject) => { child.on('error', reject); child.on('exit', resolve) })
  if (logname) await fs.writeFile(logname, text)
  assert.equal(code, 0, `${command} ${args.join(' ')}\n${text}`)
  return text
}
async function main() {
  await fs.mkdir(output, { recursive: true })
  await fs.mkdir(env.TMPDIR, { recursive: true })
  const folio = path.join(output, 'folio')
  await run('go', ['build', '-o', folio, './cmd/folio'], root, path.join(output, 'folio-build.log'))
  const browser = await chromium.launch({ headless: true, executablePath: process.env.FOLIO_CHROMIUM_EXECUTABLE || undefined })
  const reports = [], captures = []
  try {
    for (const preset of (process.env.FOLIO_PROOF_VARIANT === 'plugin' ? ['app-dashboard-chimera-plugin', 'chat-app-chimera-plugin'] : ['app-dashboard', 'chat-app'])) for (const base of ['/', '/review']) {
      const name = `${preset.replaceAll('-', '_')}_${base === '/' ? 'root' : 'review'}`
      const target = path.join(output, name)
      await run(folio, ['new', preset, target, '--non-interactive', '--input', `project_name=${name}`, ...(preset.endsWith('-plugin') ? [] : ['--input', 'gui_host=chimera']), '--input', `base_path=${base}`], root)
      const initial = await run(folio, ['inspect', target], root, path.join(target, 'inspect-initial.log'))
      assert(!/locally_modified|preset_updated|conflict|missing_locally/.test(initial), initial)
      const frontend = path.join(target, 'frontend'), lockpath = path.join(frontend, 'package-lock.json')
      const lockHash = sha(await fs.readFile(lockpath))
      await run('npm', ['ci', '--ignore-scripts', '--no-audit', '--no-fund'], frontend, path.join(target, 'npm-ci.log'))
      for (const check of ['typecheck', 'lint', 'build']) await run('npm', ['run', check], frontend, path.join(target, `frontend-${check}.log`))
      assert.equal(sha(await fs.readFile(lockpath)), lockHash, 'npm ci/build altered frozen lock')
      await run('go', ['mod', 'tidy'], target, path.join(target, 'go-tidy.log'))
      const module = JSON.parse(await run('go', ['list', '-m', '-json', 'github.com/hollis-labs/chimera'], target))
      assert.equal(module.Version, 'v0.0.0-20261008115211-389155313ee5')
      assert.equal(module.Sum, 'h1:2JzG7PC8650ivo7aKSITaquRaBHJsWmffxKzxffzXN8=')
      assert(!(await fs.readFile(path.join(target, 'go.mod'), 'utf8')).includes('replace'))
      await run('go', ['test', './...'], target, path.join(target, 'go-test.log'))
      const binary = path.join(target, 'consumer')
      await run('go', ['build', '-o', binary, `./cmd/${name}`], target, path.join(target, 'go-build.log'))
      const server = spawn(binary, [], { cwd: target, env: { ...env, LISTEN_ADDR: '127.0.0.1:18543' } })
      let serverLog = ''; server.stderr.on('data', b => { serverLog += b })
      const stopped = new Promise(resolve => server.on('exit', resolve))
      const origin = 'http://127.0.0.1:18543', url = origin + (base === '/' ? '/' : base + '/')
      try {
        await expect.poll(async () => { try { return (await fetch(origin + '/healthz')).status } catch { return 0 } }, { timeout: 15000 }).toBe(200)
        assert.deepEqual(await (await fetch(origin + '/api/health')).json(), { status: 'ok' })
        assert.equal((await fetch(origin + '/api/missing')).status, 404)
        assert.equal((await fetch(origin + '/plugins/registry')).status, preset.endsWith('-plugin') ? 200 : 404)
        assert.equal((await fetch(url + 'declared-local-deep-route')).status, 200)
        if (preset.startsWith('chat-app')) {
          assert.equal((await fetch(origin + '/api/messages', { method: 'POST', body: JSON.stringify({ message: ' ' }) })).status, 400)
          assert.equal((await fetch(origin + '/api/messages', { method: 'POST', body: 'x'.repeat(65537) })).status, 400)
        }
        for (const [width, height, label] of [[1280, 720, 'desktop'], [390, 844, 'narrow'], [390, 500, 'short']]) {
          const context = await browser.newContext({ viewport: { width, height } }), page = await context.newPage(), errors = []
          page.on('pageerror', error => errors.push(String(error)))
          page.on('request', request => assert.equal(new URL(request.url()).origin, origin, 'Unexpected external consumer request'))
          await page.goto(url)
          if (preset.startsWith('chat-app')) {
            const input = page.getByLabel('Message', { exact: true })
            await expect(input).toBeVisible()
            await input.focus()
            await input.pressSequentially('native local echo')
            await page.keyboard.press('Enter')
            await expect(page.getByText('Echo: native local echo', { exact: true })).toBeVisible()
            const reset = page.getByRole('button', { name: 'New conversation', exact: true })
            for (let count = 0; count < 12 && !(await reset.evaluate(n => n === document.activeElement)); count++) await page.keyboard.press('Tab')
            await expect(reset).toBeFocused()
            await page.keyboard.press('Enter')
            await expect(page.getByText('Start a conversation', { exact: true })).toBeVisible()
            await expect(page.getByText('Echo: native local echo', { exact: true })).toHaveCount(0)
            await input.focus()
            await input.fill('Bounded local literal '.repeat(160) + 'END OF ECHO')
            await page.keyboard.press('Enter')
            await expect(page.getByText(/^Echo: Bounded local literal/)).toBeVisible()
            const conversation = page.getByRole('region', { name: 'Conversation', exact: true })
            for (let count = 0; count < 15 && !(await conversation.evaluate(n => n === document.activeElement)); count++) await page.keyboard.press('Shift+Tab')
            await expect(conversation).toBeFocused()
            await page.keyboard.press('Control+Home')
            await expect.poll(() => page.getByText(/^Bounded local literal/).evaluate(n => {
              const walker = document.createTreeWalker(n, NodeFilter.SHOW_TEXT); const text = walker.nextNode();
              const range = document.createRange(); range.setStart(text, 0); range.setEnd(text, 12);
              const rect = range.getBoundingClientRect(); const viewport = n.closest('[role="region"]').getBoundingClientRect();
              return rect.top >= viewport.top && rect.bottom <= viewport.bottom;
            })).toBe(true)
            const startPosition = await conversation.evaluate(n => n.scrollTop)
            await page.keyboard.press('PageDown')
            await expect.poll(() => conversation.evaluate(n => n.scrollTop)).toBeGreaterThan(startPosition)
            await page.keyboard.press('Control+End')
            await expect.poll(() => conversation.evaluate(n => n.scrollTop + n.clientHeight >= n.scrollHeight - 2)).toBe(true)
            const endpoint = await page.getByText(/^Echo: Bounded local literal/).evaluate(n => {
              const walker = document.createTreeWalker(n, NodeFilter.SHOW_TEXT); let node; let found;
              while ((node = walker.nextNode())) if (node.textContent.includes('END OF ECHO')) found = node;
              const range = document.createRange(); const offset = found.textContent.lastIndexOf('END OF ECHO'); range.setStart(found, offset); range.setEnd(found, offset + 11);
              const rect = range.getBoundingClientRect(); return { x: rect.x, y: rect.y, right: rect.right, bottom: rect.bottom };
            })
            assert(endpoint.x >= 0 && endpoint.right <= width && endpoint.y >= 0 && endpoint.bottom <= height, 'Native echo endpoint is clipped')
            const echoShot = path.join(output, `${name}-${label}-echo-end.png`)
            await page.screenshot({ path: echoShot, fullPage: false })
            captures.push({path: echoShot, sha256: sha(await fs.readFile(echoShot))})
            const composer = await input.boundingBox()
            assert(composer && composer.y >= 0 && composer.y + composer.height <= height + 1, 'Pinned composer is clipped')
          } else {
            await expect(page.getByText('ok', { exact: true })).toBeVisible()
            const nav = page.getByRole('button', { name: 'Dashboard', exact: true })
            for (let count = 0; count < 14 && !(await nav.evaluate(n => n === document.activeElement)); count++) await page.keyboard.press('Tab')
            await expect(nav).toBeFocused()
            await page.keyboard.press('Enter')
            await expect(page.getByText('ready', { exact: true })).toBeVisible()
          }
          if (preset.endsWith('-plugin')) {
            const widget = page.getByRole('button', {name:'Reviewed widget local count 0',exact:true})
            await expect(widget).toBeVisible()
            assert.equal(await widget.evaluate(n=>getComputedStyle(n).paddingTop), '28px')
            await widget.click(); await expect(page.getByRole('button',{name:'Reviewed widget local count 1',exact:true})).toBeVisible()
            const leases = page.locator('link[href$="/plugins/recipe/g1/style.css"]')
            await expect(leases).toHaveCount(1)
            for (const license of ['LICENSE-Chimera.txt','LICENSE-plugin-host-ui.txt']) assert((await (await fetch(url+license)).text()).includes('MIT License'))
            const pluginShot=path.join(output, `${name}-${label}-widget.png`); await page.screenshot({path:pluginShot,fullPage:false}); captures.push({path:pluginShot,sha256:sha(await fs.readFile(pluginShot))})
            await page.getByRole('button',{name:'Unload reviewed widget',exact:true}).click()
            await expect(leases).toHaveCount(0)
            await expect(page.getByRole('button',{name:'Reviewed widget local count 1',exact:true})).toHaveCount(0)
          }
          await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth && document.documentElement.scrollHeight <= innerHeight + 1)).toBe(true)
          const assets = await page.locator('script[src],link[rel=stylesheet]').evaluateAll(nodes => nodes.map(n => n.src || n.href))
          for (const asset of assets) { assert(asset.startsWith(url), asset); assert.equal((await fetch(asset)).status, 200) }
          const background = await page.locator('#root > *').evaluate(n => getComputedStyle(n).backgroundColor)
          assert(!['transparent', 'rgba(0, 0, 0, 0)'].includes(background), background)
          await page.goto(url + 'declared-local-deep-route')
          await page.reload()
          await expect(preset.startsWith('chat-app') ? page.getByLabel('Message', { exact: true }) : page.getByText('ok', { exact: true })).toBeVisible()
          if (preset.endsWith('-plugin')) {
            const freshWidget = page.getByRole('button', {name:'Reviewed widget local count 0',exact:true})
            const freshUnload = page.getByRole('button',{name:'Unload reviewed widget',exact:true})
            await expect(freshWidget).toBeVisible(); await expect(freshUnload).toBeVisible()
            await expect(page.locator('link[href$="/plugins/recipe/g1/style.css"]')).toHaveCount(1)
            await expect.poll(() => freshWidget.evaluate(n => {
              const style=getComputedStyle(n); return style.fontWeight === style.getPropertyValue('--font-weight-semibold').trim() && style.paddingTop === '28px';
            })).toBe(true)
            for (const control of [freshWidget,freshUnload]) {
              const box=await control.boundingBox(); assert(box && box.x>=0 && box.y>=0 && box.x+box.width<=width+1 && box.y+box.height<=height+1, 'Reloaded plugin control clipped')
            }
            await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))))
          }
          const shot = path.join(output, `${name}-${label}.png`)
          await page.screenshot({ path: shot, fullPage: false })
          captures.push({ path: shot, sha256: sha(await fs.readFile(shot)) })
          assert.deepEqual(errors, [])
          reports.push({ preset, base, width, height, lockSha256: lockHash, moduleVersion: module.Version, moduleSum: module.Sum })
          await context.close()
        }
      } finally {
        server.kill('SIGTERM')
        assert.equal(await stopped, 0, serverLog)
        await fs.writeFile(path.join(target, 'server.log'), serverLog)
      }
    }
  } finally { await browser.close() }
  await fs.writeFile(path.join(output, 'proof.json'), JSON.stringify({ cases: reports, captures }, null, 2) + '\n')
  console.log(`PASS ${reports.length} fresh ${process.env.FOLIO_PROOF_VARIANT === 'plugin' ? 'plugin-wrapper-default' : 'shell'} native contexts; evidence ${output}`)
}
main().catch(error => { console.error(error); process.exitCode = 1 })
