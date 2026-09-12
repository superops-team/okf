# Traceability Matrix — Usability → Spec → Code → Test → Entry Point

| UX Case | Spec | Code Path | Automated Test | Real Entry Point |
|---|---|---|---|---|
| A1 command discoverability | S50 | cmd/okf/main.go usage | help_golden_test.go | `okf help` |
| A2 empty KB | S17/S18 | pkg/manifest, pkg/identity | manifest_test.go | `okf identity ensure`, `okf tool manifest` |
| A3 no index degradation | S15/S27 | pkg/query/semantic.go, cmd_eval.go | semantic_test.go | `okf search`, `okf eval -compare` |
| A4 v2 index remediation | S15 | pkg/vectorindex, errToTool | identity_v3_test.go | `okf search`, `okf vector status` |
| A5 dry-run→apply→resolve | S06/S07/S08/S11 | pkg/identity/migrate.go, persist.go | migrate_test.go, identity_test.go | `okf identity ensure/resolve` |
| A6 rename/move resolve | S11/S14 | pkg/identity/identity.go (resolver) | identity_test.go | `okf identity resolve` |
| A7 malformed ref | S03/S12 | pkg/identity/errors.go, identity.go | identity_test.go | `okf identity resolve` |
| A8 duplicate ID | S04 | pkg/identity/migrate.go (summarize) | migrate_test.go | `okf identity ensure --apply` |
| B1 default output | S17/S19/S20 | pkg/manifest/manifest.go, cmd_tool.go | manifest_test.go | `okf tool manifest` |
| B2 JSON output | S18 | cmd_tool.go, Service.Manifest | manifest_test.go | `okf tool manifest --json` |
| B3 pagination | S18/S21 | pkg/manifest/manifest.go (filters) | manifest_test.go | `okf tool manifest --limit` |
| B4 filter combinations | S19/S20 | pkg/manifest/manifest.go (Filter) | manifest_test.go | `okf tool manifest --type` |
| B5 corrupt frontmatter | S23 | pkg/manifest/reader.go | reader_test.go | `okf tool manifest` |
| B6 oversized frontmatter | S22/S24 | pkg/manifest/reader.go (bounded) | benchmark_evidence_test.go | `okf tool manifest` |
| B7 duplicate IDs in manifest | S04/S25 | pkg/manifest/manifest.go | manifest_test.go | `okf tool manifest` |
| B8 no implicit index/model | S24 | pkg/manifest (no vectorindex import) | reader_test.go | `okf tool manifest` |
| B9 CLI/Service/MCP consistency | S26 | pkg/tool/service.go, pkg/mcp/tools.go | tools_agent_test.go, manifest_test.go | CLI + MCP |
| C1 omitted=legacy | S27 | pkg/tool/service.go (GroupBy=="") | grouping_cli_test.go | `okf search` |
| C2 grouping semantics | S28/S29/S30 | pkg/query/project.go | project_test.go | `okf search -group-by` |
| C3 fewer than K | S31 | pkg/query/project.go (limit) | project_test.go | `okf search -group-by` |
| C4 invalid group_by | S33 | pkg/query/project.go (ErrInvalidGroupBy) | project_test.go | CLI + MCP |
| C5 no index degradation | S27/S34 | cmd_eval.go, pkg/query/semantic.go | grouping_cli_test.go | `okf search -group-by` |
| C6 cross-entry consistency | S34 | pkg/tool/service.go, pkg/mcp/tools.go | tools_agent_test.go | CLI + MCP |
| C7 hybrid utility gate | S46/S47 | pkg/eval/benchmark.go, metrics.go | hybrid_baseline_test.go, grouped_metrics_test.go | `okf eval -group-by` |
| D1 lifecycle | S35-S40 | pkg/agentconfig/* | service_extra_test.go, workflow_test.go | `okf agent plan/apply/status/remove` |
| D2 all client | S37 | pkg/agentconfig/service.go | service_test.go | `okf agent apply --client all` |
| D3 unowned config conflict | S40/S41 | pkg/agentconfig/files.go, service.go | service_test.go | `okf agent apply` |
| D4 unknown keys preserved | S38 | pkg/agentconfig/json.go, blocks.go | json_test.go | `okf agent apply` |
| D5 unbalanced markers | S41 | pkg/agentconfig/blocks.go | blocks_test.go | `okf agent apply` |
| D6 read-only file | S44 | pkg/agentconfig/files.go (atomic write) | files_test.go | `okf agent apply` |
| D7 symlink/path escape | S43 | pkg/agentconfig/files.go (safeJoin) | files_test.go | `okf agent apply` |
| D8 partial write rollback | S44 | pkg/agentconfig/service.go (restorePoint) | service_test.go | `okf agent apply` |
| D9 generated config starts MCP | S36/S39 | pkg/agentconfig/workflow.go, cmd/mcp | workflow_test.go | `okf mcp --repo .` |
| D10 canonical workflow guidance | S39 | pkg/agentconfig/* (comments) | workflow_test.go | generated config read |
| D11 interactive/non-interactive | S35 | cmd/okf/cmd_agent.go | cmd_agent_test.go | `okf agent apply` |
| E1 legacy bundle works | S05/S27 | pkg/identity (no ID = valid) | identity_test.go | all commands |
| E2 writer preserves ID | S13 | pkg/identity/persist.go, pkg/git/generator.go | identity_test.go | `okf add`, codegen |
| E3 derived chunks parent ID | S14 | pkg/convert/convert.go (WrapChunkConcept) | identity_test.go | `okf add` (large doc) |
| E4 vector v2 remediation | S15 | pkg/vectorindex, cmd_vector.go | identity_v3_test.go | `okf vector rebuild` |
| E5 concept_id vs okf_id | S01/S02 | pkg/identity/identity.go, docs | identity_test.go | frontmatter, API |
| F1 error rubric | S49 | all error paths | various | all commands |
| F2 exit codes | S49 | cmd/okf/* | cmd tests | all commands |
| F3 JSON clean streams | S18/S26 | cmd/okf/*, pkg/mcp | cmd tests, test_mcp.py | `--json` commands |
| F4 help/README examples | S50 | cmd/okf/main.go, README | help_golden_test.go | help, README |
| F5 output verbosity | S49 | cmd/okf/* | cmd tests | all commands |
| G1 MCP startup | S26/S39 | cmd/okf/cmd_mcp.go, pkg/mcp | test_mcp.py | `okf mcp --repo .` |
| G2 actual tool calls | S26/S34 | pkg/mcp/tools.go | test_mcp.py, tools_agent_test.go | MCP stdio |
| G3 MCP error handling | S33/S12 | pkg/mcp/tools.go, errToTool | tools_agent_test.go | MCP stdio |
| G4 durable note capture | S16 | pkg/tool/write.go, pkg/identity/persist.go | identity_resolve_test.go | MCP okf_note |
