const test = require('node:test')
const assert = require('node:assert/strict')
const {refreshInterval, backgroundRefreshDuration, retryDelay, nextDelay, canRefreshInBackground} = require('../web/refresh.js')

test('successful refreshes remain five minutes apart', () => {
  assert.equal(nextDelay(1000, 1000, 0, 0), refreshInterval)
  assert.equal(nextDelay(61000, 1000, 0, 0), 240000)
})

test('failed refreshes use capped exponential backoff', () => {
  assert.deepEqual(
    [1, 2, 3, 4, 5, 6].map(retryDelay),
    [30000, 60000, 120000, 240000, 300000, 300000],
  )
})

test('failure delay does not depend on the last successful refresh', () => {
  const now = 600000
  const retryAt = now + retryDelay(1)
  assert.equal(nextDelay(now, 0, 1, retryAt), 30000)
  assert.equal(nextDelay(now + 29000, 0, 1, retryAt), 1000)
})

test('background refresh remains enabled for thirty minutes', () => {
  const hiddenSince = 1000
  assert.equal(canRefreshInBackground(hiddenSince + backgroundRefreshDuration - 1, hiddenSince), true)
  assert.equal(canRefreshInBackground(hiddenSince + backgroundRefreshDuration, hiddenSince), false)
  assert.equal(canRefreshInBackground(hiddenSince + backgroundRefreshDuration + 1, hiddenSince), false)
})

test('background refresh requires a recorded hidden time', () => {
  assert.equal(canRefreshInBackground(1000, 0), false)
})
