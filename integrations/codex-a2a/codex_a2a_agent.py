#!/usr/bin/env python3
import argparse
import json
import os
import shutil
import socket
import subprocess
import threading
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

class TaskStore:
    def __init__(self, state_file: Path, output_dir: Path):
        self._state_file = state_file
        self._output_dir = output_dir
        self._lock = threading.Lock()
        self._tasks = {}
        self._load()

    def _load(self):
        self._state_file.parent.mkdir(parents=True, exist_ok=True)
        self._output_dir.mkdir(parents=True, exist_ok=True)
        if not self._state_file.exists():
            self._tasks = {}
            return
        raw = self._state_file.read_text(encoding="utf-8").strip()
        if not raw:
            self._tasks = {}
            return
        data = json.loads(raw)
        tasks = data.get("tasks", {})
        if isinstance(tasks, dict):
            self._tasks = tasks

    def _persist(self):
        payload = {"tasks": self._tasks, "updated_at": utc_now()}
        tmp = self._state_file.with_suffix(".tmp")
        tmp.write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")
        tmp.replace(self._state_file)

    def create(self, prompt: str):
        task_id = str(uuid.uuid4())
        with self._lock:
            self._tasks[task_id] = {
                "id": task_id,
                "status": "working",
                "message": "submitted",
                "artifacts": [],
                "prompt": prompt,
                "created_at": utc_now(),
                "updated_at": utc_now(),
                "output_file": str(self._output_dir / f"{task_id}.txt"),
                "proc_pid": None,
            }
            self._persist()
        return task_id

    def get(self, task_id: str):
        with self._lock:
            item = self._tasks.get(task_id)
            if item is None:
                return None
            return dict(item)

    def set_running(self, task_id: str, pid: int):
        with self._lock:
            item = self._tasks.get(task_id)
            if not item:
                return
            item["proc_pid"] = pid
            item["updated_at"] = utc_now()
            self._persist()

    def finish(self, task_id: str, status: str, message: str, artifacts):
        with self._lock:
            item = self._tasks.get(task_id)
            if not item:
                return
            if item.get("status") == "canceled":
                return
            item["status"] = status
            item["message"] = message
            item["artifacts"] = artifacts
            item["proc_pid"] = None
            item["updated_at"] = utc_now()
            self._persist()

    def cancel(self, task_id: str):
        with self._lock:
            item = self._tasks.get(task_id)
            if not item:
                return None
            item["status"] = "canceled"
            item["message"] = "canceled"
            item["updated_at"] = utc_now()
            self._persist()
            return dict(item)

class ExclusiveThreadingHTTPServer(ThreadingHTTPServer):
    allow_reuse_address = False

    def server_bind(self):
        if hasattr(socket, "SO_EXCLUSIVEADDRUSE"):
            self.socket.setsockopt(socket.SOL_SOCKET, socket.SO_EXCLUSIVEADDRUSE, 1)
        return super().server_bind()

def utc_now():
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())

def parse_user_text(params):
    message = params.get("message")
    if not isinstance(message, dict):
        return ""
    parts = message.get("parts")
    if not isinstance(parts, list):
        return ""
    for part in parts:
        if not isinstance(part, dict):
            continue
        if part.get("type") != "text":
            continue
        text = str(part.get("text", "")).strip()
        if text:
            return text
    return ""

def resolve_codex_bin(explicit_path: str):
    candidates = []
    if explicit_path:
        candidates.append(explicit_path.strip())
    env_path = os.environ.get("CODEX_BIN", "").strip()
    if env_path:
        candidates.append(env_path)
    for name in ("codex", "codex.cmd", "codex.exe"):
        found = shutil.which(name)
        if found:
            candidates.append(found)
    seen = set()
    for candidate in candidates:
        normalized = str(Path(candidate).expanduser())
        if not normalized or normalized in seen:
            continue
        seen.add(normalized)
        if Path(normalized).exists():
            return normalized
    return ""

def task_worker(task_store: TaskStore, task_id: str, prompt: str, workdir: str, codex_bin: str):
    item = task_store.get(task_id)
    if not item:
        return
    output_file = item["output_file"]
    cmd = [codex_bin, "exec", "-C", workdir, "-o", output_file, prompt]
    try:
        proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, encoding="utf-8", errors="replace")
    except FileNotFoundError:
        task_store.finish(task_id, "failed", f"codex cli not found: {codex_bin}", [])
        return
    task_store.set_running(task_id, proc.pid)
    stdout, stderr = proc.communicate()
    if proc.returncode == 0:
        text = ""
        if os.path.exists(output_file):
            text = Path(output_file).read_text(encoding="utf-8", errors="ignore").strip()
        task_store.finish(
            task_id,
            "completed",
            "completed",
            [{"text": text or "(empty output)"}],
        )
        return
    err_text = (stderr or stdout or f"codex exit code {proc.returncode}").strip()
    task_store.finish(task_id, "failed", err_text[:2000], [])

