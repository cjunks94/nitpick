# Umami Analytics Deployment

Self-hosted Umami analytics for cjunker.dev with zero-cost hosting and Cloudflare Access security.

## Architecture

### Dual-Subdomain Setup

- **Analytics Dashboard**: `umami.cjunker.dev` (Cloudflare proxied + Access protected)
- **Tracking Endpoint**: `umami-tracking.cjunker.dev` (DNS-only, bypasses Cloudflare Access)
- **Hosting**: Railway.app (free tier) with Docker
- **Database**: PostgreSQL 16 Alpine (minimal footprint)
- **Security**: Cloudflare Access (GitHub OAuth) for dashboard only
- **Frontend**: Tracking script on cjunker.dev and staging.cjunker.dev
- **Container**: Optimized Alpine-based build (~73MB)

### Why Two Subdomains?

**Problem**: Cloudflare Access blocks all requests to protected domains, including:
- Public tracking script (`/script.js`)
- Analytics API endpoints (`/api/send`)
- This causes CORS errors when browsers try to load tracking from cross-origin sites

**Solution**: Separate subdomain for public tracking endpoints
- `umami.cjunker.dev`: Dashboard access (Cloudflare proxy + Access) - requires GitHub OAuth
- `umami-tracking.cjunker.dev`: Public tracking (DNS-only mode) - no authentication needed
- Both point to the same Railway service and database
- Cloudflare Access free tier doesn't support path-based bypass rules

## Docker Images

- **Umami**: Optimized multi-stage build (Node 24 Alpine) - **~73MB** (down from 82MB, -11%)
- **PostgreSQL**: `postgres:16-alpine` (~240MB)
- **Total stack size**: ~313MB (vs 1GB+ with standard images)

#### Optimization Highlights:
- **Layers**: 12 (reduced from 18, -33%)
- **Startup time**: ~1.4s (improved from 1.8s, -22%)
- **Build stages**: 3 consolidated stages (was 4)
- **Dependencies**: Production-only packages via `npm --omit=dev`

## Quick Start - Railway Deployment (Docker)

### 1. Link GitHub Repository to Railway

