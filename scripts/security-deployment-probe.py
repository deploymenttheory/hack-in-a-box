#!/usr/bin/env python3
"""Agent-executed abuse walkthrough for a fresh, private synthetic deployment.

Run only on the isolated security Compose network. It uses real OIDC callbacks
and normal APIs, never SQL, test-clock controls or fabricated application cookies.
Session/callback/response payloads remain in memory; evidence contains labels only.
"""
import base64
from datetime import datetime, timedelta, timezone
import http.cookiejar
import json
import os
from pathlib import Path
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


observations = []


def require(condition, label):
    if not condition:
        raise AssertionError(label)


class Client:
    def __init__(self, origin):
        self.origin, self.csrf, self.person = origin, None, None
        self.cookies = http.cookiejar.CookieJar()
        self.http = urllib.request.build_opener(NoRedirect(), urllib.request.HTTPCookieProcessor(self.cookies), urllib.request.ProxyHandler({}))

    def call(self, method, path, want, label, body=None, revision=None, origin=None, csrf=True, key=None):
        require(path.startswith('/api/v1/'), 'native API path required')
        raw = json.dumps(body).encode() if body is not None else None
        headers = {'Origin': origin or self.origin, 'Cache-Control': 'no-store'}
        if raw is not None:
            headers['Content-Type'] = 'application/json'
        if method not in ('GET', 'HEAD') and csrf and self.csrf:
            headers['X-CSRF-Token'] = csrf if isinstance(csrf, str) else self.csrf
        if revision is not None:
            headers['If-Match'] = '"' + str(revision) + '"'
        if key:
            headers['Idempotency-Key'] = key
        request = urllib.request.Request(self.origin + path, raw, headers, method=method)
        try:
            response = self.http.open(request, timeout=15)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            require(response.status == want, label + ': unexpected status ' + str(response.status))
            require('no-store' in response.headers.get('Cache-Control', ''), label + ': cache policy')
            require(response.headers.get('X-Content-Type-Options') == 'nosniff', label + ': sniffing policy')
            require(response.headers.get('Access-Control-Allow-Origin') is None, label + ': no ambient cross-origin read grant')
            payload = response.read(4 << 20)
            observations.append({'scenario': label, 'status': response.status})
            if payload and response.headers.get('Content-Type', '').startswith('application/json'):
                return json.loads(payload)
            return payload

    def redirect(self, target, expected):
        try:
            response = self.http.open(urllib.request.Request(target), timeout=15)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            require(response.status == expected, 'configured OIDC redirect status')
            location = response.headers.get('Location')
            require(bool(location), 'configured OIDC redirect location')
            return urllib.parse.urljoin(target, location)

    @classmethod
    def login(cls, origin, actor):
        client = cls(origin)
        authorize = client.redirect(origin + '/auth/login?return_to=%2Fevents', 302)
        parsed = urllib.parse.urlsplit(authorize)
        require(parsed.scheme + '://' + parsed.netloc + parsed.path == 'http://provider:8080/authorize', 'exact configured synthetic issuer')
        query = urllib.parse.parse_qs(parsed.query)
        require(query.get('code_challenge_method') == ['S256'] and bool(query.get('nonce')), 'real PKCE/nonce request')
        query['actor'] = [actor]
        callback = client.redirect(urllib.parse.urlunsplit(parsed._replace(query=urllib.parse.urlencode(query, doseq=True))), 302)
        parsed_callback = urllib.parse.urlsplit(callback)
        require(parsed_callback.scheme + '://' + parsed_callback.netloc + parsed_callback.path == origin + '/auth/callback', 'exact portal callback')
        client.redirect(callback, 303)
        session = client.call('GET', '/api/v1/session', 200, 'real-oidc-session-' + actor + '-' + urllib.parse.urlsplit(origin).hostname)
        client.csrf, client.person = session['csrf_token'], session['person']['id']
        require(len(client.csrf) >= 32 and bool(client.person), 'native identity and CSRF context')
        return client


