#!/usr/bin/env python3
"""Modern MCP 2026-07-28 + Skills extension E2E test.

Tests the dual-era server's modern protocol surface:
- server/discover with per-request _meta
- skills/list, skills/get with capability gate
- resources/list, resources/read for the static Skill
- tools/list (exactly 12 modern tools), tools/call (read-only)
- Negative: unsupported version (-32022), missing capability (-32021), unknown URI (-32602)

Uses newline-delimited JSON (normative modern stdio framing).
"""
import json
import subprocess
import sys
import os

REPO = os.path.dirname(os.path.abspath(__file__))
OKF_BIN = os.environ.get("OKF_BIN", os.path.join(REPO, "okf"))

def modern_meta(skills_cap=False):
    caps = {}
    if skills_cap:
        caps["extensions"] = {"io.modelcontextprotocol/skills": {}}
    return {
        "_meta": {
            "io.modelcontextprotocol/protocolVersion": "2026-07-28",
            "io.modelcontextprotocol/clientCapabilities": caps,
        }
    }

class MCPClient:
    def __init__(self, repo):
        self.proc = subprocess.Popen(
            [OKF_BIN, "mcp", "--repo", repo],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            bufsize=1,
        )
        self.id = 0

    def call(self, method, params=None):
        self.id += 1
        req = {"jsonrpc": "2.0", "id": self.id, "method": method}
        if params:
            req["params"] = params
        self.proc.stdin.write(json.dumps(req) + "\n")
        self.proc.stdin.flush()
        line = self.proc.stdout.readline()
        return json.loads(line)

    def close(self):
        self.proc.stdin.close()
        self.proc.wait(timeout=5)

def test_discover():
    client = MCPClient(REPO)
    resp = client.call("server/discover", modern_meta())
    assert "error" not in resp, f"discover error: {resp}"
    result = resp["result"]
    assert result["resultType"] == "complete"
    assert "2026-07-28" in result["supportedVersions"]
    assert "2024-11-05" not in result["supportedVersions"]
    caps = result["capabilities"]
    assert "tools" in caps
    assert "resources" in caps
    assert "extensions" in caps
    assert "io.modelcontextprotocol/skills" in caps["extensions"]
    assert "prompts" not in caps
    assert "_meta" in result
    assert "io.modelcontextprotocol/serverInfo" in result["_meta"]
    print("PASS: server/discover")
    client.close()

def test_unsupported_version():
    client = MCPClient(REPO)
    params = {"_meta": {
        "io.modelcontextprotocol/protocolVersion": "2024-11-05",
        "io.modelcontextprotocol/clientCapabilities": {},
    }}
    resp = client.call("server/discover", params)
    assert "error" in resp
    assert resp["error"]["code"] == -32022
    print("PASS: unsupported version -32022")
    client.close()

def test_skills_list_requires_capability():
    client = MCPClient(REPO)
    resp = client.call("skills/list", modern_meta(skills_cap=False))
    assert "error" in resp
    assert resp["error"]["code"] == -32021
    print("PASS: skills/list requires capability -32021")
    client.close()

def test_skills_list_get():
    client = MCPClient(REPO)
    resp = client.call("skills/list", modern_meta(skills_cap=True))
    assert "error" not in resp, f"skills/list error: {resp}"
    result = resp["result"]
    assert result["resultType"] == "complete"
    assert len(result["skills"]) == 1
    skill = result["skills"][0]
    assert skill["uri"] == "skill://okf/SKILL.md"
    assert skill["name"] == "okf"

    # skills/get must deep-equal list entry
    resp2 = client.call("skills/get", {**modern_meta(skills_cap=True), "uri": skill["uri"]})
    assert "error" not in resp2
    assert resp2["result"]["skill"] == skill
    print("PASS: skills/list + skills/get")
    client.close()

def test_resources():
    client = MCPClient(REPO)
    resp = client.call("resources/list", modern_meta())
    assert "error" not in resp
    resources = resp["result"]["resources"]
    assert len(resources) == 1
    assert resources[0]["uri"] == "skill://okf/SKILL.md"
    assert resources[0]["mimeType"] == "text/markdown"

    # Read must match
    resp2 = client.call("resources/read", {**modern_meta(), "uri": resources[0]["uri"]})
    assert "error" not in resp2
    content = resp2["result"]["contents"][0]
    assert content["uri"] == resources[0]["uri"]
    assert "W01" in content["text"]
    assert "W07" in content["text"]
    print("PASS: resources/list + resources/read")
    client.close()

def test_tools_modern():
    client = MCPClient(REPO)
    resp = client.call("tools/list", modern_meta())
    assert "error" not in resp
    tools = resp["result"]["tools"]
    assert len(tools) == 12, f"expected 12 modern tools, got {len(tools)}"
    names = [t["name"] for t in tools]
    assert names == sorted(names), "tools must be sorted"
    # Legacy-only tools must not appear
    for legacy in ["okf_load_bundle", "okf_search", "okf_semantic_search"]:
        assert legacy not in names, f"legacy tool {legacy} should not be in modern catalog"
    print("PASS: tools/list (12 modern tools)")
    client.close()

def test_unknown_skill_uri():
    client = MCPClient(REPO)
    resp = client.call("skills/get", {**modern_meta(skills_cap=True), "uri": "skill://okf/other.md"})
    assert "error" in resp
    assert resp["error"]["code"] == -32602
    print("PASS: unknown skill URI -32602")
    client.close()

def test_modern_prompts_not_implemented():
    client = MCPClient(REPO)
    resp = client.call("prompts/list", modern_meta())
    assert "error" in resp
    assert resp["error"]["code"] == -32601
    print("PASS: modern prompts not implemented -32601")
    client.close()

def main():
    if not os.path.exists(OKF_BIN):
        print(f"Building okf binary...", file=sys.stderr)
        subprocess.run(["go", "build", "-o", OKF_BIN, "./cmd/okf"], cwd=REPO, check=True)

    tests = [
        test_discover,
        test_unsupported_version,
        test_skills_list_requires_capability,
        test_skills_list_get,
        test_resources,
        test_tools_modern,
        test_unknown_skill_uri,
        test_modern_prompts_not_implemented,
    ]
    passed = 0
    for test in tests:
        try:
            test()
            passed += 1
        except Exception as e:
            print(f"FAIL: {test.__name__}: {e}", file=sys.stderr)
    print(f"\n{passed}/{len(tests)} tests passed")
    if passed != len(tests):
        sys.exit(1)

if __name__ == "__main__":
    main()
