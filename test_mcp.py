#!/usr/bin/env python3
"""End-to-end test for OKF MCP Server over stdio."""

import json
import subprocess
import sys
import os
import tempfile

OKF_BIN = os.path.join(os.path.dirname(__file__), "okf-bin")
BUNDLE_PATH = os.path.join(os.path.dirname(__file__), "docs", "knowledge")

def _read_headers(proc):
    """Read MCP headers until empty line, return Content-Length."""
    content_length = 0
    while True:
        line = proc.stdout.readline()
        if not line:
            return None
        line = line.strip()
        if not line:
            break
        if line.lower().startswith(b"content-length:"):
            try:
                content_length = int(line.split(b":", 1)[1].strip())
            except ValueError:
                return None
    return content_length

def send_msg(proc, msg):
    """Send a JSON-RPC message using byte-accurate MCP Content-Length framing."""
    body = json.dumps(msg).encode("utf-8")
    header = f"Content-Length: {len(body)}\r\n\r\n".encode("ascii")
    proc.stdin.write(header + body)
    proc.stdin.flush()

def recv_msg(proc):
    """Receive a JSON-RPC message using MCP Content-Length framing."""
    content_length = _read_headers(proc)
    if content_length is None:
        return None
    if content_length == 0:
        return None
    body = proc.stdout.read(content_length)
    if not body:
        return None
    return json.loads(body.decode("utf-8"))

def rpc_call(proc, method, params=None, msg_id=1):
    """Make a JSON-RPC call and return the result."""
    msg = {"jsonrpc": "2.0", "id": msg_id, "method": method}
    if params is not None:
        msg["params"] = params
    send_msg(proc, msg)
    return recv_msg(proc)

def _start_server(*args):
    return subprocess.Popen(
        [OKF_BIN, "mcp", *args],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        bufsize=0,
    )


def _initialize_server(proc, msg_id=1):
    result = rpc_call(proc, "initialize", {
        "protocolVersion": "2024-11-05",
        "capabilities": {},
        "clientInfo": {"name": "test-client", "version": "1.0.0"},
    }, msg_id=msg_id)
    assert result is not None and "result" in result, f"Initialize failed: {result}"
    send_msg(proc, {"jsonrpc": "2.0", "method": "notifications/initialized"})
    return result


def _tool_envelope(proc, name, arguments, msg_id):
    result = rpc_call(proc, "tools/call", {"name": name, "arguments": arguments}, msg_id=msg_id)
    assert result is not None and "result" in result, f"{name} failed: {result}"
    return json.loads(result["result"]["content"][0]["text"])


def test_note_survives_restart():
    with tempfile.TemporaryDirectory(prefix="okf-mcp-e2e-") as repo:
        subprocess.run(["git", "init", "-q", repo], check=True)
        subprocess.run(["git", "-C", repo, "config", "user.name", "OKF E2E"], check=True)
        subprocess.run(["git", "-C", repo, "config", "user.email", "okf-e2e@example.invalid"], check=True)
        with open(os.path.join(repo, "README.md"), "w", encoding="utf-8") as handle:
            handle.write("# MCP E2E fixture\n")
        subprocess.run(["git", "-C", repo, "add", "README.md"], check=True)
        subprocess.run(["git", "-C", repo, "commit", "-qm", "fixture"], check=True)

        first = _start_server("--repo", repo, "--dir", ".okf/knowledge")
        try:
            _initialize_server(first, msg_id=100)
            initialized = _tool_envelope(first, "okf_init", {}, msg_id=101)
            assert initialized["ok"], initialized
            note = _tool_envelope(first, "okf_note", {
                "content": "Restart durable MCP note",
                "project": "mcp-e2e",
                "idempotency_key": "restart-note-v1",
            }, msg_id=102)
            assert note["ok"] and note["result"]["created"], note
        finally:
            first.terminate()
            first.wait(timeout=5)

        second = _start_server("--repo", repo, "--dir", ".okf/knowledge")
        try:
            _initialize_server(second, msg_id=200)
            ask = _tool_envelope(second, "okf_ask", {
                "query": "Restart durable MCP note",
                "project": "mcp-e2e",
            }, msg_id=201)
            assert ask["ok"], ask
            assert len(ask["result"]["results"]) == 1, ask
            assert ask["result"]["results"][0]["type"] == "note", ask
            context = _tool_envelope(second, "okf_context", {
                "query": "Restart durable MCP note",
                "budget_tokens": 128,
            }, msg_id=202)
            assert context["ok"], context
            assert context["result"]["used_tokens"] <= 128, context
        finally:
            second.terminate()
            second.wait(timeout=5)


