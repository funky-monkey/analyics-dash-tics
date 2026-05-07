# Deployment Setup

## Required GitHub Secrets

Set in Settings → Secrets → Actions:

| Secret | Example |
|---|---|
| `DEPLOY_SSH_KEY` | `-----BEGIN OPENSSH...` |
| `DEPLOY_HOST` | `analytics.yourdomain.com` |
| `DEPLOY_USER` | `analytics` |
| `DEPLOY_PATH` | `/opt/analytics` |

## Server Setup (one-time)

```bash
# Create deployment user and directory
useradd -m -s /bin/bash analytics
mkdir -p /opt/analytics/{bin,static,templates}
chown -R analytics:analytics /opt/analytics

# Install .env
cp .env.example /opt/analytics/.env
# Edit with production values: DATABASE_URL, JWT_SECRET, JWT_REFRESH_SECRET, BASE_URL

# Install systemd service
cp deploy/analytics.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable analytics
systemctl start analytics

# Allow password-less restart
echo "analytics ALL=(ALL) NOPASSWD: /bin/systemctl restart analytics" > /etc/sudoers.d/analytics
```