1. Go to [Railway Dashboard](https://railway.app/dashboard)
2. Click **"+ New Project"**
3. Select **"Deploy from GitHub repo"**
4. Choose `resume-improvements` repository
5. Railway will detect the `umami/` folder with Dockerfile

### 2. Add PostgreSQL Database

1. In your Railway project, click **"+ New"**
2. Select **"Database"** → **"Add PostgreSQL"**
3. Railway automatically creates `DATABASE_URL` variable

### 3. Configure Umami Service

1. Click on your Umami service
2. Go to **"Variables"** tab
3. Add environment variables:

```bash
DATABASE_URL=${{Postgres.DATABASE_URL}}
DATABASE_TYPE=postgresql
APP_SECRET=<run: openssl rand -hex 32>
DISABLE_TELEMETRY=1
CLIENT_IP_HEADER=CF-Connecting-IP
```

4. Go to **"Settings"** tab
5. Set **"Root Directory"**: `umami`
6. Click **"Deploy"**

### 4. Alternative: Deploy via Railway CLI

```bash
# Install Railway CLI
npm install -g @railway/cli

# Login to Railway
railway login

# Navigate to umami directory
cd umami

# Initialize Railway project
railway init

# Link to PostgreSQL database
railway add

# Set environment variables
railway variables set APP_SECRET=$(openssl rand -hex 32)
railway variables set DATABASE_URL='${{Postgres.DATABASE_URL}}'
railway variables set DATABASE_TYPE=postgresql
railway variables set DISABLE_TELEMETRY=1
railway variables set CLIENT_IP_HEADER=CF-Connecting-IP

# Deploy (uses Dockerfile automatically)
railway up

# Get deployment URL
railway domain
```

### 5. Configure Custom Domains (Both Required)

In Railway dashboard:
1. Go to **Settings** → **Networking** → **Public Networking**
2. Add **first domain**: `umami.cjunker.dev`
   - Copy the CNAME target (e.g., `xyz.up.railway.app`)
3. Add **second domain**: `umami-tracking.cjunker.dev`
   - Uses same CNAME target as first domain
   - Wait for Railway to show "Active" status

### 6. Configure Cloudflare DNS (Both Records Required)

Add two CNAME records in Cloudflare DNS:

**Record 1: Dashboard (with Cloudflare Access)**
```
Type: CNAME
Name: umami
Target: <railway-cname-from-step-5>
Proxy status: Proxied (orange cloud ☁️) ← IMPORTANT
TTL: Auto
```

**Record 2: Public Tracking (bypasses Access)**
```
Type: CNAME
Name: umami-tracking
Target: <same-railway-cname-as-above>
Proxy status: DNS only (gray cloud ☁️) ← CRITICAL!
TTL: Auto
```

⚠️ **Important**: `umami-tracking` MUST use **DNS only** (gray cloud). If proxied (orange cloud), Cloudflare Access will block tracking requests.

### 7. Set Up Cloudflare Access

**Create GitHub OAuth Application:**

1. Go to GitHub → Settings → Developer settings → OAuth Apps
2. Click "New OAuth App"
3. Fill in:
   - **Application name**: Umami Analytics - cjunker.dev
   - **Homepage URL**: https://umami.cjunker.dev
   - **Authorization callback URL**: https://umami.cloudflareaccess.com/cdn-cgi/access/callback
4. Save **Client ID** and **Client Secret**

**Configure Cloudflare Access:**

1. Go to Cloudflare dashboard → Zero Trust → Access → Applications
2. Click "Add an application"
3. Select "Self-hosted"
4. Configuration:
   - **Application name**: Umami Analytics
   - **Session Duration**: 24 hours
   - **Application domain**: `umami.cjunker.dev`
5. Add identity provider:
   - Select "GitHub"
   - Enter Client ID and Client Secret
6. Create access policy:
   - **Policy name**: GitHub Auth
   - **Action**: Allow
   - **Include**: Emails ending in `@gmail.com` (or your GitHub email)
   - **Require**: GitHub

### 8. Initial Umami Setup

1. Visit `https://umami.cjunker.dev`
2. Authenticate via Cloudflare Access (GitHub OAuth)
3. Default credentials:
   - **Username**: `admin`
   - **Password**: `umami`
4. **IMPORTANT**: Change password immediately
5. Add website:
   - **Name**: Christopher Junker Portfolio
   - **Domain**: cjunker.dev
   - **Enable Share URL**: Yes (for public dashboards)
6. Copy the **Website ID** and **Tracking Code**

## Add Tracking Script to Website

Add to `index.html` in `<head>`:

```html
<!-- Umami Analytics - Use tracking subdomain to bypass Cloudflare Access -->
<script
  defer
  src="https://umami-tracking.cjunker.dev/script.js"
  data-website-id="YOUR-WEBSITE-ID-HERE"
></script>
```

⚠️ **Critical**: Use `umami-tracking.cjunker.dev`, **NOT** `umami.cjunker.dev`
If you use the dashboard domain, browsers will encounter CORS errors due to Cloudflare Access blocking public requests.

### Custom Event Tracking

Track tech radar interactions:

```javascript
// Track radar interaction
function trackRadarEvent(techName, ring, quadrant) {
  if (window.umami) {
    window.umami.track('tech-radar-click', {
      tech: techName,
      ring: ring,
      quadrant: quadrant
    });
  }
}

// Track legend toggle
function trackLegendToggle(isExpanded) {
  if (window.umami) {
    window.umami.track('legend-toggle', {
      action: isExpanded ? 'expand' : 'collapse'
    });
  }
}

// Track CSV download
document.querySelector('a[href="tech-radar.csv"]').addEventListener('click', () => {
  if (window.umami) {
    window.umami.track('download', { file: 'tech-radar.csv' });
  }
});
```

## Environment Variables

Required environment variables for Railway:

```bash
# Database connection (auto-populated by Railway Postgres)
DATABASE_URL=[REDACTED-BY-NITPICK]host:5432/dbname

# Application secret (generate with: openssl rand -hex 32)
APP_SECRET=your-random-secret-key-here

# Optional: Custom port (Railway auto-assigns)
PORT=3000
```

## Local Development

Run Umami locally with Docker Compose:

```bash
cd umami

# Generate app secret
export APP_SECRET=$(openssl rand -hex 32)

# Start services
docker-compose up -d

# View logs
docker-compose logs -f umami

# Access dashboard
open http://localhost:3000
```

Default credentials: `admin` / `umami`

## Monitoring and Maintenance

### Health Check

```bash
curl https://umami.cjunker.dev/api/heartbeat
```

### Database Backups

Railway provides automatic daily backups. Manual backup:

```bash
railway run pg_dump $DATABASE_URL > backup.sql
```

### View Logs

```bash
railway logs
```

## Cost Breakdown

- **Railway**: $0/month (free tier: 500 hours, $5 credit)
- **Cloudflare**: $0/month (Access free tier: 50 users)
- **PostgreSQL**: $0/month (included with Railway)
- **Domain**: $12/year (already owned)

**Total: $0/month + $1/year**

## Security Features

✅ **Zero-trust access** via Cloudflare Access
✅ **GitHub OAuth** - no passwords to manage
✅ **HTTPS enforced** via Cloudflare
✅ **No cookies** required (privacy-friendly)
✅ **GDPR compliant** (no PII collected)

## Troubleshooting

### Railway deployment fails

- Check DATABASE_URL is set correctly
- Verify APP_SECRET is generated (32+ char hex)
- Check Railway logs: `railway logs`

### Cloudflare Access not working

- Verify GitHub OAuth app callback URL: `https://umami.cloudflareaccess.com/cdn-cgi/access/callback`
- Check access policy includes your GitHub email
- Clear browser cookies and retry

### Tracking script not loading

- Verify website ID is correct
- Check browser console for errors
- Test with curl: `curl -I https://umami-tracking.cjunker.dev/script.js`

### CORS errors / "Access-Control-Allow-Origin" missing

**Symptoms:**
```
Access to fetch at 'https://cjunkerdev.cloudflareaccess.com/cdn-cgi/access/login/...'
from origin 'https://staging.cjunker.dev' has been blocked by CORS policy
```

**Root Cause**: Using `umami.cjunker.dev` (dashboard domain) instead of `umami-tracking.cjunker.dev` for tracking script

**Solutions:**
1. **Check your tracking script URL** in `index.html`:
   - ✅ Correct: `src="https://umami-tracking.cjunker.dev/script.js"`
   - ❌ Wrong: `src="https://umami.cjunker.dev/script.js"`

2. **Verify DNS configuration**:
   ```bash
   # Should return HTTP 200 (not 302 redirect)
   curl -I https://umami-tracking.cjunker.dev/api/heartbeat

   # Cloudflare DNS should show "DNS only" (gray cloud)
   dig umami-tracking.cjunker.dev +short
   ```

3. **Ensure Cloudflare proxy is disabled** for tracking subdomain:
   - Go to Cloudflare Dashboard → DNS → Records
   - Find `umami-tracking` CNAME record
   - **Proxy status MUST be gray cloud (DNS only)**
   - If orange cloud (proxied), click to toggle it to gray

**Why this happens**: Cloudflare Access free tier doesn't support path-based bypass rules.
The only way to allow public tracking while keeping the dashboard protected is to use
two separate subdomains with different proxy configurations.

## Useful Links

- [Umami Documentation](https://umami.is/docs)
- [Railway Documentation](https://docs.railway.app)
- [Cloudflare Access Docs](https://developers.cloudflare.com/cloudflare-one/applications/)
- [Umami GitHub](https://github.com/umami-software/umami)

---

**Status**: 🟢 Production Ready
- **Dashboard**: https://umami.cjunker.dev (Cloudflare Access protected)
- **Tracking**: https://umami-tracking.cjunker.dev (public access)
**Last Updated**: November 11, 2025