class Handler(BaseHTTPRequestHandler):
    server_version = "codex-a2a/0.1"

    def log_message(self, format, *args):
        return

    def _reply_json(self, code: int, payload):
        raw = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def _rpc_ok(self, request_id, result):
        self._reply_json(200, {"jsonrpc": "2.0", "id": request_id, "result": result})

    def _rpc_err(self, request_id, message):
        self._reply_json(200, {"jsonrpc": "2.0", "id": request_id, "error": {"code": -32000, "message": message}})

    def do_GET(self):
        if self.path != "/.well-known/agent-card.json":
            self._reply_json(404, {"error": "not found"})
            return
        base = f"http://{getattr(self.server, 'host', self.server.server_address[0])}:{getattr(self.server, 'port', self.server.server_address[1])}"
        self._reply_json(200, {
            "name": "codex-local",
            "description": "Local Codex CLI A2A wrapper",
            "url": f"{base}/a2a/rpc",
        })

    def do_POST(self):
        if self.path != "/a2a/rpc":
            self._reply_json(404, {"error": "not found"})
            return
        raw_len = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(raw_len).decode("utf-8", errors="ignore")
        try:
            req = json.loads(body)
        except json.JSONDecodeError:
            self._reply_json(400, {"error": "invalid json"})
            return

        request_id = req.get("id")
        method = str(req.get("method", "")).strip()
        params = req.get("params") or {}

        if method == "message/send":
            prompt = parse_user_text(params)
            if not prompt:
                self._rpc_err(request_id, "empty message text")
                return
            task_id = self.server.task_store.create(prompt)
            worker = threading.Thread(
                target=task_worker,
                args=(self.server.task_store, task_id, prompt, self.server.workdir, self.server.codex_bin),
                daemon=True,
            )
            worker.start()
            self._rpc_ok(request_id, {"task": {"id": task_id, "status": {"state": "working"}, "message": "submitted"}})
            return

        if method == "tasks/get":
            task_id = str(params.get("id", "")).strip()
            task = self.server.task_store.get(task_id)
            if not task:
                self._rpc_err(request_id, "task not found")
                return
            self._rpc_ok(request_id, {
                "task": {
                    "id": task["id"],
                    "status": {"state": task["status"]},
                    "message": task["message"],
                    "artifacts": task.get("artifacts", []),
                }
            })
            return

        if method == "tasks/cancel":
            task_id = str(params.get("id", "")).strip()
            task = self.server.task_store.get(task_id)
            if not task:
                self._rpc_err(request_id, "task not found")
                return
            pid = task.get("proc_pid")
            if isinstance(pid, int) and pid > 0:
                try:
                    os.kill(pid, 9)
                except OSError:
                    pass
            task = self.server.task_store.cancel(task_id)
            self._rpc_ok(request_id, {
                "task": {"id": task["id"], "status": {"state": "canceled"}, "message": task["message"]}
            })
            return

        self._rpc_err(request_id, f"unsupported method: {method}")
def main():
    parser = argparse.ArgumentParser(description="Wrap local Codex CLI as an A2A-compatible agent.")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=9091)
    parser.add_argument("--workdir", required=True)
    parser.add_argument("--state-file", default=str(Path(__file__).parent / "state" / "tasks.json"))
    parser.add_argument("--codex-bin", default="")
    args = parser.parse_args()

    state_file = Path(args.state_file).resolve()
    output_dir = state_file.parent / "output"
    task_store = TaskStore(state_file=state_file, output_dir=output_dir)
    codex_bin = resolve_codex_bin(args.codex_bin)
    if not codex_bin:
        raise SystemExit("codex cli not found. Set --codex-bin or CODEX_BIN, or fix PATH.")

    server = ExclusiveThreadingHTTPServer((args.host, args.port), Handler)
    server.host = args.host
    server.port = args.port
    server.workdir = args.workdir
    server.codex_bin = codex_bin
    server.task_store = task_store
    print(f"codex-a2a listening on http://{args.host}:{args.port} (codex={codex_bin})")
    server.serve_forever()
if __name__ == "__main__":
    main()
