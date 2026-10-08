import importlib.machinery
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / 'bin/mega-speak'
loader = importlib.machinery.SourceFileLoader('mega_speak', str(SCRIPT))
spec = importlib.util.spec_from_loader(loader.name, loader)
mega = importlib.util.module_from_spec(spec)
loader.exec_module(mega)


class VoiceSettingsTest(unittest.TestCase):
    def test_selection_preserves_other_config_and_override_order(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            system, user, selected, explicit = [root/n for n in ['system.toml','user.toml','voice.toml','explicit.toml']]
            system.write_text('voice="system"\nvolume=80\n')
            user.write_text('voice="user"\nspeed=1.1\n')
            explicit.write_text('voice="forced"\n')
            original_paths, original_voice = mega.CONFIG_PATHS, mega.VOICE_PATH
            try:
                mega.CONFIG_PATHS=[str(system),str(user),str(selected)]
                mega.VOICE_PATH=str(selected)
                mega.save_voice('ICL_uranus_zh_male_lanyincaohunshi_tob')
                cfg,_,_=mega.load_config()
                self.assertEqual(cfg['voice'],'ICL_uranus_zh_male_lanyincaohunshi_tob')
                self.assertEqual((cfg['volume'],cfg['speed']),(80,1.1))
                self.assertEqual(user.read_text(),'voice="user"\nspeed=1.1\n')
                mega.CONFIG_PATHS.append(str(explicit))
                self.assertEqual(mega.load_config()[0]['voice'],'forced')
            finally:mega.CONFIG_PATHS,mega.VOICE_PATH=original_paths,original_voice

    def test_invalid_id_and_symlink_leave_bytes_untouched(self):
        with tempfile.TemporaryDirectory() as tmp:
            path=Path(tmp)/'voice.toml';target=Path(tmp)/'managed.toml'
            target.write_text('voice="original"\n');path.symlink_to(target)
            old=mega.VOICE_PATH
            try:
                mega.VOICE_PATH=str(path)
                with self.assertRaises(ValueError):mega.save_voice('valid')
                with self.assertRaises(ValueError):mega.save_voice('voice"\napi_key_cmd=[]')
                self.assertEqual(target.read_text(),'voice="original"\n')
            finally:mega.VOICE_PATH=old

    def test_cli_state_does_not_read_speech_or_synthesize(self):
        with tempfile.TemporaryDirectory() as tmp:
            env=dict(os.environ, MEGA_SPEAK_VOICE_CONFIG=str(Path(tmp)/'voice.toml'), MEGA_SPEAK_CONFIG='')
            result=subprocess.run(['python3',str(SCRIPT),'--set-voice','trial'],input='',text=True,capture_output=True,env=env,timeout=10)
            self.assertEqual(result.returncode,0,result.stderr)
            self.assertEqual(json.loads(result.stdout)['voice'],'trial')
            state=subprocess.run(['python3',str(SCRIPT),'--voice-state'],input='',text=True,capture_output=True,env=env,timeout=10)
            self.assertEqual(json.loads(state.stdout)['voice'],'trial')
            self.assertEqual(state.stderr,'')

if __name__=='__main__':unittest.main()
