---
name: deploy-test-101
description: Deploy latest tag to 192.168.1.101 test server (local Docker)
trigger: user-invocable
---

# Deploy to 192.168.1.101 Test Server

Deploy the latest git tag to the local Docker container `hubscope` on 192.168.1.101 (ThinkBook dev machine).

## Context
- Host: 192.168.1.101 (local machine, no SSH needed)
- Container name: `hubscope`
- Data volume: `/opt/hubscope/data` → `/data`
- Port binding: `192.168.1.101:8080:8080`
- Health endpoint: `http://192.168.1.101:8080/healthz`
- Restart policy: `unless-stopped`

## Steps

1. **Identify latest tag**
   ```bash
   git describe --tags --abbrev=0
   ```

2. **Build image from tag**
   ```bash
   TAG=$(git describe --tags --abbrev=0)
   rm -rf /tmp/hubscope-build-$TAG
   mkdir -p /tmp/hubscope-build-$TAG
   git archive $TAG | tar -x -C /tmp/hubscope-build-$TAG
   cd /tmp/hubscope-build-$TAG && docker build -t hubscope:$TAG .
   ```

3. **Backup database and record rollback anchor**
   ```bash
   PREV_IMAGE=$(docker inspect hubscope --format '{{.Image}}')
   TS=$(date +%Y%m%d-%H%M%S)
   sudo cp /opt/hubscope/data/app.db /opt/hubscope/data/app.db.bak-$TS
   echo "Rollback: docker stop hubscope && docker run -d --name hubscope -p 192.168.1.101:8080:8080 -v /opt/hubscope/data:/data --restart unless-stopped $PREV_IMAGE"
   ```

4. **Recreate container**
   ```bash
   docker rm -f hubscope
   docker run -d --name hubscope -p 192.168.1.101:8080:8080 -v /opt/hubscope/data:/data --restart unless-stopped hubscope:$TAG
   ```

5. **Fix ownership (critical: new Dockerfile runs as uid 10001)**
   ```bash
   sudo chown -R 10001:10001 /opt/hubscope/data
   docker restart hubscope
   ```

6. **Health check**
   ```bash
   for i in {1..30}; do
     code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 http://192.168.1.101:8080/healthz)
     [ "$code" = "200" ] && { echo "healthy"; break; }
     sleep 2
   done
   ```

7. **Tag as latest if healthy**
   ```bash
   docker tag hubscope:$TAG hubscope:latest
   ```

## Rollback
If health check fails:
```bash
docker rm -f hubscope
docker run -d --name hubscope -p 192.168.1.101:8080:8080 -v /opt/hubscope/data:/data --restart unless-stopped $PREV_IMAGE
sudo chown -R 10001:10001 /opt/hubscope/data
docker restart hubscope
```

## Key Lessons
- **Ownership mismatch**: Dockerfile v0.2.3+ runs as non-root (uid 10001). After container recreate, must `chown -R 10001:10001 /opt/hubscope/data` before service can migrate/write the database.
- **Old images**: Previous images ran as root, so existing `app.db` is owned by root. The chown step is mandatory on first upgrade to v0.2.3+.
- **Backup first**: Always backup `app.db` before recreate — migrate failures leave the db untouched, but better safe than sorry.
