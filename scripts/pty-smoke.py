#!/usr/bin/env python3
"""Exercise only the separate fixture; all install effects use a temporary home."""
import argparse
import errno
import fcntl
import json
import os
from pathlib import Path
import re
import select
import signal
import struct
import subprocess
import tempfile
import termios
import time

ANSI = re.compile(rb"\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b\[[0-?]*[ -/]*[@-~]|\x1b[()][A-Z0-9]|\x1b[=>]")


class Terminal:
    def __init__(self, fixture, args, dark):
        self.master, self.slave = os.openpty()
        self.before = termios.tcgetattr(self.slave)
        self.raw = bytearray()
        self.dark = dark
        self.status = None
        self.resize(80, 24, notify=False)
        self.pid = os.fork()
        if self.pid == 0:
            os.close(self.master)
            os.setsid()
            fcntl.ioctl(self.slave, termios.TIOCSCTTY, 0)
            for fd in (0, 1, 2):
                os.dup2(self.slave, fd)
            if self.slave > 2:
                os.close(self.slave)
            env = dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor")
            env.pop("NO_COLOR", None)
            os.execve(str(fixture), [str(fixture), *args], env)

    def resize(self, width, height, notify=True):
        fcntl.ioctl(self.slave, termios.TIOCSWINSZ, struct.pack("HHHH", height, width, 0, 0))
        if notify:
            os.kill(self.pid, signal.SIGWINCH)

    def pump(self, duration=0.15):
        end = time.monotonic() + duration
        while time.monotonic() < end:
            ready, _, _ = select.select([self.master], [], [], min(0.05, max(0, end-time.monotonic())))
            if ready:
                try:
                    chunk = os.read(self.master, 65536)
                except OSError as e:
                    if e.errno == errno.EIO:
                        return
                    raise
                if not chunk:
                    return
                old = bytes(self.raw[-32:])
                self.raw.extend(chunk)
                if b"\x1b]11;?" in old+chunk:
                    color = b"0000/0000/0000" if self.dark else b"ffff/ffff/ffff"
                    os.write(self.master, b"\x1b]11;rgb:"+color+b"\x1b\\")

    def wait(self, text, timeout=12):
        marker = text.encode()
        end = time.monotonic()+timeout
        while time.monotonic() < end:
            self.pump()
            if marker in ANSI.sub(b"", bytes(self.raw)):
                return
        raise AssertionError(f"missing {text!r}: {ANSI.sub(b'',bytes(self.raw))[-2500:].decode(errors='replace')}")

    def key(self, value):
        os.write(self.master, value)
        self.pump(0.25)

    def finish(self, expected):
        end = time.monotonic()+12
        while time.monotonic() < end:
            self.pump()
            pid, status = os.waitpid(self.pid, os.WNOHANG)
            if pid:
                self.status = status
                break
        assert self.status is not None, "fixture did not exit"
        assert os.waitstatus_to_exitcode(self.status) == expected, bytes(self.raw)[-2000:]
        # macOS detaches the slave when its session leader exits. The fixture
        # compares attributes immediately after Run returns, before detachment.
        assert b"\x1b[?1049l" in self.raw, "alternate screen did not close"

    def close(self):
        if self.status is None:
            os.kill(self.pid, signal.SIGKILL)
            os.waitpid(self.pid, 0)
        os.close(self.master)
        os.close(self.slave)


def preview(t, go=False, hidden=False):
    t.wait("Migration first")
    t.key(b"\r")
    t.wait("Applications")
    if hidden:
        for width,height in ((120,40),(48,18),(40,12),(20,6),(1,1),(80,24)):
            t.resize(width,height)
            t.pump()
        t.resize(40,12)
        t.wait("Enlarge the terminal")
        t.key(b" \r")
        t.resize(80,24)
        t.pump()
    t.key(b"\r")
    t.wait("Select each plugin")
    t.key(b"\r")
    t.wait("Optional lang")
    if go:
        t.key(b" ")
    t.key(b"\r")
    for _ in range(4):
        t.key(b"\r")
    if go:
        t.wait("workspace (optional")
        t.key(b"\r")
        t.wait("Go module path")
        t.key(b"q")
        t.resize(40,12)
        t.pump()
        t.key(b"text\x1b[200~paste\x1b[201~\r")
        t.resize(80,24)
        t.pump()
        t.key(b"\r")
        t.key(b"n")
    t.wait("Previous home folder")
    t.key(b"\r")
    t.wait("Previous app list")
    t.key(b"\r")
    t.wait("Review the current plan")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--fixture", type=Path, required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    fixture = args.fixture.resolve()
    assert fixture.name == "golden-setup-fixture", "this script accepts only the fixture executable"
    checks = []
    with tempfile.TemporaryDirectory(prefix="golden-setup-smoke-") as tmp:
        for dark in (True,False):
            theme = "dark" if dark else "light"
            for mode in ("finish","q-edit","handoff","handoff-cancel","handoff-failure","cancel-apply","escape","ctrl-c"):
                result = Path(tmp)/f"{theme}-{mode}.json"
                options = ["--result", str(result)]
                if mode.startswith("handoff"): options += ["--handoff"]
                if mode == "cancel-apply": options += ["--slow"]
                t = Terminal(fixture, options, dark)
                try:
                    if mode in ("escape","ctrl-c"):
                        t.wait("Migration first"); t.key(b"\r"); t.wait("Applications")
                        t.key(b"\x1b" if mode=="escape" else b"\x03")
                        t.finish(1)
                    else:
                        preview(t,go=mode=="q-edit",hidden=mode=="finish")
                        t.key(b"a")
                        if mode.startswith("handoff"):
                            t.wait("FOREGROUND INPUT:")
                            t.key(b"\x03" if mode=="handoff-cancel" else (b"wrong\n" if mode=="handoff-failure" else b"owned\n"))
                            if mode=="handoff": t.wait("FOREGROUND OK")
                        if mode=="cancel-apply":
                            t.wait("Fixture waits"); t.key(b"\x03")
                        if mode!="handoff-cancel":
                            t.wait("Setup status:")
                            t.key(b"q")
                        t.finish(1 if mode in ("cancel-apply","handoff-cancel","handoff-failure") else 0)
                        saved = json.loads(result.read_text())
                        status = "interrupted" if mode in ("cancel-apply","handoff-cancel") else ("failed" if mode=="handoff-failure" else "complete")
                        assert saved["Report"]["Status"] == status
                        if mode=="q-edit": assert saved["Options"]["Workspaces"][0]["Module"]=="q"
                        if mode=="finish": assert saved["Options"]["Apps"]==["warp","zed","applite"]
                    assert json.loads(result.read_text())["TerminalRestored"], json.loads(result.read_text())
                    checks.append(f"{theme}: {mode}, terminal restored")
                finally:
                    t.close()
        transcript = b"no\nzed\nnone\nnone\nno\nno\nno\nno\nnone\nnone\nno\n"
        for label,data,expected in (("sequential",transcript,0),("early EOF",b"no\nzed\n",1),("final EOF",transcript[:-1],1)):
            run = subprocess.run([str(fixture),"--accessible"],input=data,capture_output=True,timeout=12)
            assert run.returncode==expected,(label,run.stdout,run.stderr)
            assert b"\x1b" not in run.stdout+run.stderr,"plain mode contained ANSI"
            if expected: assert b"incomplete input" in run.stderr
            checks.append(f"plain: {label}")
    for check in checks: print("PASS",check)
    if args.output:
        args.output.write_text(json.dumps({"checks":checks,"passed":len(checks)},indent=2)+"\n")


if __name__ == "__main__":
    main()
