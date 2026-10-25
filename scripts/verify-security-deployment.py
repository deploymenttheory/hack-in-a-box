#!/usr/bin/env python3
"""Execute the private normal-command security walkthrough with pinned images."""
import argparse
import io
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile
import uuid

ROOT = Path(__file__).resolve().parents[1]


def output(command):
    return subprocess.check_output(command, cwd=ROOT, text=True).strip()


def resources(project):
    return {kind: output(command + ['--filter', 'label=com.docker.compose.project=' + project])
            for kind, command in [('containers', ['docker', 'ps', '-aq']),
                                  ('volumes', ['docker', 'volume', 'ls', '-q']),
                                  ('networks', ['docker', 'network', 'ls', '-q'])]}


def run(command, path, env):
    result = subprocess.run(command, cwd=ROOT, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    # The probe deliberately emits only fixed labels/statuses. No app/provider
    # logs, raw responses, callback URLs, cookies or session traces are collected.
    path.write_text(result.stdout)
    path.chmod(0o600)
    if result.returncode:
        raise RuntimeError('Verification command failed; inspect private ' + path.name)


def production_changes(application_source, verification_source):
    paths = output(['git', 'diff', '--name-only', application_source, verification_source, '--',
                    'cmd', 'internal', 'api', 'go.mod', 'go.sum', 'Dockerfile', 'web/src',
                    'web/package.json', 'web/package-lock.json', 'web/vite.config.ts']).splitlines()
    return [path for path in paths if not path.endswith(('_test.go', '.test.ts', '.test.tsx'))]



def synthetic_tls(directory):
    directory.mkdir(mode=0o700)
    ca_config = directory / 'ca.conf'
    ca_config.write_text("[req]\ndistinguished_name=dn\nx509_extensions=ca\nprompt=no\n[dn]\nCN=Isolated security121 synthetic CA\n[ca]\nbasicConstraints=critical,CA:TRUE\nkeyUsage=critical,keyCertSign,cRLSign\nsubjectKeyIdentifier=hash\n")
    extensions = directory / 'leaf.conf'
    extensions.write_text("basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\nsubjectAltName=DNS:security-public.test,DNS:security-admin.test,DNS:security-issuer.test\n")
    commands = [
        ['openssl', 'genrsa', '-out', str(directory / 'ca.key'), '2048'],
        ['openssl', 'req', '-new', '-x509', '-key', str(directory / 'ca.key'), '-sha256', '-days', '1', '-config', str(ca_config), '-out', str(directory / 'ca.pem')],
        ['openssl', 'req', '-new', '-newkey', 'rsa:2048', '-nodes', '-keyout', str(directory / 'leaf.key'), '-out', str(directory / 'leaf.csr'), '-subj', '/CN=security-public.test'],
        ['openssl', 'x509', '-req', '-in', str(directory / 'leaf.csr'), '-CA', str(directory / 'ca.pem'), '-CAkey', str(directory / 'ca.key'), '-CAcreateserial', '-out', str(directory / 'leaf.pem'), '-days', '1', '-sha256', '-extfile', str(extensions)],
        ['openssl', 'verify', '-CAfile', str(directory / 'ca.pem'), '-verify_hostname', 'security-public.test', str(directory / 'leaf.pem')],
    ]
    for command in commands:
        subprocess.run(command, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    # Individual read-only mounts expose only these synthetic files to the
    # nonroot fixture. Their private parent and the signing key never leave tmp.
    for name in ['ca.pem', 'leaf.pem', 'leaf.key']:
        (directory / name).chmod(0o444)
    return {name: hashlib.sha256((directory / name).read_bytes()).hexdigest() for name in ['ca.pem', 'leaf.pem']}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for kind in ['public', 'admin', 'provider', 'python']:
        parser.add_argument('--' + kind + '-image', required=True, help='Immutable sha256 image ID, never a mutable tag')
    parser.add_argument('--application-source', required=True, help='Full source SHA from the independently retained image build record')
    parser.add_argument('--tls', action='store_true', help='Production HTTPS issuer/origins, external synthetic TLS termination and actual Secure-cookie assertions; provider image must contain Node22')
    parser.add_argument('--artifacts', type=Path, required=True, help='New private directory outside this repository')
    args = parser.parse_args()
    if output(['git', 'status', '--porcelain']):
        parser.error('Commit the verification source before executing it')
    source = output(['git', 'rev-parse', 'HEAD'])
    if not re.fullmatch('[a-f0-9]{40}', args.application_source):
        parser.error('Application source must be a full source SHA')
    if output(['git', 'rev-parse', '--verify', args.application_source + '^{commit}']) != args.application_source:
        parser.error('Application source does not resolve exactly')
    if production_changes(args.application_source, source):
        parser.error('Pinned application images do not cover this verification source production code')
    artifacts = args.artifacts.resolve()
    if artifacts.exists() or artifacts.is_relative_to(ROOT):
        parser.error('Choose a new artifact directory outside this repository')
    images, embedded_sources = {}, {}
    for kind in ['public', 'admin', 'provider', 'python']:
        image = getattr(args, kind + '_image')
        if not re.fullmatch('sha256:[a-f0-9]{64}', image):
            parser.error(kind + ' image must be an immutable full image ID')
        if output(['docker', 'image', 'inspect', '--format', '{{.Id}}', image]) != image:
            parser.error(kind + ' image identity did not match')
        images[kind] = image
        if kind in ('public', 'admin'):
            embedded = output(['docker', 'image', 'inspect', '--format', '{{index .Config.Labels "org.opencontainers.image.revision"}}', image])
            if embedded not in ('', '<no value>', args.application_source):
                parser.error(kind + ' embedded source differs from the supplied build record')
            embedded_sources[kind] = embedded or '<no value>'
    project = 'hackbox-security121-' + source[:12] + '-' + uuid.uuid4().hex[:8]
    if any(resources(project).values()):
        parser.error('Private project already has resources; preserve its earlier evidence')
    artifacts.mkdir(parents=True, mode=0o700)
    env = dict(os.environ,
               HACKBOX_NORMAL_PUBLIC_IMAGE=images['public'], HACKBOX_NORMAL_ADMIN_IMAGE=images['admin'],
               HACKBOX_LIFECYCLE_IMAGE=images['provider'], HACKBOX_SECURITY_PYTHON_IMAGE=images['python'],
               HACKBOX_SECURITY_SOURCE=args.application_source, HACKBOX_SECURITY_VERIFICATION_SOURCE=source,
               HACKBOX_SECURITY_ARTIFACTS=str(artifacts))
    summary = {'verification_source': source, 'application_source': args.application_source,
               'image_source_attribution': 'Supplied independently retained build record; production source equivalence checked by this runner. Empty embedded labels are not claimed as independent source verification.',
               'embedded_application_sources': embedded_sources, 'images': images, 'project': project,
               'scope': 'Agent-executed normal API/OIDC abuse walkthrough; not future competition, complete security certification or full CI',
               'transport_profile': 'production HTTPS/external TLS and actual Secure cookies' if args.tls else 'private HTTP/development; not production TLS proof',
               'passed': False, 'complete_release_security': False}
    with tempfile.TemporaryDirectory(prefix='hackbox-security121-source-') as directory:
        snapshot = Path(directory)
        with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git', 'archive', source], cwd=ROOT))) as archive:
            archive.extractall(snapshot, filter='data')
        config = 'compose.tls.yaml' if args.tls else 'compose.yaml'
        if args.tls:
            runtime_tls = snapshot / 'synthetic-tls-runtime'
            summary['public_certificate_pem_sha256'] = synthetic_tls(runtime_tls)
            env['HACKBOX_SECURITY_TLS_DIR'] = str(runtime_tls)
        compose = ['docker', 'compose', '-p', project, '-f', str(snapshot / 'docs/verification/security' / config)]
        deployed = False
        try:
            run(compose + ['config', '--quiet'], artifacts / 'configuration.log', env)
            if any(resources(project).values()):
                raise RuntimeError('Private project appeared before startup; no deployment/cleanup attempted')
            deployed = True
            run(compose + ['up', '-d', '--wait', '--wait-timeout', '120', 'public', 'admin'], artifacts / 'startup.log', env)
            run(compose + ['run', '--rm', 'probe'], artifacts / 'walkthrough.log', env)
            receipt = json.loads((artifacts / 'security-deployment.json').read_text())
            if receipt['application_source'] != args.application_source or receipt['verification_source'] != source:
                raise RuntimeError('Walkthrough source receipt did not match the frozen execution')
            summary['observed_api_outcomes'] = len(receipt['observations'])
            summary['transport_assertions'] = receipt.get('transport_assertions', [])
            if args.tls and len(summary['transport_assertions']) != 3:
                raise RuntimeError('Supported TLS/cookie transport assertions did not all execute')
            summary['passed'] = True
        finally:
            if deployed:
                run(compose + ['down', '--volumes', '--remove-orphans'], artifacts / 'cleanup.log', env)
                summary['remaining_resources'] = resources(project)
                if any(summary['remaining_resources'].values()):
                    summary['passed'] = False
                    raise RuntimeError('Private project cleanup was incomplete')
            (artifacts / 'summary.json').write_text(json.dumps(summary, indent=2) + '\n')
    print('PASS private normal-command security walkthrough and scoped cleanup: ' + str(artifacts))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        # Tracebacks and command objects are unnecessary evidence and can include
        # a sensitive URL if a future transport implementation changes.
        message = str(error) if isinstance(error, RuntimeError) else type(error).__name__
        raise SystemExit('FAIL isolated security verification: ' + message)
