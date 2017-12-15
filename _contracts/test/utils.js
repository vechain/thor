module.exports = {
  assertFail: async function (promise) {
      let errorThorwn = false;
      try {
          await promise
      } catch (err) {
          errorThorwn = err.message.search("invalid opcode") >= 0
      }
      assert.ok(errorThorwn, "Transaction should fail")
  },

  assertEqual(actual, expected, msg) {
      if (isBigNumber(actual)) {
          assert.ok(actual.equals(expected), msg || (actual + ' equals ' + expected))
      } else if (isBigNumber(expected)) {
          assert.ok(expected.equals(actual), msg || (actual + ' equals ' + expected))
      } else
          assert.equal(actual, expected, msg)
  },
}

function isBigNumber(v) {
  return !!v.absoluteValue
}