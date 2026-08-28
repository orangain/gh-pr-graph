const test = require('node:test')
const assert = require('node:assert/strict')
const clipboardText = require('../web/clipboard.js')

test('formats the PR label and URL as one plain-text payload', () => {
  const label = '#123 improve graph'
  const url = 'https://github.com/orangain/gh-pr-graph/pull/123'

  assert.equal(clipboardText(label, url), label + '\n' + url)
})

test('trims only the label and preserves special characters', () => {
  const label = ' \t#7 [brackets] \\ <angles> & ampersand \n'
  const url = 'https://github.com/example/repo/pull/7'

  assert.equal(
    clipboardText(label, url),
    '#7 [brackets] \\ <angles> & ampersand\nhttps://github.com/example/repo/pull/7',
  )
})
