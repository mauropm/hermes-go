---
name: "deploy-go-service"
description: "Deploy a Go service to a Linux server via SSH"
version: "1.0.0"
tags:
  - deployment
  - go
  - ssh
  - linux
  - devops
tools_required:
  - ssh_execute
  - file_write
confidence: 0.95
created_at: "2026-04-01T10:00:00Z"
updated_at: "2026-04-01T10:00:00Z"
usage_count: 5
success_rate: 1.0
---

# Deploy Go Service

## When to Use
When the user asks to deploy a Go application to a remote server.

## Prerequisites
- SSH access to target server configured
- Go binary already compiled for target architecture
- Target server running Linux with systemd
- Service unit file already exists on server

## Steps
1. Verify binary exists and is executable on the local machine
2. SSH to target server and check disk space
3. Stop existing service: `systemctl stop <service-name>`
4. Upload new binary via SCP to `/opt/<service-name>/bin/`
5. Set permissions: `chmod +x /opt/<service-name>/bin/<binary>`
6. Restart service: `systemctl start <service-name>`
7. Verify health endpoint responds: `curl -s http://localhost:<port>/health`
8. Check service status: `systemctl status <service-name>`

## Common Pitfalls
- Binary architecture mismatch — always compile with `GOOS=linux GOARCH=amd64`
- Port already in use from previous instance — ensure `systemctl stop` completes
- Missing environment variables — check `/etc/<service-name>/env`
- Insufficient disk space — check with `df -h` before uploading

## Example Interaction

User: "Deploy my service to prod"

Agent:
1. Compile: `GOOS=linux GOARCH=amd64 go build -o myservice ./cmd/server`
2. SSH to prod server
3. `systemctl stop myservice`
4. SCP binary to server
5. `chmod +x /opt/myservice/bin/myservice`
6. `systemctl start myservice`
7. `curl -s http://localhost:8080/health` → `{"status":"ok"}`
8. Confirm deployment successful

## Rollback
If deployment fails:
1. Stop new service: `systemctl stop <service-name>`
2. Restore previous binary from `/opt/<service-name>/bin/<binary>.bak`
3. Restart: `systemctl start <service-name>`
4. Verify health endpoint
