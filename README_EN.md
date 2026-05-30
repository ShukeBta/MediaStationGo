# MediaStationGo

<p align="center">
  <img src="web/public/favicon.svg" width="96" height="96" alt="MediaStationGo Logo" />
</p>

<h3 align="center">A lightweight, polished, NAS-friendly private media center</h3>

<p align="center">
  <strong>Go single-binary backend · React frontend · Docker-first deployment · Emby-compatible APIs · Multi-source metadata · PT subscriptions</strong>
</p>

<p align="center">
  <a href="README.md">中文</a> ·
  <a href="#quick-start">Quick Start</a> ·
  <a href="#docker-compose-deploy">Docker Deploy</a> ·
  <a href="#screenshots">Screenshots</a> ·
  <a href="https://mgo.3jzs.com">Live Demo</a>
</p>

<p align="center">
  <img alt="Go" src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat-square&logo=go&logoColor=white" />
  <img alt="React" src="https://img.shields.io/badge/React-18-61DAFB?style=flat-square&logo=react&logoColor=111827" />
  <img alt="TypeScript" src="https://img.shields.io/badge/TypeScript-5-3178C6?style=flat-square&logo=typescript&logoColor=white" />
  <img alt="Docker" src="https://img.shields.io/badge/Docker-ready-2496ED?style=flat-square&logo=docker&logoColor=white" />
  <img alt="License" src="https://img.shields.io/badge/License-GPL--3.0-blue?style=flat-square" />
  <img alt="Use" src="https://img.shields.io/badge/Use-Non--Commercial-orange?style=flat-square" />
</p>

---

## ✨ Overview

MediaStationGo is an open-source media center for personal libraries, family NAS setups, and home-theater enthusiasts. It combines library management, metadata scraping, web playback, third-party client compatibility, PT site search, subscription-based downloads, and AI-assisted recommendations into one lightweight Go service with a polished React interface.

It is designed for users who want to:

- Manage movies, TV shows, anime, variety shows, adult content, and music from one place.
- Prefer local NFO/images first, then enrich missing metadata from TMDb, Douban, Bangumi, TheTVDB, Fanart, JavBus, and JavDB.
- Search PT sites, subscribe to resources, enqueue downloads, and organize completed files from one panel.
- Connect external clients such as Infuse, VidHub, SenPlayer, and other Emby/Jellyfin-compatible apps.
- Deploy easily on NAS, Windows, Linux, macOS, or Docker without leaking private tracker tokens to the frontend.

> The project is under active development. For production use, pin a release image tag and back up `/data` regularly.

---

## 🌱 Open Source Promise

MediaStationGo follows a fully open-source path. The core media library, metadata scraping, playback, subscriptions, downloads, external-client compatibility, and operations features are developed in this repository. The project learns from excellent open-source projects such as MoviePilot in areas like site aggregation, subscription downloads, post-download organization, and Emby/Jellyfin client compatibility, while keeping MediaStationGo as an independent implementation and avoiding incompatible code reuse.

The current base license is `GPL-3.0`, and contributions are welcome under that license: site adapters, scraping rules, UI improvements, documentation, and bug fixes all help the project grow. The maintainers also state the intended usage boundary: MediaStationGo is provided for personal learning, home NAS, self-hosted media, and non-commercial scenarios. Without explicit written permission from the author, do not use this project or derivative versions for commercial resale, paid hosting, paid SaaS, pre-installed commercial devices, closed-source redistribution, or other profit-oriented commercial use.

> Note: GPL-3.0 is a free software license, and the formal grant is defined by the repository [LICENSE](LICENSE) file. The non-commercial commitment above expresses the maintainer's intended usage boundary and commercial cooperation requirements. For commercial cooperation, enterprise deployment, or redistribution, contact the author for additional authorization first.

### Source Availability and Docker Support Boundary

- The public repository currently uses `GPL-3.0` as its base license. If a component is GPL-derived, distributing it only as a Docker image does not remove the corresponding source-distribution obligations.
- The project can still define its official support scope as **Docker-first / Docker-only support**: Docker Compose, GHCR images, and container deployment docs are maintained as the supported path, while bare-metal binaries can be community/best-effort.
- If some future functionality needs to be closed-source, keep it as a separate plugin, private service, or independently implemented module whose license boundary is clean. GPL-covered code should remain available under GPL terms.
- The README non-commercial statement describes the maintainer's intended usage boundary and commercial authorization requirement; the formal code license remains governed by [LICENSE](LICENSE).

---

## 🚀 Live Demo

