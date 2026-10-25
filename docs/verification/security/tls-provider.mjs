// External synthetic TLS terminator and RS256 OIDC issuer for #121 only.
// No application API success is fabricated; app requests are proxied verbatim.
import http from 'node:http'
import https from 'node:https'
import { readFileSync } from 'node:fs'
import { generateKeyPairSync, randomBytes, randomUUID, createHash, sign } from 'node:crypto'

const issuer = 'https://security-issuer.test:8443'
const clients = new Map([
  ['security-public', 'https://security-public.test:8443/auth/callback'],
  ['security-admin', 'https://security-admin.test:8443/auth/callback'],
])
const secret = 'synthetic-security-tls-only'
const { publicKey, privateKey } = generateKeyPairSync('rsa', { modulusLength: 2048 })
const kid = randomUUID(), codes = new Map(), tokens = new Map()
const jwk = { ...publicKey.export({ format: 'jwk' }), kid, use: 'sig', alg: 'RS256' }
const people = new Map([
  ['owner', { sub: 'owner', name: 'Synthetic security owner', email: 'security-owner@example.test', email_verified: true }],
  ['stranger', { sub: 'stranger', name: 'Synthetic security stranger', email: 'security-stranger@example.test', email_verified: true }],
])
const json = (res, status, body) => { res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' }); res.end(JSON.stringify(body)) }
const encode = value => Buffer.from(JSON.stringify(value)).toString('base64url')
function jwt(claims) {
  const unsigned = encode({ alg: 'RS256', typ: 'JWT', kid }) + '.' + encode(claims)
  return unsigned + '.' + sign('RSA-SHA256', Buffer.from(unsigned), privateKey).toString('base64url')
}
function proxy(req, res, host) {
  const target = host === 'security-public.test' ? 'public' : host === 'security-admin.test' ? 'admin' : undefined
  if (!target) return json(res, 404, { error: 'unknown_synthetic_host' })
  const upstream = http.request({ hostname: target, port: 8080, method: req.method, path: req.url, headers: req.headers }, response => {
    res.writeHead(response.statusCode, response.headers); response.pipe(res)
  })
  upstream.setTimeout(15000, () => upstream.destroy())
  upstream.on('error', () => { if (!res.headersSent) json(res, 502, { error: 'synthetic_upstream_unavailable' }); else res.destroy() })
  req.pipe(upstream)
}
async function handle(req, res, encrypted) {
  try {
    const url = new URL(req.url, issuer), host = (req.headers.host ?? '').split(':')[0]
    if (host !== 'security-issuer.test') return proxy(req, res, host)
    if (!encrypted) return json(res, 400, { error: 'issuer_requires_tls' })
    if (req.method === 'GET' && url.pathname === '/health') return json(res, 200, { ready: true })
    if (req.method === 'GET' && url.pathname === '/.well-known/openid-configuration') return json(res, 200, {
      issuer, authorization_endpoint: issuer + '/authorize', token_endpoint: issuer + '/token',
      jwks_uri: issuer + '/jwks', userinfo_endpoint: issuer + '/userinfo',
      response_types_supported: ['code'], subject_types_supported: ['public'], id_token_signing_alg_values_supported: ['RS256'],
      code_challenge_methods_supported: ['S256'], scopes_supported: ['openid', 'profile', 'email'],
      token_endpoint_auth_methods_supported: ['client_secret_basic', 'client_secret_post'],
    })
    if (req.method === 'GET' && url.pathname === '/jwks') return json(res, 200, { keys: [jwk] })
    if (req.method === 'GET' && url.pathname === '/authorize') {
      const q = url.searchParams, client = q.get('client_id'), person = people.get(q.get('actor'))
      if (!person || clients.get(client) !== q.get('redirect_uri') || q.get('response_type') !== 'code' || !q.get('state') || !q.get('nonce') || q.get('code_challenge_method') !== 'S256' || !/^[A-Za-z0-9_-]{43}$/.test(q.get('code_challenge') ?? '')) return json(res, 400, { error: 'invalid_request' })
      for (const [id, value] of codes) if (value.expires < Date.now()) codes.delete(id)
      if (codes.size >= 128) return json(res, 429, { error: 'temporarily_unavailable' })
      const code = randomBytes(32).toString('base64url')
      codes.set(code, { client, redirect: q.get('redirect_uri'), nonce: q.get('nonce'), challenge: q.get('code_challenge'), person, expires: Date.now() + 120000 })
      const target = new URL(q.get('redirect_uri')); target.searchParams.set('code', code); target.searchParams.set('state', q.get('state'))
      res.writeHead(302, { Location: target.toString(), 'Cache-Control': 'no-store' }); return res.end()
    }
    if (req.method === 'POST' && url.pathname === '/token') {
      let raw = ''
      for await (const chunk of req) { raw += chunk; if (raw.length > 8192) return json(res, 413, { error: 'invalid_request' }) }
      const q = new URLSearchParams(raw), basic = req.headers.authorization?.startsWith('Basic ') ? Buffer.from(req.headers.authorization.slice(6), 'base64').toString().split(':').map(decodeURIComponent) : []
      const client = q.get('client_id') || basic[0], provided = q.get('client_secret') || basic[1]
      if (!clients.has(client) || provided !== secret || q.get('grant_type') !== 'authorization_code') return json(res, 401, { error: 'invalid_client' })
      const code = codes.get(q.get('code')); codes.delete(q.get('code'))
      if (!code || code.expires < Date.now() || code.client !== client || code.redirect !== q.get('redirect_uri') || !q.get('code_verifier') || createHash('sha256').update(q.get('code_verifier')).digest('base64url') !== code.challenge) return json(res, 400, { error: 'invalid_grant' })
      const now = Math.floor(Date.now() / 1000), access = randomBytes(32).toString('base64url')
      for (const [id, token] of tokens) if (token.expires < now) tokens.delete(id)
      if (tokens.size >= 128) return json(res, 429, { error: 'temporarily_unavailable' })
      tokens.set(access, { person: code.person, expires: now + 3600 })
      return json(res, 200, { access_token: access, token_type: 'Bearer', expires_in: 3600, id_token: jwt({ ...code.person, iss: issuer, aud: client, azp: client, nonce: code.nonce, iat: now, exp: now + 3600 }) })
    }
    if (req.method === 'GET' && url.pathname === '/userinfo') {
      const token = tokens.get(req.headers.authorization?.replace(/^Bearer /, ''))
      return token && token.expires > Date.now() / 1000 ? json(res, 200, token.person) : json(res, 401, { error: 'invalid_token' })
    }
    json(res, 404, { error: 'not_found' })
  } catch { if (!res.headersSent) json(res, 400, { error: 'invalid_synthetic_request' }); else res.destroy() }
}
https.createServer({ key: readFileSync('/tls/leaf.key'), cert: readFileSync('/tls/leaf.pem'), minVersion: 'TLSv1.2' }, (req, res) => { void handle(req, res, true) }).listen(8443, '0.0.0.0')
// Private cleartext listener exists only to prove Secure application cookies are
// withheld on a downgrade to the same hostname. It still proxies to the real API.
http.createServer((req, res) => { void handle(req, res, false) }).listen(8080, '0.0.0.0', () => console.log('Synthetic security TLS issuer/proxy ready'))
