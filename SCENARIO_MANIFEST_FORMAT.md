# Scenario Manifest Format

FailureLab scenario manifests describe scenarios independently of their Go execution and verification implementations.

Each scenario directory contains a `scenario.yaml` file.

## Version 1

Example:

```yaml
schemaVersion: 1
id: inventory-reservation-leak
name: Inventory Reservation Leak
description: Inventory remains reserved after the order transaction rolls back.
tags:
  - inventory
  - orders
  - transaction
```

## Fields

| Field | Type | Required | Meaning |
|---|---|---|---|
| schemaVersion | integer | Yes | Manifest schema version. Version 1 is the initial format. |
| id | string | Yes | Stable scenario identifier, matching the registered Go scenario ID. |
| name | string | Yes | Human-readable scenario name. |
| description | string | Yes | Description of the failure being investigated. |
| tags | list of strings | Yes | Labels used to categorize the scenario. |

## Validation Rules

- `schemaVersion` must equal `1`.
- `id`, `name`, and `description` must not be empty.
- `id` must match the scenario directory name and registered scenario ID.
- `tags` must contain at least one non-empty string.
- Scenario IDs must be unique across manifests.
- Unknown schema versions must be rejected rather than silently interpreted.
- Parsing a manifest must not execute scenario code or modify application data.

## Separation of Responsibilities

The manifest defines metadata only. Go code remains responsible for scenario reproduction and verification.

The scenario registry will connect manifest IDs to the existing Go handlers in the next implementation step.

This document defines the contract. Runtime parsing and validation will be implemented separately.