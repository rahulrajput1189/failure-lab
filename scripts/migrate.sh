#!/bin/sh

set -eu

psql() {
    docker compose -f compose.yaml exec -T postgres \
        psql -v ON_ERROR_STOP=1 -U lab -d failurelab "$@"
}

printf '%s\n' '→ preparing migration ledger'
psql -c 'CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())'

# Existing FailureLab databases predate the migration ledger. If the original
# schema is already present, record migration 001 as the existing baseline.
baseline_exists=$(psql -Atc "SELECT (to_regclass('public.users') IS NOT NULL AND to_regclass('public.products') IS NOT NULL AND to_regclass('public.orders') IS NOT NULL)::text")
if [ "$baseline_exists" = "true" ]; then
    psql -c "INSERT INTO schema_migrations (name) VALUES ('001_initial.sql') ON CONFLICT (name) DO NOTHING"
fi

for file in migrations/*.sql
do
    name=$(basename "$file")
    applied=$(psql -Atc "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = '$name')::text")

    if [ "$applied" = "true" ]; then
        printf '→ skipping %s (already applied)\n' "$name"
        continue
    fi

    printf '→ applying %s\n' "$name"
    psql < "$file"
    psql -c "INSERT INTO schema_migrations (name) VALUES ('$name') ON CONFLICT (name) DO NOTHING"
done

printf '%s\n' '✓ migrations complete'
