"""Regression seams for provenance/resource guards; no Docker services are started."""
import contextlib
import importlib.util
import io
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parent.parent
spec = importlib.util.spec_from_file_location('fleet_runtime', ROOT / 'scripts/verify-fleet-runtime.py')
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


class RuntimeGuards(unittest.TestCase):
    def output(self, args, **kwargs):
        if args[0] == 'docker':
            if args[:4] == ['docker', 'volume', 'ls', '-q']:
                return self.volume
            if args[:4] == ['docker', 'network', 'ls', '-q']:
                return self.network
            return ''
        return subprocess.check_output(args, cwd=ROOT, text=True, **kwargs).strip()

    def setUp(self):
        self.volume = self.network = ''
        self.directory = tempfile.TemporaryDirectory(prefix='fleet-runtime-guard-')
        self.addCleanup(self.directory.cleanup)
        self.artifacts = Path(self.directory.name) / 'artifacts'
        self.addCleanup(patch.stopall)
        patch.object(sys, 'argv', ['verify-fleet-runtime.py', '--artifacts', str(self.artifacts)]).start()
        patch.object(runner, 'output', side_effect=self.output).start()
        # Port availability is tested by the native probe, not these two guards.
        patch.object(runner.socket, 'socket').start()

    def test_residual_volume_or_network_stops_before_build_and_cleanup(self):
        for resource in ['volume', 'network']:
            with self.subTest(resource=resource):
                self.volume = self.network = ''
                setattr(self, resource, 'previous-run-resource')
                errors = io.StringIO()
                with patch.object(runner, 'logged') as external, contextlib.redirect_stderr(errors):
                    with self.assertRaises(SystemExit) as stopped:
                        runner.main()
                    self.assertEqual(stopped.exception.code, 2)
                    self.assertIn('Private project resources already exist', errors.getvalue())
                    external.assert_not_called()
                    self.assertFalse(self.artifacts.exists())

    def test_live_fixture_edits_cannot_change_archived_execution_or_bind_mount(self):
        paths = [ROOT / 'docs/verification/fleet-cutover' / name for name in ['compose.native.yaml', 'native-browser.mjs', 'oidc-provider.mjs']]
        original = {path: path.read_bytes() for path in paths}
        changed = {}
        observed = set()
        snapshot = None

        def boundary(args, log, env):
            nonlocal snapshot
            if args[:2] == ['docker', 'build']:
                snapshot = Path(args[-1])
                if not changed:
                    for path, value in original.items():
                        changed[path] = value + b'\n# synthetic concurrent live edit\n'
                        path.write_bytes(changed[path])
                observed.add('build')
            if args[0] == 'node':
                self.assertEqual(Path(args[1]), snapshot / paths[1].relative_to(ROOT))
                self.assertEqual(env['FLEET_UI_SOURCE_REVISION'], subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip())
                observed.add('browser')
            if 'compose' in args:
                self.assertEqual(Path(args[args.index('-f') + 1]), snapshot / paths[0].relative_to(ROOT))
                if 'down' in args:
                    observed.add('cleanup')
            if snapshot is not None:
                for path, value in original.items():
                    self.assertEqual((snapshot / path.relative_to(ROOT)).read_bytes(), value)
            log.write_text('Stubbed external tool boundary; no image/app/runtime proof.\n')

        try:
            with patch.object(runner, 'logged', side_effect=boundary), contextlib.redirect_stdout(io.StringIO()):
                runner.main()
            self.assertEqual(observed, {'build', 'browser', 'cleanup'})
            self.assertIsNotNone(snapshot)
            self.assertFalse(snapshot.exists())
        finally:
            for path, value in changed.items():
                if path.read_bytes() != value:
                    raise AssertionError('Unexpected concurrent edit; refusing to overwrite it')
                path.write_bytes(original[path])


if __name__ == '__main__':
    unittest.main()
