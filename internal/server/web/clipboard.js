(function (root, factory) {
  const clipboardText = factory()
  if (typeof module === 'object' && module.exports) module.exports = clipboardText
  else root.clipboardText = clipboardText
})(globalThis, function () {
  return function clipboardText(label, url) {
    return `${label.trim()}\n${url}`
  }
})
