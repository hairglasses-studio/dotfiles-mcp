Key constraints:
- All batch/write tools use dry-run by default; pass `execute: true` to apply
- `bulk_settings` always reports previous state before applying
- `clean_stale` checks for uncommitted/unpushed work before deletion
- `pull_all` detects dirty repos and detached HEAD, skips safely
- Composed "tool-of-tools" exist to reduce multi-step token waste:
  full_sync, fleet_audit, cascade_reload, rice_check, bulk_pipeline,
  bt_discover_and_connect, input_auto_setup_controller, ops_iterate, ops_ship
- 99 tools across 14 categories — use composed tools first before chaining primitives
