#!/bin/bash

set -e

if [ "$1" = "--daemon" ]; then
    echo "Deamon mode"
    exec /app/run_daily.sh
fi

if [ "$1" = "bash" ] || [ "$1" = "sh" ] || [ "$1" = "ls" ]; then
    exec "$@"
fi

echo "Direct call $@"
exec /app/main "$@"