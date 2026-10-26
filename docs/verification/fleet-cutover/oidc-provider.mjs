// Verification-only external identity provider. Never packaged into app images.
import http from 'node:http'
import { generateKeyPairSync, randomBytes, randomUUID, createHash, sign } from 'node:crypto'

// Local synthetic identity provider. The app still performs its normal OIDC,
// PKCE, nonce, signature, session and resource-authorisation checks.
const issuer = 'http://oidc.localhost:19190'
const client = 'fleet-cutover-preview'
const redirects = new Set(['http://127.0.0.1:18180/auth/callback', 'http://127.0.0.1:18181/auth/callback'])
const { publicKey, privateKey } = generateKeyPairSync('rsa', { modulusLength: 2048 })
const kid = randomUUID()
const jwk = { ...publicKey.export({ format: 'jwk' }), kid, use: 'sig', alg: 'RS256' }
const codes = new Map()
const tokens = new Map()
const profile = { sub: 'fleet-cutover-owner', name: 'Synthetic cutover organiser', email: 'fleet-cutover-organiser@hackbox.localhost', email_verified: true }
const encode = value => Buffer.from(JSON.stringify(value)).toString('base64url')
function json(res, status, value) {
  res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' })
  res.end(JSON.stringify(value))
}
function jwt(claims) {
  const unsigned = `${encode({ alg: 'RS256', typ: 'JWT', kid })}.${encode(claims)}`
  return `${unsigned}.${sign('RSA-SHA256', Buffer.from(unsigned), privateKey).toString('base64url')}`
}
http.createServer(async (req, res) => {
  try {
    const url = new URL(req.url, issuer)
    if (req.method === 'GET' && url.pathname === '/health') return json(res, 200, { ready: true })
    if (req.method === 'GET' && url.pathname === '/.well-known/openid-configuration') return json(res, 200, {
      issuer, authorization_endpoint: `${issuer}/authorize`, token_endpoint: `${issuer}/token`,
      jwks_uri: `${issuer}/jwks`, userinfo_endpoint: `${issuer}/userinfo`,
      response_types_supported: ['code'], subject_types_supported: ['public'],
      id_token_signing_alg_values_supported: ['RS256'], code_challenge_methods_supported: ['S256'],
      scopes_supported: ['openid', 'profile', 'email'], token_endpoint_auth_methods_supported: ['none', 'client_secret_basic', 'client_secret_post'],
    })
    if (req.method === 'GET' && url.pathname === '/jwks') return json(res, 200, { keys: [jwk] })
    if (req.method === 'GET' && url.pathname === '/authorize') {
      const q = url.searchParams
      if (q.get('client_id') !== client || !redirects.has(q.get('redirect_uri')) || q.get('response_type') !== 'code' || !q.get('state') || !q.get('nonce') || !q.get('code_challenge') || q.get('code_challenge_method') !== 'S256') return json(res, 400, { error: 'invalid_request' })
      for (const [code, value] of codes) if (value.expires < Date.now()) codes.delete(code)
      if (codes.size >= 128) return json(res, 429, { error: 'temporarily_unavailable' })
      const code = randomBytes(32).toString('base64url')
      codes.set(code, { redirect: q.get('redirect_uri'), nonce: q.get('nonce'), challenge: q.get('code_challenge'), expires: Date.now() + 120000 })
      const target = new URL(q.get('redirect_uri'))
      target.searchParams.set('code', code); target.searchParams.set('state', q.get('state'))
      res.writeHead(302, { Location: target.toString(), 'Cache-Control': 'no-store' }); return res.end()
    }
    if (req.method === 'POST' && url.pathname === '/token') {
      let raw = ''
      for await (const chunk of req) { raw += chunk; if (raw.length > 8192) return json(res, 413, { error: 'invalid_request' }) }
      const q = new URLSearchParams(raw)
      const basic = req.headers.authorization?.startsWith('Basic ') ? Buffer.from(req.headers.authorization.slice(6), 'base64').toString().split(':') : []
      if ((q.get('client_id') || basic[0]) !== client || q.get('client_secret') || basic[1] || q.get('grant_type') !== 'authorization_code') return json(res, 401, { error: 'invalid_client' })
      const record = codes.get(q.get('code')); codes.delete(q.get('code'))
      if (!record || record.expires < Date.now() || record.redirect !== q.get('redirect_uri') || !q.get('code_verifier') || createHash('sha256').update(q.get('code_verifier')).digest('base64url') !== record.challenge) return json(res, 400, { error: 'invalid_grant' })
      const now = Math.floor(Date.now() / 1000), access = randomBytes(32).toString('base64url')
      tokens.set(access, now + 3600)
      return json(res, 200, { access_token: access, token_type: 'Bearer', expires_in: 3600, id_token: jwt({ ...profile, iss: issuer, aud: client, azp: client, nonce: record.nonce, iat: now, exp: now + 3600 }) })
    }
    if (req.method === 'GET' && url.pathname === '/userinfo') {
      const access = req.headers.authorization?.replace(/^Bearer /, '')
      return tokens.get(access) > Date.now() / 1000 ? json(res, 200, profile) : json(res, 401, { error: 'invalid_token' })
    }
    json(res, 404, { error: 'not_found' })
  } catch { json(res, 400, { error: 'invalid_request' }) }
}).listen(19190, '0.0.0.0', () => console.log('Local synthetic OIDC provider ready on port 19190'))
