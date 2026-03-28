# Disaster Recovery Runbook

Operational guide for backup, restore, and recovery of the CodeForge platform.

## 1. Scope

This runbook covers disaster recovery for all production services:

- **PostgreSQL 18** -- primary data store (projects, users, sessions, conversations, audit log)
- **NATS JetStream** -- message queue and ephemeral state (streams, consumers)
- **LiteLLM** -- LLM proxy (stateless, config in PostgreSQL + litellm-config.yaml)
- **Go Core** -- HTTP/WS server, agent lifecycle, scheduling (stateless binary)
- **Python Workers** -- LLM integration, agent execution (stateless processes)

## 2. RTO / RPO Targets

| Scenario | RPO (data loss) | RTO (downtime) | Notes |
|---|---|---|---|
| Single container crash | 0 | < 2 min | Docker restart policy handles this |
| PostgreSQL corruption | < 1 hour | < 30 min | Restore from hourly pg_dump |
| PostgreSQL disk failure | < 5 min (WAL) | < 1 hour | PITR from base backup + WAL replay |
| NATS volume loss | 0 (ephemeral) | < 5 min | Streams and consumers auto-recreate |
| Full host failure | < 1 hour | < 2 hours | Restore all volumes from backup |
| Accidental data deletion | < 1 hour | < 15 min | Point-in-time recovery |

## 3. PostgreSQL Backup

### 3.1 Logical Backup (pg_dump)

Recommended for hourly scheduled backups. Produces a portable SQL dump.

```bash
# Run from the Docker host or a backup sidecar container.
# Uses the codeforge-postgres service name from docker-compose.prod.yml.

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR=/backups/postgres

docker exec codeforge-postgres pg_dump \
  -U codeforge \
  -d codeforge \
  --format=custom \
  --compress=zstd:6 \
  --file=/tmp/codeforge_${TIMESTAMP}.dump

docker cp codeforge-postgres:/tmp/codeforge_${TIMESTAMP}.dump \
  ${BACKUP_DIR}/codeforge_${TIMESTAMP}.dump

# Clean up inside container
docker exec codeforge-postgres rm /tmp/codeforge_${TIMESTAMP}.dump
```

Schedule via cron:

```
0 * * * * /opt/codeforge/scripts/pg-backup.sh >> /var/log/codeforge-backup.log 2>&1
```

### 3.2 Physical Backup (pg_basebackup)

Required for point-in-time recovery (PITR). Take a base backup weekly.

Prerequisites -- enable WAL archiving in PostgreSQL:

```
# postgresql.conf (or via environment in docker-compose.prod.yml)
wal_level = replica
archive_mode = on
archive_command = 'cp %p /backups/postgres/wal/%f'
```

Take the base backup:

```bash
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR=/backups/postgres/base

docker exec codeforge-postgres pg_basebackup \
  -U codeforge \
  -D /tmp/basebackup_${TIMESTAMP} \
  --format=tar \
  --gzip \
  --checkpoint=fast \
  --wal-method=stream

docker cp codeforge-postgres:/tmp/basebackup_${TIMESTAMP} \
  ${BACKUP_DIR}/basebackup_${TIMESTAMP}

docker exec codeforge-postgres rm -rf /tmp/basebackup_${TIMESTAMP}
```

### 3.3 Retention Policy

| Backup type | Frequency | Retention |
|---|---|---|
| pg_dump (logical) | Hourly | 48 hours (48 files) |
| pg_basebackup (physical) | Weekly | 4 weeks (4 snapshots) |
| WAL archives | Continuous | Until next base backup + 1 week |

## 4. PostgreSQL Restore

### 4.1 From Logical Dump

