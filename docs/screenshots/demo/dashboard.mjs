// Screenshots the dashboard after scripts/demo-agent.sh: the rolled-back
// canary on Deployments and 2.0.0's error rate on Telemetry.
// Usage: node dashboard.mjs <base url> <api key> <2.0.0 version id> <out dir>
import { chromium } from 'playwright-core'

const [base, key, v2, out] = process.argv.slice(2)
const browser = await chromium.launch({ channel: 'chrome' })
const page = await browser.newPage({ viewport: { width: 1280, height: 760 }, deviceScaleFactor: 1, colorScheme: 'light' })

await page.goto(`${base}/login`)
await page.fill('input[name=api_key]', key)
await page.click('button[type=submit]')
await page.waitForURL(`${base}/`)

await page.goto(`${base}/deployments`)
await page.locator('text=ROLLED_BACK >> visible=true').first().waitFor()
await page.screenshot({ path: `${out}/1-deployments.png` })

await page.goto(`${base}/telemetry?version=${v2}`)
await page.getByText('Error Rate').waitFor()
// The header and stat cards only: the demo's 40 seconds of events fill one
// hourly point of the chart below them.
const main = await page.locator('main').boundingBox()
const card = await page.getByText('Error Rate').locator('..').boundingBox()
await page.screenshot({ path: `${out}/2-telemetry.png`, clip: { x: main.x, y: 0, width: main.width, height: card.y + card.height + 16 } })

await browser.close()
