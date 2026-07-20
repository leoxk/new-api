import assert from 'node:assert/strict'
import test from 'node:test'

import policy from '../config/glimo-b2b-commercial-policy.json' with { type: 'json' }
import { auditSnapshot } from './glimo-b2b-policy-audit.mjs'

function passingSnapshot() {
  const options = {
    AutoGroups: ['default', 'b2b', 'b2b-deepseek'],
    DefaultUseAutoGroup: true,
    GroupRatio: Object.fromEntries(
      Object.entries(policy.groups).map(([group, value]) => [group, value.usageRatio]),
    ),
    'group_ratio_setting.group_ratio': Object.fromEntries(
      Object.entries(policy.groups).map(([group, value]) => [group, value.usageRatio]),
    ),
    TopupGroupRatio: Object.fromEntries(
      Object.entries(policy.groups).map(([group, value]) => [group, value.topupRatio]),
    ),
    UserUsableGroups: { default: 'Default' },
    'group_ratio_setting.group_special_usable_group': {
      b2b: { '-:default': '', '+:auto': 'Auto', '+:b2b-deepseek': 'DeepSeek' },
    },
    ModelRatio: {},
    CompletionRatio: {},
    CacheRatio: {},
    CreateCacheRatio: {},
    ImageRatio: {},
  }
  const abilities = []
  let channelId = 1
  for (const [modelName, model] of Object.entries(policy.models)) {
    abilities.push({
      group: model.group,
      model: modelName,
      channelId: channelId++,
      abilityEnabled: true,
      channelStatus: 1,
    })
    for (const [dimension, value] of Object.entries(model.newApi)) {
      const optionKey = {
        modelRatio: 'ModelRatio',
        completionRatio: 'CompletionRatio',
        cacheRatio: 'CacheRatio',
        createCacheRatio: 'CreateCacheRatio',
        imageRatio: 'ImageRatio',
      }[dimension]
      options[optionKey][modelName] = value
    }
  }
  return {
    capturedAt: '2026-07-19T00:00:00.000Z',
    options,
    abilities,
    b2bUserCount: 0,
    b2bTokenCount: 0,
    b2bTokenViolationCount: 0,
  }
}

test('approved catalog and pricing pass', () => {
  const result = auditSnapshot(passingSnapshot(), policy)
  assert.equal(result.ok, true)
  assert.deepEqual(result.errors, [])
  assert.equal(result.summary.approvedModels, 10)
  assert.equal(result.summary.configuredModels, 10)
  assert.deepEqual(result.summary.deferredModels, [])
  assert.equal(result.customerRates['gpt-5.6-sol'].usdPerMillion.input, 1.5)
  assert.ok(
    Math.abs(result.customerRates['deepseek-v4-pro'].usdPerMillion.output - 0.957) <
      1e-12,
  )
  assert.equal(result.customerRates['gpt-image-2'].usdPerMillion.imageOutput, 9)
})

test('stale ten-percent b2b ratio fails loudly', () => {
  const snapshot = passingSnapshot()
  snapshot.options.GroupRatio.b2b = 0.1
  snapshot.options['group_ratio_setting.group_ratio'].b2b = 0.1
  snapshot.b2bUserCount = 2
  snapshot.b2bTokenCount = 57

  const result = auditSnapshot(snapshot, policy)
  assert.equal(result.ok, false)
  assert.ok(result.errors.includes('GroupRatio.b2b: expected 0.3, found 0.1'))
  assert.ok(
    result.errors.includes(
      'group_ratio_setting.group_ratio.b2b: expected 0.3, found 0.1',
    ),
  )
  assert.ok(result.warnings[0].includes('2 production user(s)'))
})

test('unapproved model, bad channel, and token fallback all fail', () => {
  const snapshot = passingSnapshot()
  snapshot.abilities.push({
    group: 'b2b',
    model: 'dall-e-3',
    channelId: 99,
    abilityEnabled: true,
    channelStatus: 2,
  })
  snapshot.b2bTokenViolationCount = 1
  snapshot.options['group_ratio_setting.group_special_usable_group'].b2b[
    '+:default'
  ] = 'Default'

  const result = auditSnapshot(snapshot, policy)
  assert.equal(result.ok, false)
  assert.ok(result.errors.some((error) => error.includes('unapproved models: dall-e-3')))
  assert.ok(result.errors.includes('dall-e-3 is exposed through b2b'))
  assert.ok(result.errors.includes('b2b must not have a +:default fallback'))
  assert.ok(result.errors.some((error) => error.includes('1 B2B token(s)')))
})

test('dimension-level pricing drift fails', () => {
  const snapshot = passingSnapshot()
  snapshot.options.CacheRatio['gpt-5.4'] = 1
  snapshot.options.CreateCacheRatio['gpt-5.5'] = 1.25
  snapshot.options.ImageRatio['gpt-image-2'] = 2

  const result = auditSnapshot(snapshot, policy)
  assert.equal(result.ok, false)
  assert.ok(result.errors.includes('cacheRatio.gpt-5.4: expected 0.1, found 1'))
  assert.ok(
    result.errors.includes('createCacheRatio.gpt-5.5: unexpected configured value'),
  )
  assert.ok(result.errors.includes('imageRatio.gpt-image-2: expected 1.6, found 2'))
})
