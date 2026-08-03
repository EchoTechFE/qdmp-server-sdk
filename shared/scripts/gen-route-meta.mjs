#!/usr/bin/env node
// 读取 shared/openapi.yaml，提取每个 operation 的路由元数据以及全局错误码清单，
// 生成 shared/generated/route-meta.json 与 shared/generated/error-codes.json。
//
// shared/openapi.yaml 的内容是合法 JSON（JSON 是 YAML 的严格子集），所以这里直接用
// Node 内置的 fs + JSON.parse 读取，零第三方依赖。三端（Node/Java/Go）最终的类型/
// 语言格式生成不在本脚本范围内，这里只产出通用的中间 JSON 产物。

import { readFileSync, mkdirSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)

const SHARED_DIR = path.resolve(__dirname, '..')
const SPEC_PATH = path.join(SHARED_DIR, 'openapi.yaml')
const OUT_DIR = path.join(SHARED_DIR, 'generated')
const ROUTE_META_PATH = path.join(OUT_DIR, 'route-meta.json')
const ERROR_CODES_PATH = path.join(OUT_DIR, 'error-codes.json')

const HTTP_METHODS = ['get', 'post', 'put', 'patch', 'delete', 'options', 'head', 'trace']

function loadSpec() {
  const raw = readFileSync(SPEC_PATH, 'utf8')
  let spec
  try {
    spec = JSON.parse(raw)
  } catch (err) {
    throw new Error(`无法解析 ${SPEC_PATH}：内容不是合法 JSON（本项目约定 openapi.yaml 内容为合法 JSON）。原始错误：${err.message}`)
  }
  return spec
}

function extractRouteMeta(spec) {
  const paths = spec.paths ?? {}
  const routeMeta = []
  const missingFields = []

  for (const [routePath, pathItem] of Object.entries(paths)) {
    for (const method of HTTP_METHODS) {
      const operation = pathItem[method]
      if (!operation) continue

      const operationId = operation.operationId
      const authScheme = operation['x-qdmp-auth-scheme']
      const tokenRequired = operation['x-qdmp-token-required']

      const entry = {
        operationId: operationId ?? null,
        method: method.toUpperCase(),
        path: routePath,
        authScheme: authScheme ?? null,
        tokenRequired: typeof tokenRequired === 'boolean' ? tokenRequired : null,
      }
      routeMeta.push(entry)

      if (!operationId) missingFields.push(`${method.toUpperCase()} ${routePath} 缺少 operationId`)
      if (!authScheme) missingFields.push(`${method.toUpperCase()} ${routePath} 缺少 x-qdmp-auth-scheme`)
      if (typeof tokenRequired !== 'boolean') missingFields.push(`${method.toUpperCase()} ${routePath} 缺少/非法 x-qdmp-token-required`)
    }
  }

  // operationId 是三端查路由元数据的键，重复了就会有一条路由被另一条盖掉：
  // 生成物看着正常，实际少了一个接口，且各端的覆盖检查也看不出来。
  const seen = new Map()
  for (const entry of routeMeta) {
    if (!entry.operationId) continue
    const prev = seen.get(entry.operationId)
    if (prev) {
      missingFields.push(
        `operationId ${entry.operationId} 重复：${prev.method} ${prev.path} 与 ${entry.method} ${entry.path}`,
      )
    } else {
      seen.set(entry.operationId, entry)
    }
  }

  return { routeMeta, missingFields }
}

function extractErrorCodes(spec) {
  const errorCodes = spec['x-error-codes']
  if (!Array.isArray(errorCodes)) {
    throw new Error('顶层 x-error-codes 缺失或不是数组')
  }
  return errorCodes
}

function main() {
  const spec = loadSpec()

  const { routeMeta, missingFields } = extractRouteMeta(spec)
  if (missingFields.length > 0) {
    console.error('发现字段缺失，请检查 openapi.yaml：')
    for (const m of missingFields) console.error(`  - ${m}`)
    process.exitCode = 1
    return
  }

  const errorCodes = extractErrorCodes(spec)

  mkdirSync(OUT_DIR, { recursive: true })
  writeFileSync(ROUTE_META_PATH, JSON.stringify(routeMeta, null, 2) + '\n', 'utf8')
  writeFileSync(ERROR_CODES_PATH, JSON.stringify(errorCodes, null, 2) + '\n', 'utf8')

  console.log(`已生成 ${ROUTE_META_PATH}（${routeMeta.length} 条 operation）`)
  console.log(`已生成 ${ERROR_CODES_PATH}（${errorCodes.length} 条错误码）`)

  // ── 自我验证：重新读回生成的 JSON，确认合法且字段完整 ──
  const verifyRouteMeta = JSON.parse(readFileSync(ROUTE_META_PATH, 'utf8'))
  const verifyErrorCodes = JSON.parse(readFileSync(ERROR_CODES_PATH, 'utf8'))

  const requiredKeys = ['operationId', 'method', 'path', 'authScheme', 'tokenRequired']
  for (const entry of verifyRouteMeta) {
    for (const key of requiredKeys) {
      if (!(key in entry) || entry[key] === null) {
        throw new Error(`route-meta.json 校验失败：${JSON.stringify(entry)} 缺少或为 null 的字段 ${key}`)
      }
    }
  }
  for (const entry of verifyErrorCodes) {
    if (!('code' in entry) || !('name' in entry) || !('message' in entry) || !('source' in entry)) {
      throw new Error(`error-codes.json 校验失败：${JSON.stringify(entry)} 缺少 code/name/message/source`)
    }
  }

  console.log(`校验通过：route-meta.json 共 ${verifyRouteMeta.length} 条，字段完整；error-codes.json 共 ${verifyErrorCodes.length} 条，字段完整。`)
}

main()
