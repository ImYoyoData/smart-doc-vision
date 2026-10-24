import { chromium } from 'playwright-core'
import { mkdir } from 'node:fs/promises'
import path from 'node:path'

const base = process.env.BASE_URL || 'http://localhost:5173'
const token = process.env.SDV_TOKEN
const browserPath =
  process.env.BROWSER_PATH || 'C:/Program Files/Google/Chrome/Application/chrome.exe'

if (!token) {
  console.error('SDV_TOKEN is required')
  process.exit(1)
}

const outDir = path.resolve('public/screens')
await mkdir(outDir, { recursive: true })

const browser = await chromium.launch({ executablePath: browserPath, headless: true })
const context = await browser.newContext({
  viewport: { width: 1600, height: 900 },
  deviceScaleFactor: 2,
})
await context.addInitScript((value) => {
  localStorage.setItem('sdv.access_token', value)
}, token)

async function shot(page, name) {
  await page.waitForTimeout(1800)
  const file = path.join(outDir, `${name}.png`)
  await page.screenshot({ path: file })
  console.log('saved', file)
}

const pages = [
  ['dashboard', '/dashboard'],
  ['audits', '/audits'],
  ['providers', '/providers'],
]

for (const [name, route] of pages) {
  const page = await context.newPage()
  await page.goto(base + route, { waitUntil: 'networkidle' })
  await shot(page, name)
  await page.close()
}

// Script editor with the AI assistant panel open.
try {
  const page = await context.newPage()
  await page.goto(base + '/audits', { waitUntil: 'networkidle' })
  const card = page.locator('article').filter({ hasText: 'JS 回调联动测试' }).first()
  await card.getByRole('button', { name: '更多' }).click()
  await page.getByText('编辑配置', { exact: true }).click()
  await page.waitForTimeout(800)
  await page.getByRole('button', { name: '编辑脚本' }).click()
  await page.waitForSelector('.script-editor-modal', { timeout: 8000 })
  await page.waitForTimeout(800)
  const aiTab = page.getByRole('button', { name: 'AI 脚本助手' }).first()
  if (await aiTab.count()) {
    await aiTab.click()
    await page.waitForTimeout(900)
  }
  // Replace the test action's script with a credential-free demo for the video.
  const promoScript = `// 审核结论回调：查业务库 → HMAC 签名 → 通知第三方
const db = ctx.mysql.connect({
  host: ctx.secret('BIZ_DB_HOST'),
  user: ctx.secret('BIZ_DB_USER'),
  password: ctx.secret('BIZ_DB_PASSWORD'),
  database: ctx.secret('BIZ_DB_NAME')
})
const rows = db.query(
  'select order_no from orders where biz_key = ?',
  [ctx.task.biz_key]
)
db.close()

const nonce = ctx.crypto.uuid()
const ts = String(Date.now())
const sign = ctx.crypto.hmacSha256(
  ctx.secret('APP_SECRET'),
  'biz_key=' + ctx.task.biz_key + '&nonce=' + nonce + '&ts=' + ts
)

const res = ctx.http.request({
  url: 'https://api.example.com/audit/callback',
  method: 'POST',
  headers: { 'X-Sign': sign, 'X-Timestamp': ts, 'X-Nonce': nonce },
  body: { result: ctx.result, orders: rows },
  timeout_ms: 5000
})
ctx.log('callback', res.status)
;({ ok: res.ok, result: ctx.result })`
  const editor = page.locator('.script-editor-modal .cm-content').first()
  await editor.click()
  await page.keyboard.press('Control+A')
  await page.keyboard.insertText(promoScript)
  await page.waitForTimeout(500)
  await shot(page, 'script-editor')
  await page.close()
} catch (error) {
  console.warn('script editor capture skipped:', error.message)
}

await browser.close()
