# PostgreSQL Worker Queues

## Introduction

PostgreSQL can act as a lightweight distributed queue for background jobs.
This document explains how a fleet of workers can pull jobs from a single
`jobs` table safely, without external message brokers, using only
transactions and row locking primitives that ship with PostgreSQL itself.

The approach scales to a moderate number of workers and jobs per second,
and is a pragmatic choice when a team already operates PostgreSQL and does
not want to add a message broker to their infrastructure.

This introduction sets up the problem: many workers, one jobs table, and the
need to hand each row to exactly one worker at a time.

We will look at claiming jobs, handling retries, and recovering from
worker crashes in the following sections.

## Claiming Jobs

Workers claim jobs atomically using `SELECT ... FOR UPDATE SKIP LOCKED`.

```sql
SELECT id FROM jobs
WHERE status = 'pending'
ORDER BY created_at
FOR UPDATE SKIP LOCKED
LIMIT 1;
```

`FOR UPDATE` locks the selected row for the duration of the transaction.
`SKIP LOCKED` tells PostgreSQL to skip rows that another worker already has
locked, instead of blocking. This means concurrent workers never collide on
the same job row: each worker gets a distinct row, or no row at all if the
queue is empty.

Once a worker has claimed a row, it updates the row's status to `running`
and commits the transaction before doing any actual work, so the lock is
released quickly and the claim is durable even if the worker crashes.

## Retries and Failures

Workers should record a lease expiration timestamp when they claim a job.
A separate reaper process periodically looks for jobs whose lease has
expired without completion and returns them to the `pending` state so
another worker can retry them.

Retries should track an attempt counter and back off exponentially, moving
a job to a `dead` status after too many failures so a single bad job cannot
loop forever.

Because `SKIP LOCKED` releases the lock automatically when a worker process
dies (the transaction is rolled back), a crashed worker's claimed jobs
become visible to other workers again as soon as the connection drops,
even before the lease timestamp expires.

This combination of `FOR UPDATE SKIP LOCKED`, lease timestamps, and retry
counters gives horizontally scaled PostgreSQL-backed worker queues
reasonably strong guarantees without a dedicated queue product.
