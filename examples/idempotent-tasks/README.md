# Idempotent Tasks Example

Demonstrates safe task retries with deduplication using the tasks package.

## Components

- **producer/** - Submits tasks with idempotency keys
- **worker/** - Claims and processes tasks with retry handling

## Running

Start the worker:
```bash
go run ./examples/idempotent-tasks/worker/
```

In another terminal, submit tasks:
```bash
go run ./examples/idempotent-tasks/producer/
```

## What This Shows

- Task submission with idempotency keys
- Claim-based ownership
- Automatic retry on failure
- Deduplication of repeated submissions
