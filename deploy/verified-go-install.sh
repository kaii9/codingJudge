#!/bin/sh
set -eu

# Retry only transient transport errors. Keep Go's module checksum validation
# and the caller's exact source/version/flags; integrity failures are terminal.
task_install_log=$(mktemp)
trap 'rm -f "$task_install_log"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
task_attempt=1
while :; do
    if go install "$@" >"$task_install_log" 2>&1; then
        cat "$task_install_log"
        exit 0
    else
        task_status=$?
    fi
    cat "$task_install_log" >&2
    if grep -Eiq 'SECURITY ERROR|checksum mismatch' "$task_install_log"; then
        exit "$task_status"
    fi
    if [ "$task_attempt" -ge 3 ] || ! grep -Eq \
        'stream error:.*INTERNAL_ERROR|TLS handshake timeout|i/o timeout|connection reset by peer|unexpected EOF|502 Bad Gateway|503 Service Unavailable|504 Gateway Timeout' \
        "$task_install_log"; then
        exit "$task_status"
    fi
    printf 'Transient Go download failure (attempt %s/3); retrying with checksum verification intact.\n' "$task_attempt" >&2
    task_attempt=$((task_attempt + 1))
    sleep 2
done
