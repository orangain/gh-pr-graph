const test = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const vm = require('node:vm')
const refreshTiming = require('../web/refresh.js')

const app = fs.readFileSync(require.resolve('../web/app.js'), 'utf8')
const loadSource = app.slice(app.indexOf('async function load()'), app.indexOf('function resumeAutoRefresh()'))

test('hidden tab fetches and schedules the next refresh without animation frames', async () => {
  let frames = 0
  let fetches = 0
  let scheduledDelay
  const context = vm.createContext({
    loading: false, initialLoadComplete: true, loadProgressValue: 0,
    lastUpdated: 0, refreshFailures: 0, retryAt: 0, timer: null,
    hiddenSince: Date.now(), sourceData: null,
    document: {hidden: true}, navigator: {onLine: true}, auto: {checked: true},
    progress: {hidden: true, classList: {toggle() {}}},
    refresh: {disabled: false}, updated: {}, errorBox: {},
    refreshTiming,
    requestAnimationFrame() { frames++ }, // Hidden tabs never deliver frames.
    clearTimeout() {}, setTimeout(callback, delay) { scheduledDelay = delay },
    graphURL: () => '/api/v1/graph',
    async fetch() { fetches++; return {} },
    async readGraphResponse() { return {nodes: [], edges: []} },
    includedByPullRequest: () => new Map(), retainIncluded() {},
    setLoadProgress() {}, filterBotPullRequests: value => value,
    render() { context.requestAnimationFrame(() => {}) },
    async inspectPullRequests() {}, async completeLoadProgress() {},
    async hydrateIncluded() {},
  })
  vm.runInContext(loadSource, context)
  await Promise.race([
    context.load(),
    new Promise((_, reject) => {
      const timeout = setTimeout(() => reject(new Error('refresh blocked by a missing animation frame')), 1000)
      timeout.unref()
    }),
  ])
  assert.equal(fetches, 1)
  assert.equal(frames, 2)
  assert.ok(context.lastUpdated > 0)
  assert.equal(context.loading, false)
  assert.equal(context.refresh.disabled, false)
  assert.equal(context.errorBox.hidden, true)
  assert.ok(scheduledDelay > 0 && scheduledDelay <= refreshTiming.refreshInterval)
})
