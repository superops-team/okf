# Conformance Audit — add-okf-project-gate

> Status: local implementation complete. This audit maps each Scenario in
> `specs/okf-project-gate/spec.md` to implementation paths and executable local
> verification. Full heavyweight gauntlet status is reported separately because
> the project gate is an L0 structural gate, not a replacement for build/test
> admission.

## Audit metadata

- Change ID: `add-okf-project-gate`
- Verification target: current working tree
- Primary entry point: `tools/okf-project-gate.sh`
- Self-verification: `tools/verify-okf-project-gate.sh`
- Heavy gate integration: `tools/gauntlet.sh`

## Scenario mapping

| Scenario | Requirement | Implementation path/symbol | Automated test / evidence | Fresh command / result | Align |
|---|---|---|---|---|---|
| S01 | Go and downmark constraints are pinned | `tools/okf-project-gate.sh` `check_core_invariants` | Current repo gate + downmark negative control | `tools/okf-project-gate.sh --change-id add-okf-project-gate`; `tools/verify-okf-project-gate.sh` | fully |
| S02 | Gauntlet critical layers remain present | `tools/okf-project-gate.sh` `check_core_invariants` | Current repo gate checks `tools/gauntlet.sh` strings | `tools/okf-project-gate.sh --change-id add-okf-project-gate` | fully |
| S03 | Core gate is read-only | `tools/okf-project-gate.sh` core path; no repository writes | Script inspection + current repo gate | `tools/okf-project-gate.sh` | fully |
| S04 | Explicit change has required artifacts | `tools/okf-project-gate.sh` `check_change` | Current change gate + missing-spec negative control | `tools/okf-project-gate.sh --change-id add-okf-project-gate`; `tools/verify-okf-project-gate.sh` | fully |
| S05 | Spec uses Requirement and Scenario structure | `tools/okf-project-gate.sh` `check_change` | Current change gate + missing-Scenario negative control | `tools/verify-okf-project-gate.sh` | fully |
| S06 | Tasks express full dependency boundary | `tools/okf-project-gate.sh` `check_change` | Current change gate validates completion contract and P0-P4 | `tools/okf-project-gate.sh --change-id add-okf-project-gate` | fully |
| S07 | Conformance maps scenarios to verification | `tools/okf-project-gate.sh` `check_change` | Current change gate validates scenario, verification, and status words | `tools/okf-project-gate.sh --change-id add-okf-project-gate` | fully |
| S08 | Current change can be inferred from git status | `tools/okf-project-gate.sh` `infer_changed_ids` | Untracked/modified current change is inferred when no change ID is supplied | `tools/okf-project-gate.sh` | fully |
| S09 | Valid fixture passes | `tools/verify-okf-project-gate.sh` fixture setup | Positive fixture run | `tools/verify-okf-project-gate.sh` | fully |
| S10 | Version drift negative control fails | `tools/verify-okf-project-gate.sh` downmark mutation | Negative control expects non-zero | `tools/verify-okf-project-gate.sh` | fully |
| S11 | Missing spec negative control fails | `tools/verify-okf-project-gate.sh` spec removal | Negative control expects non-zero | `tools/verify-okf-project-gate.sh` | fully |
| S12 | Missing Scenario negative control fails | `tools/verify-okf-project-gate.sh` Scenario removal | Negative control expects non-zero | `tools/verify-okf-project-gate.sh` | fully |
| S13 | Gauntlet runs project gate as L0 | `tools/gauntlet.sh` L0 step | Script syntax + file-level gate check | `bash -n tools/gauntlet.sh`; full `tools/gauntlet.sh` remains the heavyweight final gate | fully |

## Verification log

| Command | Result |
|---|---|
| `bash -n tools/okf-project-gate.sh tools/verify-okf-project-gate.sh tools/gauntlet.sh` | PASS |
| `tools/okf-project-gate.sh --change-id add-okf-project-gate` | PASS |
| `tools/okf-project-gate.sh` | PASS; inferred `add-okf-project-gate` from current git status |
| `tools/verify-okf-project-gate.sh` | PASS; valid fixture accepted, downmark drift/missing spec/missing Scenario negative controls rejected |
| `tools/gauntlet.sh` | PASS; project-gate/build/vet/staticcheck/tests/tests(-race)/coverage(72%)/shuffle/new-package/property(20)/secret-scan/mod-verify/mutation/agent-mutation/governed-mutation/temporal-mutation/real-exec |

## Allowed alignment values

- `fully`: exact behavior is implemented and verified by the cited command.
- `aligned`: behavior is met through an equivalent existing implementation.
- `partial`: some acceptance clauses are unmet; reason and blocking task required.
- `gap`: no verified implementation; blocking task required.

## Final gates

- [x] S01-S13 rows exist with no duplicates.
- [x] Every row names an implementation path and executable verification.
- [x] L0 project gate does not replace heavyweight gauntlet.
- [x] Fresh local verification commands have been run and this file updated from pending to observed results.
