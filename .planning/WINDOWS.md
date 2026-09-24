---
schema_version: 1
open_count: 1
waived_count: 0
fixed_count: 0
total_count: 1
last_updated: 2026-09-24T07:12:28.392Z
---

# Broken Windows Ledger

> Cross-phase defect register. With `workflow.windows_enforce` enabled, `/gsd-ship` blocks while `open_count > 0`.
> Waive with `gsd-tools windows waive <id> "<reason>"` (reason required).
> Mark fixed with `gsd-tools windows fixed <id>`.

| id | phase | kind | file | line | description | status | reason | recorded_at | resolved_at |
|----|-------|------|------|------|-------------|--------|--------|-------------|-------------|
| 1 | quick-260924-cad | deviation | .github/workflows/ci.yml |  | vuln job installs govulncheck via floating go install ...@latest (T-quick-cad-SC, accepted); pinning it is a separate ci: change | open |  | 2026-09-24T07:12:28.392Z |  |

````json
[
  {
    "id": 1,
    "kind": "deviation",
    "phase": "quick-260924-cad",
    "file": ".github/workflows/ci.yml",
    "line": null,
    "description": "vuln job installs govulncheck via floating go install ...@latest (T-quick-cad-SC, accepted); pinning it is a separate ci: change",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-24T07:12:28.392Z",
    "resolved_at": null
  }
]
````
