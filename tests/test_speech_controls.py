import importlib.machinery,importlib.util,json,os,subprocess,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
loader=importlib.machinery.SourceFileLoader('controls',str(Path(__file__).resolve().parents[1]/'bin/mega-speak'));spec=importlib.util.spec_from_loader(loader.name,loader);mega=importlib.util.module_from_spec(spec);loader.exec_module(mega)
server_source=(Path(__file__).resolve().parents[1]/'bin/mega-speak-server').read_text();ns={}
with patch('sys.argv',['server','1','token','cli']):exec(server_source[:server_source.index('\n\nThreadingHTTPServer((')],ns)
class SpeechControlsTest(unittest.TestCase):
 def test_single_auxiliary_string_retains_context_and_both_directions(self):
  cfg={**mega.DEFAULTS,'instruction':'开心地说','delivery_instruction':'悄悄话，快一点','context':'用户正在专心工作','section_id':'unique-1','speed':1.25}
  req=mega.synthesis_request(cfg,'CI通过了','pcm')['req_params'];aux=json.loads(req['additions'])
  self.assertEqual(req['text'],'CI通过了');self.assertEqual(len(aux['context_texts']),1)
  for s in ['开心地说','悄悄话，快一点','用户正在专心工作']:self.assertIn(s,aux['context_texts'][0])
  self.assertEqual(aux['section_id'],'unique-1');self.assertEqual(req['audio_params']['speech_rate'],25)
  self.assertNotIn('additions',mega.synthesis_request(mega.DEFAULTS,'text','pcm')['req_params'])
 def test_settings_merge_preserves_voice_and_rejects_invalid_values(self):
  with tempfile.TemporaryDirectory() as tmp:
   path=Path(tmp)/'speech.toml';voice=Path(tmp)/'voice.toml';voice.write_text('voice="myVoice"\n')
   with patch.object(mega,'SETTINGS_PATH',str(path)),patch.object(mega,'CONFIG_PATHS',[str(voice),str(path)]):
    mega.save_speech_settings({'delivery_instruction':'悄悄话','speed':1.1,'volume':80});mega.save_speech_settings({'volume':60})
    cfg=mega.load_config()[0];self.assertEqual((cfg['voice'],cfg['speed'],cfg['volume'],cfg['delivery_instruction']),('myVoice',1.1,60,'悄悄话'))
    before=path.read_bytes()
    for data in [{'speed':float('nan')},{'speed':True},{'volume':101},{'api_key_file':'oops'},{'delivery_instruction':'x'*301}]:
     with self.assertRaises(ValueError):mega.save_speech_settings(data)
     self.assertEqual(path.read_bytes(),before)
 def test_forwarded_http_and_ssh_names_are_validated(self):
  text,flags=ns['flags']({'text':'成功','instruction':'轻声说','context':'用户在开会\n不要打扰','section_id':'session-a'})
  self.assertIn('--context=用户在开会\n不要打扰',flags);self.assertIn('--section-id=session-a',flags)
  for bad in [{'context':'x'*1001},{'section_id':False},{'context':'a\x00b'}]:
   with self.assertRaises(ValueError):ns['flags']({'text':'test',**bad})
 def test_settings_operation_is_silent(self):
  with tempfile.TemporaryDirectory() as tmp:
   env=dict(os.environ,MEGA_SPEAK_SETTINGS_CONFIG=str(Path(tmp)/'speech.toml'),MEGA_SPEAK_CONFIG='')
   proc=subprocess.run(['python3',mega.__file__,'--set-settings',json.dumps({'delivery_instruction':'用悄悄话说','volume':77})],env=env,input='',capture_output=True,text=True,timeout=10)
   self.assertEqual(proc.returncode,0,proc.stderr);self.assertEqual(json.loads(proc.stdout)['volume'],77);self.assertEqual(proc.stderr,'')
if __name__=='__main__':unittest.main()
