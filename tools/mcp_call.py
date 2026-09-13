#!/usr/bin/env python3
"""MCP stdio tool caller for usability verification.

Usage: python3 mcp_call.py <binary> <repo> <tool_name> [json_args]
Sends initialize + notifications/initialized, then tools/call, prints result JSON to stdout.
Exit code 0 on success, 1 on tool error (isError=true), 2 on protocol failure.
"""
import json
import subprocess
import sys
import time


def send_msg(proc, msg):
    data = json.dumps(msg)
    proc.stdin.write(f"Content-Length: {len(data)}\r\n\r\n{data}")
    proc.stdin.flush()


def recv_msg(proc, timeout=10):
    headers = {}
    while True:
        line = proc.stdout.readline()
        if not line:
            return None
        line = line.strip()
        if not line:
            break
        if ":" in line:
            k, v = line.split(":", 1)
            headers[k.strip().lower()] = v.strip()
    length = int(headers.get("content-length", "0"))
    body = proc.stdout.read(length) if length > 0 else ""
    return json.loads(body) if body else None


def rpc_call(proc, method, params, msg_id=1):
    send_msg(proc, {"jsonrpc": "2.0", "id": msg_id, "method": method, "params": params})
    return recv_msg(proc)


def main():
    if len(sys.argv) < 4:
        print("Usage: mcp_call.py <binary> <repo> <tool_name> [json_args]", file=sys.stderr)
        sys.exit(2)

    binary = sys.argv[1]
    repo = sys.argv[2]
    tool_name = sys.argv[3]
    args = json.loads(sys.argv[4]) if len(sys.argv) > 4 else {}

    proc = subprocess.Popen(
        [binary, "mcp", "--repo", repo],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        text=True,
        bufsize=1,
    )

    try:
        # Initialize
        init = rpc_call(proc, "initialize", {
            "protocolVersion": "2024-11-05",
            "capabilities": {},
            "clientInfo": {"name": "usability-test", "version": "0"},
        }, msg_id=1)
        if init is None or "result" not in init:
            print(json.dumps({"error": "initialize failed", "raw": init}), file=sys.stderr)
            sys.exit(2)

        send_msg(proc, {"jsonrpc": "2.0", "method": "notifications/initialized"})

        # Load bundle if needed (most tools require it)
        if tool_name not in ("okf_load_bundle",):
            rpc_call(proc, "tools/call", {"name": "okf_load_bundle", "arguments": {}}, msg_id=2)

        # Call the requested tool
        result = rpc_call(proc, "tools/call", {
            "name": tool_name,
            "arguments": args,
        }, msg_id=3)

        if result is None:
            print(json.dumps({"error": "no response"}), file=sys.stderr)
            sys.exit(2)

        if "result" in result:
            r = result["result"]
            print(json.dumps(r, indent=2))
            if r.get("isError"):
                sys.exit(1)
            sys.exit(0)
        elif "error" in result:
            print(json.dumps(result["error"], indent=2), file=sys.stderr)
            sys.exit(1)
        else:
            print(json.dumps(result, indent=2), file=sys.stderr)
            sys.exit(2)
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=3)
        except subprocess.TimeoutExpired:
            proc.kill()


if __name__ == "__main__":
    main()