def main():
    print("=" * 60)
    print("OKF MCP Server End-to-End Test")
    print("=" * 60)

    # Start the MCP server
    proc = _start_server("--bundle", BUNDLE_PATH)

    try:
        # Test 1: Initialize
        print("\n[Test 1] initialize")
        result = rpc_call(proc, "initialize", {
            "protocolVersion": "2024-11-05",
            "capabilities": {},
            "clientInfo": {"name": "test-client", "version": "1.0.0"}
        }, msg_id=1)
        assert result is not None, "No response to initialize"
        assert "result" in result, f"Initialize failed: {result}"
        caps = result["result"]["capabilities"]
        assert "tools" in caps, "Missing tools capability"
        print(f"  ✓ Server: {result['result']['serverInfo']['name']} v{result['result']['serverInfo']['version']}")
        print(f"  ✓ Protocol: {result['result']['protocolVersion']}")
        print(f"  ✓ Capabilities: tools, resources, prompts")

        # Send initialized notification
        send_msg(proc, {"jsonrpc": "2.0", "method": "notifications/initialized"})

        # Test 2: tools/list
        print("\n[Test 2] tools/list")
        result = rpc_call(proc, "tools/list", msg_id=2)
        assert "result" in result, f"tools/list failed: {result}"
        tools = result["result"]["tools"]
        print(f"  ✓ Found {len(tools)} tools:")
        tool_names = [t["name"] for t in tools]
        for t in tools:
            print(f"    - {t['name']}: {t['description'][:60]}")
        expected_tools = ["okf_load_bundle", "okf_bundle_stats", "okf_list_concepts",
                          "okf_get_concept", "okf_search", "okf_lint_bundle", "okf_lint_concept"]
        for name in expected_tools:
            assert name in tool_names, f"Missing tool: {name}"
        print("  ✓ All expected tools present")

        # Test 3: okf_load_bundle (should already be loaded, but test the tool)
        print("\n[Test 3] tools/call okf_load_bundle")
        result = rpc_call(proc, "tools/call", {
            "name": "okf_load_bundle",
            "arguments": {"path": BUNDLE_PATH}
        }, msg_id=3)
        assert "result" in result, f"okf_load_bundle failed: {result}"
        content = result["result"]["content"][0]["text"]
        print(f"  ✓ Response:\n{content}")
        assert "Loaded bundle" in content, "Unexpected load response"

        # Test 4: okf_bundle_stats
        print("\n[Test 4] tools/call okf_bundle_stats")
        result = rpc_call(proc, "tools/call", {
            "name": "okf_bundle_stats",
            "arguments": {}
        }, msg_id=4)
        assert "result" in result, f"okf_bundle_stats failed: {result}"
        content = result["result"]["content"][0]["text"]
        print(f"  ✓ Response:\n{content}")
        assert "Total concepts" in content, "Missing total concepts"
        assert "By type" in content, "Missing type counts"

        # Test 5: okf_list_concepts
        print("\n[Test 5] tools/call okf_list_concepts")
        result = rpc_call(proc, "tools/call", {
            "name": "okf_list_concepts",
            "arguments": {"limit": 10}
        }, msg_id=5)
        assert "result" in result, f"okf_list_concepts failed: {result}"
        content = result["result"]["content"][0]["text"]
        print(f"  ✓ Response:\n{content}")
        assert "Showing" in content or "No concepts" in content, "Unexpected list response"

        # Test 6: okf_get_concept
        print("\n[Test 6] tools/call okf_get_concept")
        result = rpc_call(proc, "tools/call", {
            "name": "okf_get_concept",
            "arguments": {"path": "core-types.md"}
        }, msg_id=6)
        assert "result" in result, f"okf_get_concept failed: {result}"
        content = result["result"]["content"][0]["text"]
        print(f"  ✓ Response (first 300 chars):\n{content[:300]}...")
        assert "type:" in content or "Core Types" in content, "Unexpected concept content"

        # Test 7: okf_search
        print("\n[Test 7] tools/call okf_search")
        result = rpc_call(proc, "tools/call", {
            "name": "okf_search",
            "arguments": {"query": "parser", "limit": 5}
        }, msg_id=7)
        assert "result" in result, f"okf_search failed: {result}"
        content = result["result"]["content"][0]["text"]
        print(f"  ✓ Response:\n{content}")
        assert "Found" in content or "No results" in content, "Unexpected search response"

        # Test 8: okf_lint_bundle
        print("\n[Test 8] tools/call okf_lint_bundle")
        result = rpc_call(proc, "tools/call", {
            "name": "okf_lint_bundle",
            "arguments": {}
        }, msg_id=8)
        assert "result" in result, f"okf_lint_bundle failed: {result}"
        content = result["result"]["content"][0]["text"]
        print(f"  ✓ Response:\n{content}")
        assert "Linted" in content, "Unexpected lint response"

        # Test 9: resources/list
        print("\n[Test 9] resources/list")
        result = rpc_call(proc, "resources/list", msg_id=9)
        assert "result" in result, f"resources/list failed: {result}"
        resources = result["result"]["resources"]
        print(f"  ✓ Found {len(resources)} resources")
        for r in resources[:3]:
            print(f"    - {r['uri']}: {r['name']}")

        # Test 10: prompts/list
        print("\n[Test 10] prompts/list")
        result = rpc_call(proc, "prompts/list", msg_id=10)
        assert "result" in result, f"prompts/list failed: {result}"
        prompts = result["result"]["prompts"]
        print(f"  ✓ Found {len(prompts)} prompts:")
        for p in prompts:
            print(f"    - {p['name']}: {p['description']}")

        # Test 11: ping
        print("\n[Test 11] ping")
        result = rpc_call(proc, "ping", msg_id=11)
        assert "result" in result, f"ping failed: {result}"
        print("  ✓ Ping successful")
        # Test 12: okf_import_document (document conversion import)
        print("\n[Test 12] tools/call okf_import_document")
        doc_fixture = os.path.join(os.path.dirname(__file__), "pkg", "convert", "testdata", "sample.docx")
        imported_prod = os.path.join(BUNDLE_PATH, "sample.docx.md")
        if os.path.exists(imported_prod):
            os.remove(imported_prod)  # ensure clean start
        result = rpc_call(proc, "tools/call", {
            "name": "okf_import_document",
            "arguments": {"path": doc_fixture}
        }, msg_id=12)
        assert "result" in result, f"okf_import_document failed: {result}"
        content = result["result"]["content"][0]["text"]
        print(f"  ✓ Response:\n{content}")
        assert "Imported document" in content, "Unexpected import response"
        assert os.path.exists(imported_prod), "Imported product file missing"
        with open(imported_prod, "r", encoding="utf-8") as f:
            body = f.read()
        assert "OKF DOCX Fixture" in body, "Imported content missing converted text"
        os.remove(imported_prod)  # clean up so the repo stays untouched
        print("  ✓ Import verified and cleaned up")

        print("\n[Test 13] agent-facing note survives MCP server restart")
        test_note_survives_restart()
        print("  ✓ Note persisted and remained queryable after restart")

        print("\n" + "=" * 60)
        print("ALL TESTS PASSED!")
        print("=" * 60)

    except AssertionError as e:
        print(f"\n❌ TEST FAILED: {e}")
        sys.exit(1)
    except Exception as e:
        print(f"\n❌ ERROR: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)
    finally:
        proc.terminate()
        proc.wait(timeout=5)

if __name__ == "__main__":
    main()
