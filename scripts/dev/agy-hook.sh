#!/usr/bin/env bash
set -euo pipefail

event="${1:-}"
if [[ "$event" != "pre-tool" && "$event" != "post-tool" ]]; then
  echo "usage: agy-hook.sh <pre-tool|post-tool>" >&2
  exit 64
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"
payload="$(cat)"

if [[ "$event" == "post-tool" ]]; then
  printf '{}\n'
  exit 0
fi

tool_name="$(jq -r '.toolCall.name // ""' <<<"$payload")"

if [[ -f "${repo_root}/scripts/dev/codex-hook.sh" ]]; then
  canonical_payload="$(jq -c '
    def canonical_name:
      if . == "run_command" then "Bash"
      elif . == "view_file" then "Read"
      elif . == "write_to_file" then "Write"
      elif . == "replace_file_content" or . == "multi_replace_file_content" then "Edit"
      else . end;
    def canonical_input($name; $args):
      if $name == "run_command" then {command: ($args.CommandLine // ""), cwd: ($args.Cwd // "")}
      elif $name == "view_file" then {file_path: ($args.AbsolutePath // "")}
      elif $name == "write_to_file" or $name == "replace_file_content" or $name == "multi_replace_file_content" then {file_path: ($args.TargetFile // "")}
      else $args end;
    (.toolCall.name // "") as $name |
    (.toolCall.args // {}) as $args |
    {
      session_id: (.conversationId // ""),
      transcript_path: (.transcriptPath // ""),
      cwd: ($args.Cwd // (.workspacePaths[0] // "")),
      hook_event_name: "PreToolUse",
      tool_name: ($name | canonical_name),
      tool_input: canonical_input($name; $args)
    }
  ' <<<"$payload")"

  if hook_output="$(printf '%s' "$canonical_payload" | RALPH_HOOK_PROVIDER=agy bash "${repo_root}/scripts/dev/codex-hook.sh" pre-tool 2>/dev/null)"; then
    if [[ -n "$hook_output" ]] && jq -e '(.decision == "block") or (.hookSpecificOutput.permissionDecision == "deny") or (.hookSpecificOutput.decision.behavior == "deny")' >/dev/null 2>&1 <<<"$hook_output"; then
      reason="$(jq -r '.reason // .hookSpecificOutput.permissionDecisionReason // .hookSpecificOutput.decision.message // "Blocked by safety policy"' <<<"$hook_output")"
      jq -cn --arg reason "$reason" '{decision:"deny",reason:$reason}'
      exit 0
    fi
  fi
fi

case "$tool_name" in
  run_command|write_to_file|replace_file_content|multi_replace_file_content|manage_task|schedule|ask_permission|invoke_subagent|define_subagent|send_message|manage_subagents)
    jq -cn --arg name "$tool_name" '{decision:"force_ask",reason:("Safety policy requires explicit review for tool " + $name)}'
    ;;
  *)
    printf '{"decision":"allow"}\n'
    ;;
esac
