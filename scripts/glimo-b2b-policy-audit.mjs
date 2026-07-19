#!/usr/bin/env node

import fs from 'node:fs/promises'
import path from 'node:path'
import process from 'node:process'
import { fileURLToPath, pathToFileURL } from 'node:url'

const SCRIPT_DIR = path.dirname(fileURLToPath(import.meta.url))
const DEFAULT_POLICY_PATH = path.resolve(
  SCRIPT_DIR,
  '../config/glimo-b2b-commercial-policy.json',
)
const EPSILON = 1e-12

function approximatelyEqual(actual, expected) {
  return (
    typeof actual === 'number' &&
    typeof expected === 'number' &&
    Math.abs(actual - expected) <= EPSILON
  )
}

function parseArguments(argv) {
  const result = { policyPath: DEFAULT_POLICY_PATH, snapshotPath: null, json: false }
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index]
    if (argument === '--policy') {
      result.policyPath = path.resolve(argv[++index] ?? '')
    } else if (argument === '--snapshot') {
      result.snapshotPath = path.resolve(argv[++index] ?? '')
    } else if (argument === '--json') {
      result.json = true
    } else {
      throw new Error(`unknown argument: ${argument}`)
    }
  }
  return result
}

async function readStandardInput() {
  const chunks = []
  for await (const chunk of process.stdin) chunks.push(chunk)
  return Buffer.concat(chunks).toString('utf8')
}

export async function loadJson(filePath) {
  return JSON.parse(await fs.readFile(filePath, 'utf8'))
}

function addNumericCheck(errors, label, actual, expected) {
  if (!approximatelyEqual(actual, expected)) {
    errors.push(`${label}: expected ${expected}, found ${String(actual)}`)
  }
}

function sorted(values) {
  return [...values].sort((left, right) => left.localeCompare(right))
}

function customerRate(model, policy) {
  const ratio = policy.groups[model.group].usageRatio
  return Object.fromEntries(
    Object.entries(model.officialUsdPerMillion).map(([dimension, rate]) => [
      dimension,
      rate * ratio,
    ]),
  )
}

