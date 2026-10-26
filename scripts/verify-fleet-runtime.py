#!/usr/bin/env python3
"""Build tracked normal images, browser-test a private UI deployment, clean only it."""
import argparse
import io
import json
import os
from pathlib import Path
import re
import socket
import subprocess
import tarfile
import tempfile
import uuid

ROOT = Path(__file__).resolve().parent.parent


def output(args, **kwargs):
    return subprocess.check_output(args, cwd=ROOT, text=True, **kwargs).strip()


def redact(value):
    value = re.sub(r'(https?://[^\s?]+)\?[^\s]+', r'\1?<redacted>', value)
    return re.sub(r'(postgres(?:ql)?://[^:]+:)[^@\s]+(@)', r'\1<redacted>\2', value)


def logged(args, path, env):
    result = subprocess.run(args, cwd=ROOT, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    path.write_text(redact(result.stdout))
    path.chmod(0o600)
    if result.returncode:
        raise RuntimeError(f'{args[0]} command failed ({result.returncode}); inspect {path}')


def project_resources(project):
    remaining = {}
    for kind, command in [('containers', ['docker', 'ps', '-aq']), ('volumes', ['docker', 'volume', 'ls', '-q']), ('networks', ['docker', 'network', 'ls', '-q'])]:
        remaining[kind] = output(command + ['--filter', f'label=com.docker.compose.project={project}'])
    return remaining


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--artifacts', type=Path, required=True, help='New private directory outside the repository')
    args = parser.parse_args()
    artifacts = args.artifacts.resolve()
    if artifacts.is_relative_to(ROOT) or artifacts.exists():
        parser.error('Artifact directory must be new and outside the repository')
    if output(['git', 'status', '--porcelain']):
        parser.error('Commit the verification source first; the runner builds only tracked HEAD')
    for port in [18180, 18181, 18182, 19190]:
        with socket.socket() as probe:
            probe.bind(('127.0.0.1', port))
    revision = output(['git', 'rev-parse', 'HEAD'])
    project = 'hackbox-fleet245-' + revision[:12] + '-' + uuid.uuid4().hex[:8]
    if any(project_resources(project).values()):
        parser.error('Private project resources already exist; inspect them rather than overwrite their proof')
    artifacts.mkdir(parents=True, mode=0o700)
    env = dict(os.environ, FLEET_UI_PROJECT=project, FLEET_UI_ARTIFACTS=str(artifacts), FLEET_UI_SOURCE_REVISION=revision)
    for kind in ['PUBLIC', 'ADMIN', 'GUIDE']:
        env[f'FLEET_UI_{kind}_IMAGE'] = f'hackbox-fleet245-{kind.lower()}:{revision[:12]}'
    # Keep every executable fixture and bind mount in the same tracked snapshot
    # until services are removed; live worktree edits cannot alter this run.
    with tempfile.TemporaryDirectory(prefix='hackbox-fleet245-source-') as temporary:
        snapshot = Path(temporary)
        archive = subprocess.check_output(['git', 'archive', revision], cwd=ROOT)
        with tarfile.open(fileobj=io.BytesIO(archive)) as tracked:
            tracked.extractall(snapshot, filter='data')
        if (snapshot / 'web/package-lock.json').read_bytes() != (ROOT / 'web/package-lock.json').read_bytes():
            raise RuntimeError('Installed frontend dependency source differs from archived HEAD')
        (snapshot / 'web/node_modules').symlink_to(ROOT / 'web/node_modules', target_is_directory=True)
        compose = ['docker', 'compose', '-p', project, '-f', str(snapshot / 'docs/verification/fleet-cutover/compose.native.yaml')]
        deployed = False
        try:
            for kind, target in [('PUBLIC', 'public'), ('ADMIN', 'admin'), ('GUIDE', 'styleguide')]:
                print(f'Build normal {target} image from {revision[:12]}', flush=True)
                logged(['docker', 'build', '--target', target, '--label', f'org.opencontainers.image.revision={revision}', '--build-arg', 'VITE_STYLEGUIDE_URL=http://127.0.0.1:18182', '--build-arg', f'STYLEGUIDE_REVISION={revision}', '-t', env[f'FLEET_UI_{kind}_IMAGE'], str(snapshot)], artifacts / f'build-{target}.log', env)
            logged(compose + ['config', '--quiet'], artifacts / 'compose-validation.log', env)
            if any(project_resources(project).values()):
                raise RuntimeError('Private project appeared during build; no deployment/cleanup attempted')
            deployed = True
            print(f'Start isolated {project}; existing user projects are untouched', flush=True)
            logged(compose + ['up', '-d', '--wait', '--wait-timeout', '120', 'public', 'admin', 'styleguide'], artifacts / 'startup.log', env)
            print('Verify real OIDC, native create/review/read, themes/widths and independent failure', flush=True)
            logged(['node', str(snapshot / 'docs/verification/fleet-cutover/native-browser.mjs')], artifacts / 'browser.log', env)
        finally:
            if deployed:
                logged(compose + ['down', '--volumes', '--remove-orphans'], artifacts / 'cleanup.log', env)
                remaining = project_resources(project)
                if any(remaining.values()):
                    raise RuntimeError('Private project cleanup is incomplete: ' + json.dumps(remaining))
                (artifacts / 'cleanup.json').write_text(json.dumps({'project': project, 'remaining': remaining, 'source': revision}, indent=2) + '\n')
    print(f'PASS: normal-image browser/runtime proof and scoped cleanup: {artifacts}', flush=True)


if __name__ == '__main__':
    main()
