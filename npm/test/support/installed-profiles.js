"use strict";

const assert = require('node:assert/strict');

// Candidate and public-registry installs enforce the same release expectation.
// Keeping it here makes an outdated smoke expectation fail before publication.
function assertFlashAccountProfile(template) {
  assert.equal(template.profiles.flash.provider, 'deepseek-account');
}

module.exports = { assertFlashAccountProfile };