```bash
# Stop all services that connect to PostgreSQL
docker compose -f docker-compose.prod.yml stop core litellm workers

# Drop and recreate the database
docker exec codeforge-postgres psql -U codeforge -c "DROP DATABASE IF EXISTS codeforge;"
docker exec codeforge-postgres psql -U codeforge -c "CREATE DATABASE codeforge OWNER codeforge;"

# Restore from dump
docker cp ${BACKUP_DIR}/codeforge_${TIMESTAMP}.dump codeforge-postgres:/tmp/restore.dump
docker exec codeforge-postgres pg_restore \
  -U codeforge \
  -d codeforge \
  --no-owner \
  --no-privileges \
  /tmp/restore.dump

docker exec codeforge-postgres rm /tmp/restore.dump

# Restart services
docker compose -f docker-compose.prod.yml up -d core litellm workers
```

### 4.2 From Base Backup with WAL Replay (PITR)

Use this to recover to a specific point in time (e.g., just before accidental deletion).

```bash
# Stop PostgreSQL
docker compose -f docker-compose.prod.yml stop postgres

# Remove existing data volume
docker volume rm codeforge_postgres_data

# Create fresh volume and restore base backup
docker volume create codeforge_postgres_data
docker run --rm \
  -v codeforge_postgres_data:/var/lib/postgresql/data \
  -v ${BACKUP_DIR}/basebackup_${TIMESTAMP}:/backup:ro \
  postgres:18 \
  bash -c "tar xzf /backup/base.tar.gz -C /var/lib/postgresql/data"

# Create recovery signal file with target time
docker run --rm \
  -v codeforge_postgres_data:/var/lib/postgresql/data \
  postgres:18 \
  bash -c "cat > /var/lib/postgresql/data/recovery.signal && \
    echo \"restore_command = 'cp /backups/postgres/wal/%f %p'\" >> /var/lib/postgresql/data/postgresql.auto.conf && \
    echo \"recovery_target_time = '${TARGET_TIME}'\" >> /var/lib/postgresql/data/postgresql.auto.conf && \
    echo \"recovery_target_action = 'promote'\" >> /var/lib/postgresql/data/postgresql.auto.conf"

# Start PostgreSQL -- it will replay WAL up to TARGET_TIME
docker compose -f docker-compose.prod.yml up -d postgres

# Monitor recovery progress
docker logs -f codeforge-postgres

# After recovery completes, restart remaining services
docker compose -f docker-compose.prod.yml up -d core litellm workers
```

## 5. NATS JetStream Recovery

NATS JetStream state is ephemeral for CodeForge. The Go backend auto-recreates streams and consumers on startup (see `internal/port/messagequeue/jetstream.go`).

### Recovery Steps

```bash
# Stop NATS
docker compose -f docker-compose.prod.yml stop nats

# Remove NATS data volume
docker volume rm codeforge_nats_data

# Recreate and start
docker volume create codeforge_nats_data
docker compose -f docker-compose.prod.yml up -d nats

# Restart Go Core -- it recreates streams/consumers automatically
docker compose -f docker-compose.prod.yml restart core
```

No data is lost because:
- All durable state is in PostgreSQL
- In-flight messages are retried by publishers
- Consumers are recreated by Go Core on startup (FIX-10)

## 6. LiteLLM Recovery

LiteLLM is stateless. Its configuration comes from two sources:

1. **litellm-config.yaml** -- model routing rules (mounted as a volume)
2. **PostgreSQL** -- API keys, usage tracking (shared database)

### Recovery Steps

```bash
# Simply recreate the container
docker compose -f docker-compose.prod.yml up -d --force-recreate litellm

# Verify health
curl -s http://localhost:4000/health | jq .
```

If the config file is lost, restore from version control:

```bash
git checkout -- litellm-config.yaml
docker compose -f docker-compose.prod.yml restart litellm
```

## 7. Full Recovery Runbook

Use this checklist for a complete platform recovery (e.g., host migration, full disk failure).

### Prerequisites

- [ ] Docker and Docker Compose installed on the target host
- [ ] Access to backup storage (pg_dump files, base backups, WAL archives)
- [ ] CodeForge repository cloned (for docker-compose.prod.yml and configs)
- [ ] Environment variables configured (.env file)

### Step-by-Step