def event_input(name):
    live = datetime.now(timezone.utc).replace(microsecond=0) + timedelta(days=15)
    iso = lambda value: value.isoformat().replace('+00:00', 'Z')
    return {'id': str(uuid.uuid4()), 'name': 'Synthetic security ' + name, 'slug': 'security-' + str(uuid.uuid4()), 'acknowledge_competition_restriction': True,
            'configuration': {'description': 'Isolated security verification only', 'timezone': 'Europe/London', 'venue': 'Synthetic', 'capacity': 10, 'minimum_attendance': 1,
                              'live_start': iso(live), 'live_end': iso(live + timedelta(hours=8)), 'submission_cutoff': iso(live + timedelta(hours=7)),
                              'team_freeze': iso(live - timedelta(days=8)), 'admission_cutoff': iso(live - timedelta(days=9)), 'dst_choice': '',
                              'calendar': {'working_days': [1, 2, 3, 4, 5], 'excluded_dates': []}, 'schedule': []}}


def main():
    require(os.environ.get('HACKBOX_SECURITY_ISOLATED') == 'synthetic-only', 'isolated synthetic deployment flag required')
    for portal in ('admin', 'public'):
        anonymous = Client('http://' + portal + ':8080')
        for route in ('session', 'events', 'projects'):
            anonymous.call('GET', '/api/v1/' + route, 401, 'anonymous-' + portal + '-' + route, origin='https://attacker.invalid')
    owner = Client.login('http://admin:8080', 'owner')
    public = Client.login('http://public:8080', 'owner')
    stranger = Client.login('http://admin:8080', 'stranger')
    require(owner.person == public.person and owner.csrf != public.csrf, 'independent portals retain one identity with distinct session contexts')
    events = [owner.call('POST', '/api/v1/events', 200, 'native-create-' + name, event_input(name)) for name in ('Harbour', 'Orchard')]
    first, second = events
    base = '/api/v1/events/' + first['id']
    stranger.call('GET', base, 403, 'guessed-private-event')
    stranger.call('POST', '/api/v1/events', 403, 'unprivileged-event-creation', event_input('Denied'))
    for name, origin, csrf, expected in (('missing-csrf', None, False, 400), ('invalid-csrf', None, 'x' * 32, 403), ('foreign-origin', 'https://attacker.invalid', True, 403), ('origin-suffix', 'http://admin:8080.attacker.invalid', True, 403)):
        owner.call('POST', '/api/v1/events', expected, name, event_input('Unsent'), origin=origin, csrf=csrf)
    require(len(owner.call('GET', '/api/v1/events', 200, 'denied-commands-created-no-events')['items']) == 2, 'denied mutation persisted no event')
    forged = event_input('Forged'); forged['platform_admin'] = True
    owner.call('POST', '/api/v1/events', 400, 'unknown-privilege-field', forged)
    body = {'filename': 'safe.txt', 'media_type': 'text/plain', 'content': base64.b64encode(b'PRIVATE-SECURITY121-MANUAL-FILE').decode()}
    file = owner.call('POST', base + '/files', 201, 'native-safe-file-positive-control', body, revision=first['revision'], key=str(uuid.uuid4()))
    private_file = base + '/files/' + file['id']
    stranger.call('GET', private_file, 403, 'guessed-private-file')
    owner.call('GET', '/api/v1/events/' + second['id'] + '/files/' + file['id'], 404, 'foreign-event-file-id-substitution')
    require(public.call('GET', private_file, 200, 'independent-public-session-authorised-file') == b'PRIVATE-SECURITY121-MANUAL-FILE', 'positive file bytes')
    malicious = {'filename': 'active.png', 'media_type': 'image/png', 'content': base64.b64encode(b'<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>').decode()}
    owner.call('POST', base + '/files', 400, 'active-asset-mime-spoof', malicious, revision=first['revision'], key=str(uuid.uuid4()))
    stranger.call('GET', base + '/exports/readiness.csv?audience=organiser', 403, 'guessed-organiser-export')
    content_id = str(uuid.uuid4())
    page = {'kind': 'page', 'document': {'title': 'PRIVATE-SECURITY121-DRAFT', 'markdown': 'PRIVATE-SECURITY121-DRAFT-BODY', 'audience': {'confirmed': True, 'roles': [], 'teams': []}, 'attachments': [], 'scheduled_at': None}}
    saved = owner.call('PUT', base + '/content/' + content_id, 200, 'native-unreleased-content-positive-control', page, revision=0)
    require(saved['state'] == 'draft', 'private draft was not published by saving')
    stranger.call('GET', base + '/content/' + content_id, 404, 'direct-unreleased-content-url')
    stranger.call('GET', base + '/content/' + content_id + '?preview=true', 404, 'preview-query-is-not-authority')
    grant = {'person_id': stranger.person, 'role': 'mentor', 'active': True, 'acknowledge_competition_restriction': True}
    stranger.call('PUT', base + '/staff', 403, 'self-granted-role-attack', grant, revision=first['revision'])
    current = owner.call('GET', base, 200, 'read-current-role-change-base')
    owner.call('PUT', base + '/staff', 200, 'explicit-native-role-grant', grant, revision=current['revision'])
    stranger.call('GET', '/api/v1/session', 200, 'existing-session-remains-authenticated-after-role-grant')
    stranger.call('GET', base, 200, 'new-current-role-positive-control')
    stranger.call('GET', '/api/v1/events/' + second['id'], 403, 'role-does-not-cross-events')
    stranger.call('GET', base + '/content/' + content_id + '?preview=true', 404, 'mentor-role-not-early-draft-access')
    current = owner.call('GET', base, 200, 'read-current-revocation-base')
    grant['active'] = False
    owner.call('PUT', base + '/staff', 200, 'explicit-native-role-revocation', grant, revision=current['revision'])
    stranger.call('GET', '/api/v1/session', 200, 'revoked-role-is-independent-of-identity-session')
    stranger.call('GET', base, 403, 'existing-session-current-role-revocation')
    stranger = Client.login('http://admin:8080', 'stranger')
    stranger.call('GET', base, 403, 'fresh-login-cannot-resurrect-revoked-role')
    # Establish positive private-file/draft access before revocation, then reuse
    # that same valid identity session: denial must come from current authority.
    current = owner.call('GET', base, 200, 'read-organiser-grant-base')
    organiser = {'person_id': stranger.person, 'role': 'organiser', 'active': True, 'acknowledge_competition_restriction': True}
    owner.call('PUT', base + '/staff', 200, 'explicit-native-organiser-grant', organiser, revision=current['revision'])
    require(stranger.call('GET', private_file, 200, 'organiser-private-file-positive-control') == b'PRIVATE-SECURITY121-MANUAL-FILE', 'organiser file positive bytes')
    stranger.call('GET', base + '/content/' + content_id + '?preview=true', 200, 'organiser-private-draft-positive-control')
    current = owner.call('GET', base, 200, 'read-organiser-revocation-base')
    organiser['active'] = False
    owner.call('PUT', base + '/staff', 200, 'explicit-native-organiser-revocation', organiser, revision=current['revision'])
    stranger.call('GET', private_file, 403, 'same-session-revoked-organiser-file')
    stranger.call('GET', base + '/content/' + content_id + '?preview=true', 404, 'same-session-revoked-organiser-draft')
    stranger.call('GET', base, 403, 'same-session-revoked-organiser-event')
    stranger = Client.login('http://admin:8080', 'stranger')
    stranger.call('GET', private_file, 403, 'fresh-login-cannot-resurrect-organiser-file')
    public.call('POST', '/api/v1/logout', 204, 'logout-public-session-only')
    public.call('GET', '/api/v1/session', 401, 'logged-out-cookie-denied')
    owner.call('GET', base, 200, 'independent-admin-session-survives-public-logout')
    receipt = {'scope': 'agent-executed private normal-command deployment abuse; future/live competition, showcase browser and mentor feature retain separate proofs',
               'normal_api_commands': True, 'configured_oidc_pkce': True, 'test_clock_or_sql_or_fabricated_session': False,
               'application_source': os.environ['HACKBOX_SECURITY_SOURCE'], 'verification_source': os.environ.get('HACKBOX_SECURITY_VERIFICATION_SOURCE', 'unrecorded'), 'observations': observations, 'complete_release_security': False}
    Path('/artifacts/security-deployment.json').write_text(json.dumps(receipt, indent=2) + '\n')
    print('PASS isolated normal-command security walkthrough: ' + str(len(observations)) + ' observed API outcomes; no payload/session evidence retained')


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        # No exception traceback: urllib errors can contain callback URLs/codes.
        safe = str(error) if isinstance(error, AssertionError) else type(error).__name__
        print('FAIL isolated security walkthrough: ' + safe, file=sys.stderr)
        sys.exit(1)
