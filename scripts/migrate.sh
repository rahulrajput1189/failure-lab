#!/bin/sh

set -e

echo "→ running migrations"

for file in migrations/*.sql
do
    echo "→ applying $file"

    cat "$file" | docker compose \
        -f compose.yaml \
        exec -T postgres \
        psql -U lab -d failurelab
done

echo "✓ migrations complete"