1. **Clone the repository**
   ```bash
   git clone https://github.com/Strob0t/CodeForge.git
   cd CodeForge
   git checkout <production-tag>
   ```

2. **Restore environment configuration**
   ```bash
   cp /backups/env/.env.prod .env
   ```

3. **Start PostgreSQL only**
   ```bash
   docker compose -f docker-compose.prod.yml up -d postgres
   ```

4. **Restore PostgreSQL from latest dump**
   ```bash
   LATEST=$(ls -t /backups/postgres/codeforge_*.dump | head -1)
   docker cp ${LATEST} codeforge-postgres:/tmp/restore.dump
   docker exec codeforge-postgres pg_restore \
     -U codeforge -d codeforge --no-owner --no-privileges /tmp/restore.dump
   docker exec codeforge-postgres rm /tmp/restore.dump
   ```

5. **Verify database integrity**
   ```bash
   docker exec codeforge-postgres psql -U codeforge -d codeforge \
     -c "SELECT count(*) FROM users; SELECT count(*) FROM projects;"
   ```

6. **Start NATS**
   ```bash
   docker compose -f docker-compose.prod.yml up -d nats
   ```

7. **Start LiteLLM**
   ```bash
   docker compose -f docker-compose.prod.yml up -d litellm
   ```

8. **Start Go Core**
   ```bash
   docker compose -f docker-compose.prod.yml up -d core
   ```

9. **Verify Go Core health**
   ```bash
   curl -s http://localhost:8080/health | jq .
   ```

10. **Start Python Workers**
    ```bash
    docker compose -f docker-compose.prod.yml up -d workers
    ```

11. **Verify end-to-end connectivity**
    ```bash
    # Check NATS streams
    curl -s http://localhost:8222/jsz | jq '.streams'

    # Check LiteLLM models
    curl -s http://localhost:4000/v1/models -H "Authorization: Bearer ${LITELLM_MASTER_KEY}" | jq '.data | length'

    # Check API
    curl -s http://localhost:8080/api/v1/health | jq .
    ```

12. **Start frontend (if separate)**
    ```bash
    docker compose -f docker-compose.prod.yml up -d frontend
    ```

## 8. Backup Verification

Run monthly to ensure backups are restorable.

### Procedure

1. **Spin up an isolated test environment**
   ```bash
   docker compose -f docker-compose.test-restore.yml up -d postgres-test
   ```

2. **Restore the latest dump into the test database**
   ```bash
   LATEST=$(ls -t /backups/postgres/codeforge_*.dump | head -1)
   docker cp ${LATEST} postgres-test:/tmp/restore.dump
   docker exec postgres-test pg_restore \
     -U codeforge -d codeforge_test --no-owner --no-privileges /tmp/restore.dump
   ```

3. **Run integrity checks**
   ```bash
   docker exec postgres-test psql -U codeforge -d codeforge_test -c "
     SELECT schemaname, tablename, n_live_tup
     FROM pg_stat_user_tables
     ORDER BY n_live_tup DESC
     LIMIT 20;
   "
   ```

4. **Verify row counts match production**
   ```bash
   # Compare key table counts against production
   for table in users projects conversations agents tasks runs; do
     echo "--- ${table} ---"
     docker exec postgres-test psql -U codeforge -d codeforge_test -t \
       -c "SELECT count(*) FROM ${table};"
   done
   ```

5. **Tear down test environment**
   ```bash
   docker compose -f docker-compose.test-restore.yml down -v
   ```

6. **Record results** in the operations log with date, backup file used, and pass/fail status.

## 9. Volume Reference

| Docker Volume | Service | Data | Backup? |
|---|---|---|---|
| codeforge_postgres_data | codeforge-postgres | Database files | Yes (pg_dump + pg_basebackup) |
| codeforge_nats_data | codeforge-nats | JetStream state | No (auto-recreated) |
| codeforge_litellm_config | codeforge-litellm | litellm-config.yaml | No (in version control) |
| codeforge_workspaces | codeforge-core | Cloned repositories | Optional (re-clone from VCS) |