export function auditSnapshot(snapshot, policy) {
  const errors = []
  const warnings = []
  const options = snapshot.options ?? {}
  const groupRatio = options.GroupRatio ?? {}
  const topupGroupRatio = options.TopupGroupRatio ?? {}

  for (const [group, expected] of Object.entries(policy.groups)) {
    addNumericCheck(
      errors,
      `GroupRatio.${group}`,
      groupRatio[group],
      expected.usageRatio,
    )
    addNumericCheck(
      errors,
      `TopupGroupRatio.${group}`,
      topupGroupRatio[group],
      expected.topupRatio,
    )
  }

  const expectedByGroup = new Map()
  for (const [modelName, model] of Object.entries(policy.models)) {
    const models = expectedByGroup.get(model.group) ?? new Set()
    models.add(modelName)
    expectedByGroup.set(model.group, models)
  }

  const actualByGroup = new Map()
  for (const ability of snapshot.abilities ?? []) {
    const models = actualByGroup.get(ability.group) ?? new Set()
    models.add(ability.model)
    actualByGroup.set(ability.group, models)
    if (ability.abilityEnabled !== true) {
      errors.push(`ability ${ability.group}/${ability.model} is disabled`)
    }
    if (ability.channelStatus !== 1) {
      errors.push(
        `channel ${ability.channelId} for ${ability.group}/${ability.model} is not enabled`,
      )
    }
  }

  for (const [group, expectedModels] of expectedByGroup) {
    const actualModels = actualByGroup.get(group) ?? new Set()
    const missing = sorted([...expectedModels].filter((model) => !actualModels.has(model)))
    const extra = sorted([...actualModels].filter((model) => !expectedModels.has(model)))
    if (missing.length > 0) errors.push(`${group} missing models: ${missing.join(', ')}`)
    if (extra.length > 0) errors.push(`${group} has unapproved models: ${extra.join(', ')}`)
  }

  for (const model of policy.excludedModels) {
    for (const [group, actualModels] of actualByGroup) {
      if (actualModels.has(model)) errors.push(`${model} is exposed through ${group}`)
    }
  }

  const ratioMaps = {
    modelRatio: options.ModelRatio ?? {},
    completionRatio: options.CompletionRatio ?? {},
    cacheRatio: options.CacheRatio ?? {},
    createCacheRatio: options.CreateCacheRatio ?? {},
    imageRatio: options.ImageRatio ?? {},
  }
  for (const [modelName, model] of Object.entries(policy.models)) {
    for (const [dimension, expected] of Object.entries(model.newApi)) {
      addNumericCheck(
        errors,
        `${dimension}.${modelName}`,
        ratioMaps[dimension]?.[modelName],
        expected,
      )
    }
    for (const optionalDimension of ['createCacheRatio', 'imageRatio']) {
      if (
        !(optionalDimension in model.newApi) &&
        Object.hasOwn(ratioMaps[optionalDimension], modelName)
      ) {
        errors.push(`${optionalDimension}.${modelName}: unexpected configured value`)
      }
    }
  }

  const autoGroups = new Set(options.AutoGroups ?? [])
  for (const group of ['b2b', 'b2b-deepseek']) {
    if (!autoGroups.has(group)) errors.push(`AutoGroups is missing ${group}`)
  }
  if (options.DefaultUseAutoGroup !== true) {
    errors.push('DefaultUseAutoGroup must be true')
  }
  const exposedGroups = options.UserUsableGroups ?? {}
  for (const group of ['b2b', 'b2b-deepseek']) {
    if (Object.hasOwn(exposedGroups, group)) {
      errors.push(`UserUsableGroups must not expose ${group}`)
    }
  }
  const b2bSpecialGroups =
    options['group_ratio_setting.group_special_usable_group']?.b2b ?? {}
  for (const required of ['+:auto', '+:b2b-deepseek']) {
    if (!Object.hasOwn(b2bSpecialGroups, required)) {
      errors.push(`b2b special usable groups is missing ${required}`)
    }
  }
  if (Object.hasOwn(b2bSpecialGroups, '+:default')) {
    errors.push('b2b must not have a +:default fallback')
  }

  if ((snapshot.b2bTokenViolationCount ?? 0) !== 0) {
    errors.push(
      `${snapshot.b2bTokenViolationCount} B2B token(s) are not auto or allow cross-group retry`,
    )
  }
  if ((snapshot.b2bUserCount ?? 0) > 0) {
    warnings.push(
      `${snapshot.b2bUserCount} production user(s) currently belong to b2b; verify each assignment is approved`,
    )
  }

  const customerRates = Object.fromEntries(
    Object.entries(policy.models).map(([name, model]) => [
      name,
      {
        category: model.category,
        group: model.group,
        usdPerMillion: customerRate(model, policy),
      },
    ]),
  )

  return {
    ok: errors.length === 0,
    policyVersion: policy.version,
    capturedAt: snapshot.capturedAt ?? null,
    errors,
    warnings,
    summary: {
      approvedModels: Object.keys(policy.models).length,
      b2bUsers: snapshot.b2bUserCount ?? 0,
      b2bTokens: snapshot.b2bTokenCount ?? 0,
    },
    customerRates,
  }
}

function renderText(result) {
  const lines = [
    `Glimo B2B policy ${result.policyVersion}: ${result.ok ? 'PASS' : 'FAIL'}`,
    `Snapshot: ${result.capturedAt ?? 'unknown'}`,
    `Models: ${result.summary.approvedModels}; B2B users: ${result.summary.b2bUsers}; B2B tokens: ${result.summary.b2bTokens}`,
  ]
  for (const error of result.errors) lines.push(`ERROR: ${error}`)
  for (const warning of result.warnings) lines.push(`WARN: ${warning}`)
  return `${lines.join('\n')}\n`
}

async function main() {
  const args = parseArguments(process.argv.slice(2))
  const policy = await loadJson(args.policyPath)
  const rawSnapshot = args.snapshotPath
    ? await fs.readFile(args.snapshotPath, 'utf8')
    : await readStandardInput()
  if (rawSnapshot.trim() === '') throw new Error('snapshot input is empty')
  const result = auditSnapshot(JSON.parse(rawSnapshot), policy)
  process.stdout.write(args.json ? `${JSON.stringify(result, null, 2)}\n` : renderText(result))
  if (!result.ok) process.exitCode = 1
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch((error) => {
    process.stderr.write(`Glimo B2B policy audit failed: ${error.message}\n`)
    process.exitCode = 2
  })
}
