"""Integration tests use local TLS and HTTP-CONNECT fixtures; no subscription required."""
import base64, http.server, json, os, pathlib, select, socket, socketserver, ssl, subprocess, tempfile, threading, time, unittest
ROOT=pathlib.Path(__file__).resolve().parents[1]
WORKER=ROOT/'bin/node-worker'
def run(payload,env=None):
    p=subprocess.run([str(WORKER)],input=json.dumps(payload),text=True,capture_output=True,timeout=25,env=env)
    lines=[x[7:] for x in p.stdout.splitlines() if x.startswith('RESULT:')]
    assert lines,p.stderr
    return json.loads(lines[-1])
class Site(http.server.BaseHTTPRequestHandler):
    calls=0
    def log_message(self,*a):pass
    def do_GET(self):
        Site.calls+=1
        n=int(self.path.split('bytes=')[-1]);self.send_response(200);self.send_header('Content-Length',str(n));self.end_headers()
        try:
            while n:
                b=min(n,16384);self.wfile.write(b'x'*b);self.wfile.flush();n-=b;time.sleep(.01)
        except (BrokenPipeError,ConnectionResetError):pass
class Tunnel(socketserver.StreamRequestHandler):
    calls=0
    def handle(self):
        request=self.rfile.readline().decode();Tunnel.calls+=1
        while self.rfile.readline().strip():pass
        _,target,_=request.split();host,port=target.rsplit(':',1)
        assert host=='127.0.0.1'
        with socket.create_connection((host,int(port))) as other:
            self.wfile.write(b'HTTP/1.1 200 Connection Established\r\n\r\n');self.wfile.flush()
            while True:
                ready,_,_=select.select([self.connection,other],[],[],5)
                if not ready:return
                for src in ready:
                    try:data=src.recv(65536)
                    except ConnectionResetError:return
                    if not data:return
                    (other if src is self.connection else self.connection).sendall(data)
class Server(socketserver.ThreadingTCPServer):allow_reuse_address=True;daemon_threads=True
class WorkerTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temp=tempfile.TemporaryDirectory();p=pathlib.Path(cls.temp.name)
        subprocess.run(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-days','1','-keyout',str(p/'key.pem'),'-out',str(p/'cert.pem'),'-subj','/CN=localhost','-addext','subjectAltName=IP:127.0.0.1'],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        cls.site=http.server.ThreadingHTTPServer(('127.0.0.1',0),Site)
        ctx=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER);ctx.load_cert_chain(p/'cert.pem',p/'key.pem');# Local HTTP target keeps the test independent of macOS trust-store changes.
        cls.proxy=Server(('127.0.0.1',0),Tunnel)
        for s in [cls.site,cls.proxy]:threading.Thread(target=s.serve_forever,daemon=True).start()
        cls.env={**os.environ,'SSL_CERT_FILE':str(p/'cert.pem')}
    @classmethod
    def tearDownClass(cls):cls.site.shutdown();cls.proxy.shutdown();cls.temp.cleanup()
    def test_end_to_end_download_via_proxy(self):
        row=run({'action':'test','node':{'name':'fixture','type':'http','server':'127.0.0.1','port':self.proxy.server_address[1]},'mode':'speed','seconds':3,'megabytes':1,'timeout':2,'ip_info':False,'server':f'http://127.0.0.1:{self.site.server_address[1]}'},self.env)
        self.assertEqual(row['status'],'complete',row)
        self.assertEqual(row['bytes'],1_000_000)
        self.assertGreater(row['average_MBps'],0)
        self.assertIsNone(row['tls_ms'])
        self.assertEqual(row['probe_failure_pct'],0)
        self.assertGreaterEqual(Tunnel.calls,4)
        self.assertGreater(len(row['samples']),0)
        # Result remains in test memory.
    def test_unreachable_is_failure_not_unlock(self):
        row=run({'node':{'type':'socks5','server':'127.0.0.1','port':9,'name':'dead','password':'test-secret-never-export'},'mode':'both','platforms':['Disney+','Netflix','YouTube','OpenAI','Gemini','Spotify'],'timeout':1,'ip_info':False})
        self.assertEqual(row['status'],'failed')
        self.assertIsNone(row['average_MBps']);self.assertIsNone(row['https_ms'])
        self.assertEqual(len(row['services']),6)
        self.assertTrue(all(v['status']=='error' for v in row['services'].values()))
        self.assertNotIn('test-secret',json.dumps(row))
    def test_yaml_duplicate_names_are_preserved(self):
        content='proxies:\n- {name: same, type: socks5, server: 127.0.0.1, port: 9}\n- {name: same, type: socks5, server: 127.0.0.1, port: 10}\n'
        self.assertEqual(len(run({'action':'parse','content':content})['nodes']),2)
        self.assertEqual(len(run({'action':'parse','content':base64.b64encode(content.encode()).decode()})['nodes']),2)
    def test_uri_subscription_and_bad_input(self):
        content='trojan://fake-password@127.0.0.1:443#Example'
        r=run({'action':'parse','content':base64.b64encode(content.encode()).decode()})
        self.assertEqual(r['nodes'][0]['type'],'trojan')
        self.assertIn('error',run({'action':'parse','content':'<html>Bad response</html>'}))
    def test_invalid_node_is_skipped(self):
        r=run({'node':{'name':'bad','type':'made-up'},'mode':'speed'})
        self.assertEqual(r['status'],'failed');self.assertIn('配置',r['error'])
if __name__=='__main__':unittest.main(verbosity=2)
