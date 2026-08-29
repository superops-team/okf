# Golden files

These files lock `pkg/convert` conversion output to **downmark v0.10.0**.

Regenerate ONLY after an intentional downmark upgrade:
```
UPDATE_GOLDEN=1 go test ./pkg/convert -run TestConvertGolden
```
Then review the git diff manually. Version drift on a pinned dependency
indicates a behavior change and must be reported in Release Notes.
