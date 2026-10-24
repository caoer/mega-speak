import importlib.machinery
import importlib.util
import json
import os
from pathlib import Path
import stat
import subprocess
import tempfile
import unittest
from unittest.mock import patch

SCRIPT = Path(__file__).resolve().parents[1] / 'bin/mega-speak'
loader = importlib.machinery.SourceFileLoader('mega_speak_key', str(SCRIPT))
spec = importlib.util.spec_from_loader(loader.name, loader)
mega = importlib.util.module_from_spec(spec)
loader.exec_module(mega)

KEY = 'sk-bf-test0123456789wxyz'


class ApiKeyTest(unittest.TestCase):
    def test_set_key_saves_0600_and_state_never_prints_it(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'cfg' / 'api-key'
            env = dict(os.environ, MEGA_SPEAK_KEY_PATH=str(path), MEGA_SPEAK_CONFIG='')
            env.pop('MEGA_SPEAK_API_KEY', None)
            r = subprocess.run(['python3', str(SCRIPT), '--set-key'], input=KEY + '\n', text=True, capture_output=True, env=env, timeout=10)
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertEqual(path.read_text(), KEY + '\n')
            self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)
            state = json.loads(r.stdout)
            self.assertEqual((state['saved'], state['hint']), (True, '…wxyz'))
            self.assertNotIn(KEY, r.stdout + r.stderr)
            bad = subprocess.run(['python3', str(SCRIPT), '--set-key'], input='has space in it\n', text=True, capture_output=True, env=env, timeout=10)
            self.assertNotEqual(bad.returncode, 0)
            self.assertEqual(path.read_text(), KEY + '\n')

    def test_configured_sources_win_over_the_saved_key(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'api-key'
            path.write_text(KEY + '\n')
            cfg = dict(mega.DEFAULTS)
            with patch.object(mega, 'KEY_PATH', str(path)), patch.dict(os.environ, {}, clear=False):
                os.environ.pop('MEGA_SPEAK_API_KEY', None)
                self.assertEqual((mega.key_source(cfg), mega.api_key(cfg)), ('saved', KEY))
                self.assertEqual(mega.key_source({**cfg, 'api_key_cmd': ['echo', 'other']}), 'api_key_cmd')
                self.assertEqual(mega.key_source({**cfg, 'api_key_file': '/x'}), 'api_key_file')
                os.environ['MEGA_SPEAK_API_KEY'] = 'env-key'
                self.assertEqual(mega.key_source({**cfg, 'api_key_cmd': ['echo', 'other']}), 'env')


if __name__ == '__main__':
    unittest.main()
