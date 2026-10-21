#!/bin/sh

set -e

echo "→ seeding database"

cat scripts/seed.sql | docker compose \
    -f compose.yaml \
    exec -T postgres \
    psql -U lab -d failurelab

echo "✓ database seeded"