- Demo: [https://mgo.3jzs.com](https://mgo.3jzs.com)
- Username: `admin`
- Password: `admin123`

> The demo is for feature preview only. Do not store private API keys, tracker tokens, or personal media information there.

---

## 🧭 Feature Overview

| Area | Capabilities |
| --- | --- |
| Libraries | Movies, TV, anime, variety, music, adult content; folder covers, series, seasons, and episodes |
| Scanning | Recursive scanning, ffprobe probing, filename parsing, season/episode recognition, duplicate prevention |
| Local metadata | NFO, poster, fanart, season poster, episode image, local adult artwork first |
| Online metadata | TMDb, TheTVDB, Bangumi, Douban, Fanart.tv, JavBus/JavDB page scraping |
| Playback | Direct streaming, HTTP Range seeking, HLS transcoding, external subtitles, progress, resume, external players |
| Discovery | TMDb / Douban / Bangumi recommendation rails and subscription entry points |
| PT sites | Site management, M-Team API token, cross-site search, download URL resolution |
| Subscriptions | RSS/search subscriptions, resolution/quality/effects/release-group rules, wash toggle and priorities |
| Downloads | qBittorrent task cards, status, speed, progress, uploaded/downloaded size, private URL redaction |
| Compatibility | Emby-style APIs for Infuse, VidHub, SenPlayer, and similar clients |
| Operations | Runtime status, task queue, duplicate files, recycle bin, file manager, storage settings, notifications |
| AI | OpenAI-compatible API settings, AI search, recommendations, and operations assistant |

---

<a id="screenshots"></a>

## 🖼️ Screenshots

> Screenshots below were captured from a running local instance. Personal media, local paths, accounts, API keys, tokens, and secrets have been visually redacted.

<details open>
<summary><strong>Core Experience</strong></summary>

| Login & Home | Library Overview |
| --- | --- |
| <img src="docs/screenshots/00-login.jpg" alt="Login" width="100%"> | <img src="docs/screenshots/01-home.jpg" alt="Home" width="100%"> |
| Libraries | Library Detail |
| <img src="docs/screenshots/02-libraries.jpg" alt="Libraries" width="100%"> | <img src="docs/screenshots/03-library-detail.jpg" alt="Library Detail" width="100%"> |
| Poster Wall | Media Detail |
| <img src="docs/screenshots/04-poster-wall.jpg" alt="Poster Wall" width="100%"> | <img src="docs/screenshots/05-media-detail.jpg" alt="Media Detail" width="100%"> |
| Player | Discover |
| <img src="docs/screenshots/06-player.jpg" alt="Player" width="100%"> | <img src="docs/screenshots/07-discover.jpg" alt="Discover" width="100%"> |
| Smart Search | DLNA Cast |
| <img src="docs/screenshots/08-search.jpg" alt="Smart Search" width="100%"> | <img src="docs/screenshots/09-dlna.jpg" alt="DLNA" width="100%"> |

</details>

<details>
<summary><strong>Personal Space & Playback</strong></summary>

| AI Assistant | Favorites |
| --- | --- |
| <img src="docs/screenshots/10-ai.jpg" alt="AI Assistant" width="100%"> | <img src="docs/screenshots/11-favourites.jpg" alt="Favorites" width="100%"> |
| Playlists | Watch History |
| <img src="docs/screenshots/12-playlists.jpg" alt="Playlists" width="100%"> | <img src="docs/screenshots/13-history.jpg" alt="Watch History" width="100%"> |
| Profile | Downloads |
| <img src="docs/screenshots/14-profile.jpg" alt="Profile" width="100%"> | <img src="docs/screenshots/15-downloads.jpg" alt="Downloads" width="100%"> |

</details>

<details>
<summary><strong>Downloads, Subscriptions & Sites</strong></summary>

| Download Clients | Subscriptions |
| --- | --- |
| <img src="docs/screenshots/16-download-clients.jpg" alt="Download Clients" width="100%"> | <img src="docs/screenshots/17-subscriptions.jpg" alt="Subscriptions" width="100%"> |
| Site Search | Sites & Downloaders |
| <img src="docs/screenshots/18-site-search.jpg" alt="Site Search" width="100%"> | <img src="docs/screenshots/20-sites.jpg" alt="Sites" width="100%"> |

</details>

<details>
<summary><strong>Administration & Operations</strong></summary>

| Media & Users | Tools |
| --- | --- |
| <img src="docs/screenshots/19-admin.jpg" alt="Admin" width="100%"> | <img src="docs/screenshots/21-tools.jpg" alt="Tools" width="100%"> |
| Storage & Files | Runtime Status |
| <img src="docs/screenshots/22-storage.jpg" alt="Storage" width="100%"> | <img src="docs/screenshots/23-stats.jpg" alt="Stats" width="100%"> |
| Settings | Tasks |
| <img src="docs/screenshots/24-settings.jpg" alt="Settings" width="100%"> | <img src="docs/screenshots/25-tasks.jpg" alt="Tasks" width="100%"> |
| Duplicates | Recycle Bin |
| <img src="docs/screenshots/26-duplicates.jpg" alt="Duplicates" width="100%"> | <img src="docs/screenshots/27-recycle.jpg" alt="Recycle Bin" width="100%"> |
| Scheduler | File Manager |
| <img src="docs/screenshots/28-scheduler.jpg" alt="Scheduler" width="100%"> | <img src="docs/screenshots/29-files.jpg" alt="Files" width="100%"> |
| STRM | Storage Config |
| <img src="docs/screenshots/30-strm.jpg" alt="STRM" width="100%"> | <img src="docs/screenshots/31-storage-config.jpg" alt="Storage Config" width="100%"> |
| Notifications | Operations Assistant |
| <img src="docs/screenshots/32-notify-channels.jpg" alt="Notifications" width="100%"> | <img src="docs/screenshots/33-assistant.jpg" alt="Operations Assistant" width="100%"> |

</details>

---

## 🧱 Tech Stack

| Layer | Technology | Notes |
| --- | --- | --- |
| Backend | Go 1.25+ | Single-binary deployment, low resource usage |
| Web framework | Gin | REST APIs, auth middleware, static file serving |
| Database | SQLite + GORM | Simple backup and migration for personal/NAS usage |
| Frontend | React 18 + TypeScript | Typed component-based UI |
| Build tool | Vite | Fast frontend development and production builds |
| Styling | Tailwind CSS | Unified bright visual system and responsive layout |
| State | Zustand | Lightweight auth and permission state |
| Playback | HTML5 Video / HLS / FFmpeg | Direct play, Range seek, HLS transcoding, subtitles |
| Metadata | TMDb / Douban / Bangumi / TheTVDB / Fanart / JavBus / JavDB | Posters, backdrops, descriptions, ratings, episode metadata |
| Downloads | qBittorrent / PT site adapters | Search, subscriptions, task cards, private URL redaction |
| Compatibility | Emby-style API / DLNA | External clients and player integrations |
| Deployment | Docker / Docker Compose / Shell / PowerShell | NAS, Linux, Windows, and macOS friendly |
| CI/CD | GitHub Actions / GHCR | Version tags or manual Actions runs publish multi-arch Docker images and release packages |

---

<a id="quick-start"></a>

## 📦 Quick Start

<a id="docker-compose-deploy"></a>

### Docker Compose (Recommended)

Docker is the most stable and portable deployment option. The default compose file creates four main mounts:

| Host path | Container path | Purpose |
| --- | --- | --- |
| `./data` | `/data` | Database, JWT secret, runtime settings. Back this up. |
| `./cache` | `/cache` | Posters, backdrops, scraping cache, transcoding cache |
| `./media` | `/media` | Media library root, mounted read-only by default |
| `./downloads` | `/downloads` | Subscription/site download target |

#### Linux / NAS Zero-to-One Deployment Without Cloning Source

This path is friendly for Ubuntu, Debian, CentOS, AlmaLinux, Rocky Linux, and most Linux-based NAS hosts.

1. Install Docker:

```bash
bash <(curl -sSL https://cdn.jsdelivr.net/gh/SuperManito/LinuxMirrors@main/DockerInstallation.sh)

docker --version
```

2. Install Docker Compose:

```bash
curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
chmod +x /usr/local/bin/docker-compose

docker-compose version
```

> If writing to `/usr/local/bin/docker-compose` fails with a permission error, prefix the `curl` and `chmod` commands with `sudo`.

> If your system already supports `docker compose version`, keep using `docker compose`. If you installed only the standalone binary above, replace later `docker compose` commands with `docker-compose`.

3. Create the deployment directory:

```bash
mkdir -p ~/MediaStationGo
cd ~/MediaStationGo
mkdir -p data cache media downloads
```

4. Create `.env`. For NAS deployments, the recommended mode is mapping the host absolute path to the exact same container path, so the web UI can use `/your-nas/...` directly:

```bash
cat > .env <<'EOF'
MEDIASTATION_IMAGE_TAG=MediaStationGo-v0.0.32
MEDIASTATION_HTTP_PORT=18080
MEDIASTATION_DATA_DIR=./data
MEDIASTATION_CACHE_DIR=./cache
MEDIASTATION_MEDIA_DIR=/your-nas/media
MEDIASTATION_MEDIA_CONTAINER_DIR=/your-nas/media
MEDIASTATION_DOWNLOAD_DIR=/your-nas/downloads
MEDIASTATION_DOWNLOAD_CONTAINER_DIR=/your-nas/downloads
# If the NAS already uses v2rayA redirect / transparent proxy, do not add HTTP_PROXY here.
TZ=Asia/Shanghai
PUID=1000
PGID=1000
EOF
```

> Important: do not write NAS absolute paths as relative paths (e.g. `./media`). A relative path means a subdirectory under the current compose project, which can become a wrong path such as `/your-nas/MediaStationGo/media/...`. Correct NAS paths must start with `/` (absolute), e.g. `/your-nas/media`.

For local testing without existing NAS folders, use the deploy-directory mounts instead:

```env
MEDIASTATION_MEDIA_DIR=./media
MEDIASTATION_MEDIA_CONTAINER_DIR=/media
MEDIASTATION_DOWNLOAD_DIR=./downloads
MEDIASTATION_DOWNLOAD_CONTAINER_DIR=/downloads
```

5. Download the default compose file:

```bash
curl -fsSL https://raw.githubusercontent.com/ShukeBta/MediaStationGo/main/docker-compose.yml -o docker-compose.yml
```

If GitHub Raw is unavailable, create it manually and paste the template from `docker-compose.yml` in this repository:

```bash
vi docker-compose.yml
# or
vim docker-compose.yml
```

6. Start MediaStationGo:

```bash
docker compose pull
docker compose up -d

# If only docker-compose is available:
# docker-compose pull
# docker-compose up -d
```

For existing deployments, use the update helper. It pulls and recreates the service, then removes old unused `ghcr.io/shukebta/mediastation-go` images and dangling layers so NAS disks do not fill up with historical images:

```bash
curl -fsSL https://cdn.jsdelivr.net/gh/ShukeBta/MediaStationGo@main/scripts/docker-compose-update.sh -o docker-compose-update.sh
chmod +x docker-compose-update.sh
./docker-compose-update.sh
```

7. Check status and logs:

```bash
docker compose ps
docker compose logs -f mediastation-go

# Or:
# docker-compose ps
# docker-compose logs -f mediastation-go
```

Open:

```text
http://<server-ip>:18080
```

Default account:

```text
Username: admin
Password: admin123
```

> Change the administrator password immediately after first login. If LAN access fails, check the server firewall, NAS firewall, security group, and the `18080:8080` port mapping.

#### Quick Start from Source Checkout

Developers or users who already cloned the repository can use the built-in compose file directly:

```bash
git clone https://github.com/ShukeBta/MediaStationGo.git
cd MediaStationGo

docker compose pull
docker compose up -d
```

For later upgrades, run this from the deployment directory:

```bash
./docker-compose-update.sh
```

### Pin a Release Version

For production, pin a specific release tag instead of using `latest`. Recommended NAS `.env`:

```bash
cat > .env <<'EOF'
MEDIASTATION_IMAGE_TAG=MediaStationGo-v0.0.32
MEDIASTATION_HTTP_PORT=18080
MEDIASTATION_DATA_DIR=./data
MEDIASTATION_CACHE_DIR=./cache
MEDIASTATION_MEDIA_DIR=/your-nas/media
MEDIASTATION_MEDIA_CONTAINER_DIR=/your-nas/media
MEDIASTATION_DOWNLOAD_DIR=/your-nas/downloads
MEDIASTATION_DOWNLOAD_CONTAINER_DIR=/your-nas/downloads
TZ=Asia/Shanghai
PUID=1000
PGID=1000
EOF

docker compose pull
docker compose up -d
```

### Media Library Paths

#### Host Paths vs Container Paths

The left side of a Docker Compose volume is the real host directory, and the right side is the path inside the container. Absolute NAS host paths are read directly; they are not copied into the deployment folder and will not be nested under the compose project directory.

```yaml
# Real host path                         # Container path
- /your-nas/media:/media:ro
- /your-nas/downloads:/downloads
```

Inside the app, use only `/media` and `/downloads`. If you see `/your-nas/MediaStationGo/your-nas/...`, your `.env` or compose file probably uses `./your-nas/...`; remove the dot and use `/your-nas/...`.

> If adding `/your-nas/media/电视剧/国产剧` as a library reports inaccessible, the app is running inside the container and normally sees `/media/电视剧/国产剧`. The updated compose passes host-path hints so the backend can auto-convert common mistakes, but `/media/...` remains the recommended and most stable input.

If you prefer entering the NAS absolute path directly in the web UI, map the host path to the exact same container path. This does not copy files or use extra disk space:

```env
MEDIASTATION_MEDIA_DIR=/your-nas/media
MEDIASTATION_MEDIA_CONTAINER_DIR=/your-nas/media
MEDIASTATION_DOWNLOAD_DIR=/your-nas/downloads
MEDIASTATION_DOWNLOAD_CONTAINER_DIR=/your-nas/downloads
```

Equivalent compose mounts:

```yaml
- /your-nas/media:/your-nas/media:ro
- /your-nas/downloads:/your-nas/downloads
```

With this mode, add libraries using paths such as `/your-nas/media/电视剧/国产剧`.

> Safety policy: scanning and playback read the original directory. “Organize entire library” no longer moves files already inside the library root, protecting local NFO, posters, subtitles, and other sidecar metadata. Auto-organize after downloads remains disabled by default.

If your compose mounts are:

```yaml
- /mnt/nas/media:/media:ro
- /mnt/nas/downloads:/downloads
```

then add libraries in the web UI using container paths:

| Type | Recommended path |
| --- | --- |
| Movie library | `/media/电影` |
| TV / anime / variety library | `/media/电视剧` |
| Adult | `/media/Adult` |
| Download root | `/downloads` |

#### NAS Absolute Path Syntax

On NAS systems, media folders are usually absolute host paths. In compose, use paths starting from the filesystem root, such as `/your-nas/...`, `/volume1/...`, or `/mnt/...`; do not use `./your-nas/...` unless that directory really exists under the compose project directory.

```yaml
# Correct: absolute host paths
- /your-nas/media:/media:ro
- /your-nas/downloads:/downloads

# Wrong: relative to the compose directory
- ./your-nas/media:/media:ro
- ./your-nas/downloads:/downloads
```

You can also keep the paths in `.env`:

```env
MEDIASTATION_MEDIA_DIR=/your-nas/media
MEDIASTATION_DOWNLOAD_DIR=/your-nas/downloads
```

Inside MediaStationGo, add `/media/电影` and `/media/电视剧` as media library roots. Organized files will land in category folders such as `/media/电影/动画电影` and `/media/电视剧/国产剧`. Use `/downloads` as the download root; subscriptions will save to folders such as `/downloads/动画电影` and `/downloads/国产剧`.

### External Access and v2rayA

If the NAS already uses v2rayA `redirect` / transparent proxy with routing rules, MediaStationGo usually does not need explicit `HTTP_PROXY` / `HTTPS_PROXY` variables.

Avoid stacking proxy variables in `.env`, `docker-compose.yml`, or the Docker daemon unless you intentionally use an application-level HTTP proxy. Stacked proxies can break GHCR pulls and site APIs.

Test from inside the container first:

```bash
docker exec -it mediastation-go sh -lc 'wget -S -O- --timeout=20 https://api.m-team.cc/api/torrent/search'
```

If it still hangs during TLS, check v2rayA redirect rules and whether Docker bridge traffic is covered by the transparent proxy instead of adding another `HTTP_PROXY` layer.

### qBittorrent Connection

If qBittorrent runs on the same NAS/host, do not use `127.0.0.1` from MediaStationGo; inside the container it means the MediaStationGo container itself. In Download Clients, use:

```text
http://host.docker.internal:8085
```

The default `docker-compose.yml` includes:

```yaml
extra_hosts:
  - "host.docker.internal:host-gateway"
```

If `http://192.168.1.125:8085` times out but `http://172.17.0.1:8085` returns 403, the container can reach qBittorrent but the WebUI rejects login. Check username/password, IP bans, CSRF/Host Header validation, and allowed subnets/domains.

Quick test from the MediaStationGo container:

```bash
docker exec -it mediastation-go sh -lc 'wget -S -O- --post-data="username=YOUR_USER&password=YOUR_PASS" http://host.docker.internal:8085/api/v2/auth/login'
```

`Ok.` means the connection is healthy. `Forbidden` / `403` means qBittorrent WebUI security settings or credentials still need adjustment.

### Download Client Paths

If qBittorrent also runs in Docker, make sure qBittorrent and MediaStationGo share the same host directory and use consistent container paths.

Recommended mapping:

```text
Host: /mnt/nas/downloads
MediaStationGo container: /downloads
qBittorrent container: /downloads
```

Recommended subscription save root:

```text
/downloads
```

With smart classification enabled, subscription and site-search downloads are saved to category folders such as:

```text
/downloads/动画电影
/downloads/华语电影
/downloads/外语电影
/downloads/国产剧
/downloads/国漫
/downloads/日番
/downloads/欧美剧
/downloads/日韩剧
/downloads/综艺
```

---

## 🐳 Docker Compose Configuration

The repository includes a heavily commented `docker-compose.yml`. Common variables:

| Variable | Default | Description |
| --- | --- | --- |
| `MEDIASTATION_IMAGE_TAG` | `latest` | Image tag. Pin a release for production. |
| `MEDIASTATION_HTTP_PORT` | `18080` | Host access port |
| `MEDIASTATION_DATA_DIR` | `./data` | Persistent data directory |
| `MEDIASTATION_CACHE_DIR` | `./cache` | Image and transcoding cache |
| `MEDIASTATION_MEDIA_DIR` | `./media` | Host media library root; on NAS use an absolute path such as `/your-nas/media` |
| `MEDIASTATION_MEDIA_CONTAINER_DIR` | `/media` | Container media path; set it equal to `MEDIASTATION_MEDIA_DIR` if you want to enter `/your-nas/...` directly in the web UI |
| `MEDIASTATION_DOWNLOAD_DIR` | `./downloads` | Host download target; on NAS use an absolute path such as `/your-nas/downloads` |
| `MEDIASTATION_DOWNLOAD_CONTAINER_DIR` | `/downloads` | Container download path; set it equal to `MEDIASTATION_DOWNLOAD_DIR` if your downloader save path should also be `/your-nas/...` |
| `PUID` / `PGID` | `1000` / `1000` | Linux/NAS file permission mapping |
| `TZ` | `Asia/Shanghai` | Container timezone |

View logs:

```bash
docker logs -f mediastation-go
```

Update:

```bash
curl -fsSL https://cdn.jsdelivr.net/gh/ShukeBta/MediaStationGo@main/scripts/docker-compose-update.sh -o docker-compose-update.sh
chmod +x docker-compose-update.sh
./docker-compose-update.sh
```

Note: plain `docker compose pull && docker compose up -d` switches to the new image but does not remove old images. The helper keeps the currently running MediaStationGo image, removes unused older images from the same repository, and runs `docker image prune -f` for dangling layers. To aggressively remove all unused images, run `PRUNE_ALL_UNUSED=1 ./docker-compose-update.sh`.

Stop:

```bash
docker compose down
```

Back up data:

```bash
tar -czf mediastationgo-data-backup.tgz ./data
```

---

## 🖥️ One-Click Deployment Scripts

If you do not want Docker, run MediaStationGo directly on the host. The scripts build the frontend, compile the backend, start the service, and verify `/api/health`.

### Linux / macOS

```bash
git clone https://github.com/ShukeBta/MediaStationGo.git
cd MediaStationGo
chmod +x scripts/deploy.sh
PORT=18080 DATA_DIR=/opt/mediastation/data CACHE_DIR=/opt/mediastation/cache ./scripts/deploy.sh
```

### Windows PowerShell

```powershell
git clone https://github.com/ShukeBta/MediaStationGo.git
cd MediaStationGo
.\scripts\deploy.ps1 -Port 18080 -DataDir D:\MediaStationGo\data -CacheDir D:\MediaStationGo\cache
```

The scripts will:

1. Install frontend dependencies and build `web/dist`
2. Compile the Go backend into `bin/`
3. Create data and cache directories
4. Stop any previous process and start a new one
5. Verify the service through `/api/health`

---

## 🧩 Release Package Deployment

Each release provides multi-platform archives:

| Platform | Package example |
| --- | --- |
| Linux x86_64 | `MediaStationGo-v0.0.32-linux-amd64.tar.gz` |
| Linux ARM64 | `MediaStationGo-v0.0.32-linux-arm64.tar.gz` |
| Windows x86_64 | `MediaStationGo-v0.0.32-windows-amd64.zip` |
| macOS Intel | `MediaStationGo-v0.0.32-darwin-amd64.tar.gz` |
| macOS Apple Silicon | `MediaStationGo-v0.0.32-darwin-arm64.tar.gz` |

Linux example:

```bash
tar -xzf MediaStationGo-v0.0.32-linux-amd64.tar.gz
cd MediaStationGo-v0.0.32-linux-amd64
MEDIASTATION_APP_PORT=18080 ./mediastation-go
```

Windows example:

```powershell
Expand-Archive .\MediaStationGo-v0.0.32-windows-amd64.zip
cd .\MediaStationGo-v0.0.32-windows-amd64
$env:MEDIASTATION_APP_PORT = "18080"
.\mediastation-go.exe
```

> Release binaries listen on `8080` by default. Set `MEDIASTATION_APP_PORT=18080` as shown above if you want the same port as the Docker examples.

---

## 🛠️ Local Development

### Requirements

| Component | Version | Purpose |
| --- | --- | --- |
| Go | 1.25+ | Backend build and tests |
| Node.js | 20+ | Frontend build |
| FFmpeg / ffprobe | Recommended | Media probing and transcoding |
| Docker | Optional | Container deployment and multi-arch builds |
| qBittorrent | Optional | Download integration testing |

### Build Locally

```bash
cp config.example.yaml config.yaml
cd web
npm ci
npm run build
cd ..
go build -o bin/mediastation-go ./cmd/server
./bin/mediastation-go
```

Windows:

```powershell
Copy-Item config.example.yaml config.yaml
Set-Location web
npm ci
npm run build
Set-Location ..
go build -o bin\mediastation-go.exe .\cmd\server
.\bin\mediastation-go.exe
```

### Common Commands

```bash
make build       # Build frontend and backend
make test        # Run Go tests
make smoke       # Smoke test
make docker      # docker compose up -d
make deploy      # Linux one-click deploy
make docker-push # Multi-arch buildx push
```

---

## 🏗️ Repository Layout

```text
MediaStationGo/
├── cmd/server/                 # Server entry point
├── internal/
│   ├── config/                  # Config loading and defaults
│   ├── database/                # SQLite initialization and migrations
│   ├── handler/                 # HTTP API, Emby API, admin endpoints
│   ├── middleware/              # Auth, permission, logging middleware
│   ├── model/                   # GORM models
│   ├── repository/              # Data access layer
│   └── service/                 # Scanner, scraper, playback, downloads, subscriptions
├── web/
│   ├── public/                  # favicon and static public assets
│   ├── src/                     # React pages, components, API clients, stores
│   └── dist/                    # Frontend build output, ignored by git
├── scripts/                     # Deploy, package, Docker scripts
├── docs/                        # Design docs, screenshots, architecture notes
├── docker-compose.yml           # Default Docker Compose deployment
├── Dockerfile                   # Multi-stage image build
├── config.example.yaml          # Config template
└── README.md / README_EN.md     # Documentation
```

---

## ⚙️ Configuration

Configuration precedence, from low to high:

1. Built-in defaults
2. `config.yaml`
3. `config/*.yaml`
4. `MEDIASTATION_` environment variables
5. Runtime settings stored in the database

Common variables:

| Variable | Default | Description |
| --- | --- | --- |
| `MEDIASTATION_APP_HOST` | `0.0.0.0` | Listen address |
| `MEDIASTATION_APP_PORT` | `8080` | Listen port |
| `MEDIASTATION_APP_WEB_DIR` | `./web/dist` | Frontend static bundle |
| `MEDIASTATION_APP_DATA_DIR` | `./data` | App data directory |
| `MEDIASTATION_DATABASE_DB_PATH` | `./data/mediastation.db` | SQLite database path |
| `MEDIASTATION_CACHE_CACHE_DIR` | `./cache` | Image/transcode cache |
| `MEDIASTATION_SECRETS_JWT_SECRET` | Auto-generated | JWT and encrypted settings seed |
| `MEDIASTATION_APP_CORS_ORIGINS` | empty | Extra CORS origins |
| `MEDIASTATION_TELEGRAM_API_BASE_URL` | `https://api.telegram.org` | Telegram Bot API base URL; use a reverse proxy when Telegram is blocked or slow |
| `MEDIASTATION_TELEGRAM_PROXY_URL` | empty | Telegram outbound proxy, e.g. `http://172.17.0.1:7890` or `socks5://172.17.0.1:1080` |

Runtime settings from the admin UI:

- API keys: TMDb, Bangumi, TheTVDB, Fanart, OpenAI Compatible.
- Sites: M-Team, NexusPHP, Unit3D, custom RSS.
- Download clients: qBittorrent, Transmission, Aria2.
- Notifications: Telegram, Bark, Webhook, Email. Telegram channels can use a per-channel API base URL or proxy, and test errors redact Bot Tokens automatically.
- Playback profiles, permissions, scheduler tasks, storage settings.

---

## 👥 Users and Permissions

- The default administrator is created on first startup as `admin / admin123`. This account can be renamed, but it cannot be deleted or demoted and always keeps the highest privileges.
- The open-source edition allows up to 20 users by default to reduce abuse on home NAS or public test instances. Binding a private license server can raise the quota according to the activated license policy.
- Users created from the admin panel are “viewer users” by default: they can log in through the Web UI and Emby-compatible clients, browse libraries, play media, use external players, favorite items, and keep watch history.
- Viewer users cannot scan libraries, rescrape metadata, delete media, probe media tracks, export NFO files, manage files, manage STRM links, manage download clients, create download tasks, or create/run subscriptions.
- Because playback necessarily streams media data to the client, MediaStationGo can block download-management features and torrent/download tasks, but it cannot fully prevent an authorized browser or external player from saving an already authorized stream at the protocol level.

---

## 🔐 Private License Server

MediaStationGo includes a server-side bridge for the private standalone `MediaStationLicenseServer`:

- License server: `ShukeBta/MediaStationLicenseServer`; a local backup may live at `C:\Users\Administrator\WorkBuddy\license_server_backup`.
- MediaStationGo exposes `/api/license/activate`, `/api/license/status`, and `/api/license/heartbeat`; these backend routes proxy the License Server and do not expose the HMAC secret to browsers.
- License Server public endpoints are `/api/v1/activate`, `/api/v1/status/:fingerprint`, and `/api/v1/heartbeat`.
- Configure `license.server_url` and `license.hmac_secret` under Settings → License Server, then bind a key on the License page.
- Without a valid license, MediaStationGo stays in open-source mode. With a valid license, the current implementation raises the user quota to the licensed tier.

Example environment variables:

```bash
MEDIASTATION_LICENSE_SERVER_URL=http://127.0.0.1:8001
MEDIASTATION_LICENSE_HMAC_SECRET=must-match-LICENSE_HMAC_SECRET
```

---

## 🎞️ On-Demand FFmpeg / ffprobe

MediaStationGo does not keep `ffmpeg` or `ffprobe` running as resident daemons. They are launched only when needed:

- `ffprobe` runs during library scanning or manual media-track probing.
- `ffmpeg` runs when browser direct play is not suitable and HLS transcoding is required.
- Admin tool-status checks or manual tool installation may briefly execute version checks/install logic.

When playback stops, a transcode job is cancelled, or the service shuts down, the corresponding transcoding process is stopped. If there is no scanning, probing, or transcoding, `ffmpeg/ffprobe` should not continuously consume CPU.

The default HLS profile is NAS-friendly: `MEDIASTATION_TRANSCODER_ENABLED=true` is the global switch, and disabling it prevents ffmpeg transcode jobs from starting; `MEDIASTATION_TRANSCODER_HARDWARE_ACCEL=false` is the hardware acceleration switch, and hardware encoders are used only when it is enabled together with `MEDIASTATION_TRANSCODER_ENCODER=nvenc/qsv/vaapi`; `MEDIASTATION_TRANSCODER_REALTIME=true` throttles input to playback speed, `MEDIASTATION_TRANSCODER_THREADS=2` caps software encoding threads, `MEDIASTATION_TRANSCODER_MAX_CONCURRENT=1` limits simultaneous transcodes, and `MEDIASTATION_TRANSCODER_IDLE_TIMEOUT_SECONDS=120` stops ffmpeg after the player stops requesting segments.

---

## 🔍 Metadata Strategy

MediaStationGo avoids unnecessary repeated scraping and tries not to overwrite good local metadata:

1. Read local NFO, poster, fanart, season poster, and episode images first.
2. Parse media type, title, year, season, and episode from file paths.
3. Fill missing data through TMDb, TheTVDB, Bangumi, and Douban.
4. Use Fanart.tv for higher-quality artwork when available.
5. Adult content prefers local NFO/images, then enriches from public JavBus/JavDB pages.
6. Existing local metadata is not blindly overwritten.

Recommended layout:

```text
/media/Movies/Inception (2010)/Inception (2010).mkv
/media/TV/Some Show/Season 01/Some Show S01E01.mkv
/media/Anime/Anime Title/Season 01/Anime Title S01E01.mkv
/media/Variety/Show Name/Season 2026/Show Name S2026E01.mkv
/media/Adult/ABCD-123/ABCD-123.mp4
```

Common local artwork names:

```text
poster.jpg
fanart.jpg
folder.jpg
season01-poster.jpg
S01E01-thumb.jpg
movie.nfo
tvshow.nfo
episode.nfo
```

### Smart Classification Directory Rules

MediaStationGo separates download classification from final media organization:

1. Download stage: subscriptions and site-search downloads are saved under the downloader root by category.
2. Organization stage: manual or automatic organization moves files into the media library root, then into type and category folders.

Recommended host directories:

```text
/your-nas/downloads
/your-nas/media/电影
/your-nas/media/电视剧
```

Container paths:

```text
/downloads
/media/电影
/media/电视剧
```

Download classification examples:

```text
/downloads/动画电影
/downloads/国产剧
/downloads/国漫
/downloads/华语电影
/downloads/日番
/downloads/外语电影
/downloads/综艺
```

Organized media examples:

```text
/media/电视剧/国产剧/Show Name (2026)/Season 01/Show Name - S01E01 - 第 1 集.mkv
/media/电视剧/国漫/Anime Name (2026)/Season 01/Anime Name - S01E01 - 第 1 集.mkv
/media/电视剧/欧美剧/Show Name (2026)/Season 01/Show Name - S01E01 - 第 1 集.mkv
/media/电视剧/日番/Anime Name (2026)/Season 01/Anime Name - S01E01 - 第 1 集.mkv
/media/电视剧/日韩剧/Show Name (2026)/Season 01/Show Name - S01E01 - 第 1 集.mkv
/media/电视剧/综艺/Variety Name (2026)/Season 2026/Variety Name - S2026E01 - 第 1 集.mp4
/media/电影/动画电影/Movie Name (2026)/Movie Name (2026) - 1080p.mkv
/media/电影/华语电影/Movie Name (2026)/Movie Name (2026) - 1080p.mkv
/media/电影/外语电影/Movie Name (2026)/Movie Name (2026) - 1080p.mkv
```

If the library root is set directly to `/media`, the organizer automatically adds the `电影/` or `电视剧/` type folder when smart classification is enabled. If the library root is already `/media/电影` or `/media/电视剧`, it will not add the type folder again.

Automatic and manual organization are separate switches:

- `organizer.smart_classify`: controls smart category folders only.
- `organizer.auto_after_download` / `organize.auto`: controls whether completed downloads are organized automatically.
- If auto organization is disabled, use the Tools page to organize a library or a single media item manually.

### Organization and Scraping Naming Templates

Use separate organization templates by media type. TV shows, anime, and variety shows should keep title, year, season folder, season/episode number, and episode title. Movies should keep title, year, part marker, and video format.

Recommended template for TV / anime / variety:

```jinja
{{title}}{% if year %} ({{year}}){% endif %}/Season {{season}}/{{title}} - {{season_episode}}{% if part %}-{{part}}{% endif %}{% if episode %} - 第 {{episode}} 集{% endif %}{{fileExt}}
```

Example output:

```text
Some Show (2024)/Season 01/Some Show - S01E01 - 第 1 集.mkv
Some Anime (2025)/Season 02/Some Anime - S02E03 - 第 3 集.mkv
Some Variety (2026)/Season 2026/Some Variety - S2026E01 - 第 1 集.mp4
```

Recommended template for movies:

```jinja
{{title}}{% if year %} ({{year}}){% endif %}/{{title}}{% if year %} ({{year}}){% endif %}{% if part %}-{{part}}{% endif %}{% if videoFormat %} - {{videoFormat}}{% endif %}{{fileExt}}
```

Example output:

```text
Inception (2010)/Inception (2010) - 1080p.mkv
Dune (2021)/Dune (2021)-CD1 - 2160p.mkv
```

Common variables:

| Variable | Description |
| --- | --- |
| `title` | Media title, preferably from local NFO or online metadata |
| `year` | Year, appended when available |
| `season` | Season number, used for `Season 01` folders |
| `season_episode` | Season/episode code, such as `S01E01` or `S2026E01` |
| `episode` | Episode number, used for Chinese episode titles |
| `part` | Part marker, such as `CD1` or `Part1` |
| `videoFormat` | Video format, such as `1080p`, `2160p`, or `WEB-DL` |
| `fileExt` | Original file extension, such as `.mkv` or `.mp4` |

---

## 🔎 Discovery, Search & Subscriptions

### Multi-source discovery

Discover supports:

- TMDb: trending, popular movies, popular TV, top-rated movies.
- Douban: hot movies, top movies, hot TV.
- Bangumi: calendar and anime entries.

### Smart search

Smart search can combine:

- Existing local library content.
- Online results from TMDb, Douban, and Bangumi.
- Subscription keywords and media types.

### Subscription rules

| Rule | Description |
| --- | --- |
| Media type | Movie, TV, anime, variety, or auto-detect |
| Search mode | Keyword or IMDB ID |
| Resolution | Best, 2160p, 1080p, 720p |
| Quality | REMUX, BluRay, WEB-DL, HDTV |
| Effects | HDR, Dolby Vision, Atmos |
| Release groups | Preferred release groups |
| Exclude words | Filter CAM, TS, low-quality releases |
| Wash | Disabled by default; can prioritize resolution, quality, effects, or seeders |

Download and subscription cards show only safe display metadata such as title, poster, speed, progress, and size. Raw torrent URLs are hidden to prevent tracker token leaks in multi-user deployments.

---

## 🔌 External Clients & Emby Compatibility

MediaStationGo exposes Emby/Jellyfin-style APIs for clients such as:

- Infuse
- VidHub
- SenPlayer
- Other Emby/Jellyfin-compatible players

Server URL:

```text
http://<server-ip>:18080
```

If a client cannot connect, check:

1. Docker port mapping, typically `18080:8080`.
2. Firewall access from LAN to port `18080`.
3. Username/password.
4. Reverse proxy handling of `/api`, video streams, and Range requests.

### Functional Reference to MoviePilot

For external-client compatibility and media workflow integration, MediaStationGo references MoviePilot's mature product direction: a unified media library, subscription downloads, post-download organization, and Emby/Jellyfin-compatible APIs that connect the Web management interface with clients such as Infuse, VidHub, and SenPlayer. MediaStationGo does not aim to replace Emby/Jellyfin; instead, it provides the common browsing, playback, poster wall, season/episode hierarchy, progress tracking, and external-client access capabilities inside a lightweight Go service.

Current compatibility focus:

- Library, collection, season, and episode hierarchy output.
- Basic metadata output such as posters, backdrops, descriptions, year, and ratings.
- Stream URLs, HTTP Range support, playback progress, and resume.
- Emby/Jellyfin-style endpoints required for external-client login, browsing, and playback.

Still being improved:

- More complete Emby/Jellyfin device capability negotiation.
- More detailed transcoding profiles and subtitle capability declarations.
- Multi-user permissions, library filtering, and playback-history synchronization.
- Closed-loop integration with subscriptions, post-download organization, and upgrade rules.

> MoviePilot is licensed under GPL-3.0. MediaStationGo references its public product ideas and interaction patterns only, and does not copy private data, secrets, tracker accounts, or incompatible implementations.

---

## 🧠 AI and External Services

Configure external services in the admin UI:

| Service | Purpose |
| --- | --- |
| TMDb | Movie/TV posters, backdrops, descriptions |
| Bangumi | Anime and Chinese anime metadata |
| TheTVDB | Additional TV/season/episode metadata |
| Fanart.tv | High-quality logos and artwork |
| Douban | Chinese movie/TV search and recommendation supplement |
| OpenAI Compatible | AI search, recommendations, operations assistant |

For M-Team, use an API Access Token:

```text
Control Panel → Lab → Access Token
HTTP Header: x-api-key
```

Avoid using cookies for open API calls to reduce account risk.

---

---

## 🔐 Privacy and Safety

The repository ignores personal/runtime data by default:

- `data/`, `cache/`, `logs/`
- `.tmp-deploy-data/`, `.tmp-deploy-server.*`
- `.mediastation.pid`
- `config.yaml`, `.env*`
- `*.db`, `*.db-wal`, `*.log`
- `web/dist/`, `node_modules/`, `bin/`
- API keys, cookies, tokens, passwords, certificates, and secret files

Before pushing, run:

```bash
git status --short
git ls-files | grep -E 'data/|cache/|\.db|\.log|jwt_secret|config.yaml|\.env|token|apikey|password' || true
```

---

## ❓ FAQ

### Pulling the GHCR image fails with `EOF`.

`EOF` usually means the connection from your server/NAS to GHCR was interrupted. It is normally a network or registry connectivity issue, not a compose syntax issue. Try:

```bash
# 1. Clear any stale GHCR login state
docker logout ghcr.io || true

# 2. Pull the image directly
docker pull ghcr.io/shukebta/mediastation-go:latest

# 3. On x86_64/AMD64 hosts, retry with an explicit platform
docker pull --platform linux/amd64 ghcr.io/shukebta/mediastation-go:latest

# 4. Start after the pull succeeds
docker compose up -d
```

If the server is behind a restricted network, configure a Docker daemon proxy that can reach GHCR. A shell-only proxy is often not inherited by the Docker service. The default compose file uses `pull_policy: missing` to avoid contacting GHCR on every container restart.

If your media path is an absolute NAS path, use `/your-nas/...` instead of `./your-nas/...`; the latter is relative to the compose directory.

### The Docker deployment starts but the browser cannot open the site.

Check container status and logs:

```bash
docker ps
docker logs -f mediastation-go
```

Use the host port, usually `http://<server-ip>:18080`.

### External clients report that the server does not respond.

Check firewall rules, Docker port mappings, reverse proxy configuration, and LAN access to `18080`. The container listens on `8080`; the host default is `18080`.

### Posters are missing.

Check:

1. Local `poster.jpg`, `fanart.jpg`, and NFO files.
2. TMDb / Bangumi / Douban connectivity.
3. Proxy settings if the host is behind a restricted network.
4. Whether file names contain clear title, year, season, and episode information.

### Why are raw download URLs hidden?

PT download URLs often include private tokens. Download and subscription views intentionally hide raw URLs and only show safe metadata such as title, poster, speed, progress, and size.

### Which Docker package should be kept?

Keep `ghcr.io/shukebta/mediastation-go`. The old `mediastationgo` package can be removed to avoid users pulling the wrong image.

---

## 🗺️ Roadmap

- Broader Emby/Jellyfin client compatibility.
- Stronger adult metadata handling from local files and public pages.
- More granular subscription wash and post-download organization rules.
- Better mobile and TV interaction patterns.
- Plugin-style site adapters and notification providers.
- More end-to-end tests and automated screenshot generation.

---

## 🤝 Contributing

Issues, pull requests, site adapters, scraping rules, UI improvements, and documentation fixes are welcome.

Before submitting changes, please run:

```bash
go test ./...
cd web && npm run build
```

---

## 👥 Developer Group

- Telegram: <https://t.me/MediaStationGo>

---

## 🍜 Donation

If MediaStationGo saves you time, feel free to buy the author a bowl of noodles.

<img width="200" height="200" alt="WeChat Donation QR" src="https://github.com/user-attachments/assets/d6077de5-8305-400d-8b82-470ef05d926e" />

---

## ⭐ Star History

<a href="https://www.star-history.com/?repos=ShukeBta%2FMediaStationGo&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=ShukeBta/MediaStationGo&type=date&theme=dark&legend=top-left" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=ShukeBta/MediaStationGo&type=date&legend=top-left" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=ShukeBta/MediaStationGo&type=date&legend=top-left" />
 </picture>
</a>

---

## 📄 License and Non-Commercial Statement

This project uses `GPL-3.0` as its base license. See [LICENSE](LICENSE) for details. The maintainers also state and request the following usage boundary:

- The project is intended for personal learning, home NAS, self-hosted media, non-commercial research, and community collaboration.
- Without explicit written permission from the author, do not use this project or derivative versions for commercial resale, paid hosting, paid SaaS, pre-installed commercial devices, closed-source redistribution, or other profit-oriented commercial use.
- For commercial cooperation, enterprise deployment, custom development, integrated redistribution, or commercial authorization, contact the author first to confirm the authorization scope.
- If there is any interpretive difference between the README non-commercial statement and the formal `GPL-3.0` license text, the code license is governed by [LICENSE](LICENSE); commercial usage should additionally obtain author permission.

---

<p align="center">Made with ❤️ by ShukeBta</p>
