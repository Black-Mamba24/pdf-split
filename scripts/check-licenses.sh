#!/usr/bin/env bash
set -euo pipefail

status=0
while IFS= read -r module_dir; do
  [[ -z "$module_dir" ]] && continue
  license_file="$(find "$module_dir" -maxdepth 1 -type f \( -iname 'license*' -o -iname 'copying*' \) | head -n 1)"
  if [[ -z "$license_file" ]]; then
    echo "missing license: $module_dir" >&2
    status=1
    continue
  fi
  if grep -Eiq 'GNU (AFFERO )?GENERAL PUBLIC LICENSE|Server Side Public License' "$license_file"; then
    echo "disallowed copyleft license: $license_file" >&2
    status=1
  fi
done < <(go list -m -f '{{if and (not .Main) .Dir}}{{.Dir}}{{end}}' all)

exit "$status"
