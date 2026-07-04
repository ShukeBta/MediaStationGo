# MediaStationGo

<p align="center">
  <img src="web/public/brand/mgo-emby-icon.svg" width="96" height="96" alt="MediaStationGo Logo" />
</p>

<h3 align="center">A lightweight, polished, NAS-friendly private media center</h3>

<p align="center">
  <strong>Docker-first setup · Multi-user management · Media library · Metadata · Downloads · Emby-protocol clients · Cloud playback</strong>
</p>

<p align="center">
  <a href="README.md">中文</a> ·
  <a href="#quick-start">Quick Start</a> ·
  <a href="#docker-compose-recommended">Docker Compose</a> ·
  <a href="#faq">FAQ</a> ·
  <a href="https://mgo.3jzs.com">Live Demo</a>
</p>

<p align="center">
  <img alt="Go" src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat-square&logo=go&logoColor=white" />
  <img alt="React" src="https://img.shields.io/badge/React-18-61DAFB?style=flat-square&logo=react&logoColor=111827" />
  <img alt="Docker" src="https://img.shields.io/badge/Docker-ready-2496ED?style=flat-square&logo=docker&logoColor=white" />
  <img alt="License" src="https://img.shields.io/badge/License-GPL--3.0-blue?style=flat-square" />
</p>

---

## What is it?

MediaStationGo is a self-hosted media center for personal libraries, home NAS, and home-theater users.

It helps you:

- Manage movies, TV shows, anime, variety shows, music, and adult libraries.
- Create multiple user accounts for family members, friends, or different devices.
- Scan files and enrich posters, summaries, years, seasons, and episodes.
- Play in the web UI, or log in with a MediaStationGo account from Emby-protocol apps such as Infuse, VidHub, SenPlayer, and Emby clients.
- Connect qBittorrent for search, subscriptions, downloads, and post-download organization.
- Connect OpenList, CloudDrive2, WebDAV, and other storage backends with STRMURL or 302 redirect playback.
- Run on NAS, mini PCs, VPS, Linux, Windows Docker Desktop, or any Docker-friendly host.

> The project is moving fast. With the default PostgreSQL deployment, back up both `data/` and `postgres/`.

---

## Key Highlights

- **One server, many clients**: deploy MediaStationGo once; you do not need to run a separate Emby server.
- **Emby-protocol compatibility**: add the server in third-party players as an Emby/Jellyfin-compatible server, then log in with your MediaStationGo username and password.
- **Multi-user management**: supports admins, regular users, account enable/disable, expiry dates, device management, Bot registration, and redeem codes.
- **Local + cloud media in one place**: manage local disks, download folders, OpenList, CloudDrive2, WebDAV, and other storage backends from one panel.
- **Download-to-library workflow**: connect qBittorrent for search, subscriptions, download completion organization, and metadata matching.
- **NAS-friendly**: simple Docker Compose deployment. The primary database lives under `postgres/`, while runtime secrets and files live under `data/`.

---

## Who is it for?

- **Beginners** who want to edit one `docker-compose.yml` and start the service.
- **NAS users** who want a low-resource media center for local disks and cloud storage.
- **PT/download users** who want downloads, organization, metadata, and playback in one panel.
- **External-player users** who want to log in to Emby-protocol third-party apps with one MediaStationGo account.
- **Family-sharing users** who want separate user accounts without deploying a separate media server for each person.
- **Developers** who want to study or extend a Go + React self-hosted media app.

---

## Live Demo

- URL: [https://mgo.3jzs.com](https://mgo.3jzs.com)
- Username: `admin`
- Password: `admin123`

> The demo is for feature preview only. Do not save private API keys, tracker cookies, or personal data there.

---

## Quick Start

Docker Compose is the recommended path. Beginners do not need `.env`, bare-metal binaries, or source builds. Use the single-image SQLite template if you want the smallest possible setup.

```bash
mkdir -p MediaStationGo
cd MediaStationGo
# Simplest option: one MediaStationGo container + SQLite
curl -fsSL https://raw.githubusercontent.com/ShukeBta/MediaStationGo/main/docker-compose.simple.yml -o docker-compose.yml
# Or tier 1: MediaStationGo + PostgreSQL
# curl -fsSL https://raw.githubusercontent.com/ShukeBta/MediaStationGo/main/docker-compose.yml -o docker-compose.yml
```

Edit `docker-compose.yml`:

```bash
vi docker-compose.yml
```

Start:

```bash
docker compose up -d
```

Open:

```text
http://SERVER_IP:18080
```

Default login:

```text
Username: admin
Password: admin123
```

---

## Docker Compose Recommended

The repository `docker-compose.yml` is the lightweight recommended template: no `.env` required, and by default it only starts `MediaStationGo + PostgreSQL`. This is the best starting point for most NAS users.

If you already have an older `./data/mediastation.db`, the first start with the new compose file automatically imports it into PostgreSQL. Keep `./data`; it still stores the JWT secret, runtime data, and the old SQLite migration source.

### Deployment modes

| Mode | Command | Best for |
| --- | --- | --- |
| Single image: SQLite | `docker compose -f docker-compose.simple.yml up -d` | Beginners and single-user setups that want one image only, no PostgreSQL/Redis |
| Lightweight: PG only | `docker compose up -d` | Most NAS devices, lowest resource use |
| Standard: PG + Redis | `docker compose -f docker-compose.standard.yml up -d` | Multi-user use and frequent Emby client refreshes |
| Search enhanced: PG + Redis + OpenSearch | `docker compose -f docker-compose.search.yml up -d` | Huge libraries and future standalone search indexing |

Each compose file is standalone. Do not stack multiple `-f` files together.

The single-image `docker-compose.simple.yml` runs only MediaStationGo with a built-in SQLite database — the simplest starting point. Do not set `MEDIASTATION_DATABASE_DSN` there, or it switches back to PostgreSQL. Move up to the PostgreSQL modes for multi-user or high-concurrency use (keep `./data` when you switch). Redis and OpenSearch are enhancement layers, not source databases. Do not enable OpenSearch by default on low-memory NAS devices.

### Database Choice And Disabling SQLite

The current Docker Compose setup uses PostgreSQL by default. SQLite is no longer the primary database in the recommended Docker deployment. The runtime database is controlled by:

```yaml
environment:
  MEDIASTATION_DATABASE_TYPE: postgres
  MEDIASTATION_DATABASE_DSN: postgres://mediastation:mediastation@postgres:5432/mediastation?sslmode=disable
```

`MEDIASTATION_DATABASE_DB_PATH` is only used as a one-time migration source for old SQLite data:

- Fresh installs: `docker compose up -d` uses PostgreSQL and does not create a new SQLite primary database.
- Upgrades: if `./data/mediastation.db` exists, the first start with the new compose file imports it into PostgreSQL.
- Migration fills missing rows by primary key and skips rows that already exist. If it fails partway through, a later start continues the remaining tables.
- After a successful import, PostgreSQL gets a completion marker in the `settings` table, so the old SQLite file is not imported again.
- Redis is a hot cache and OpenSearch is a search index; neither is a source database.

Recommended SQLite to PostgreSQL upgrade flow:

```bash
docker compose pull mediastation-go
docker compose up -d --no-deps mediastation-go
docker compose logs -f mediastation-go
```

After you see `sqlite data migrated to postgres`, or after the web UI shows your users, libraries, and settings correctly, you can stop using the old SQLite file as a migration source.

To make the deployment PostgreSQL-only after migration, keep PostgreSQL selected and point the old SQLite migration path at a non-existent file:

> Only do this after the web UI confirms that users, libraries, settings, and media rows are already present in PostgreSQL.

```yaml
environment:
  MEDIASTATION_DATABASE_TYPE: postgres
  MEDIASTATION_DATABASE_DSN: postgres://mediastation:mediastation@postgres:5432/mediastation?sslmode=disable
  MEDIASTATION_DATABASE_DB_PATH: /data/disabled-sqlite-migration.db
```

Then rename or move the old host-side SQLite file as an offline backup:

```bash
mv data/mediastation.db data/mediastation.sqlite.bak
```

For bare-metal or custom `config.yaml` deployments, use the same idea:

```yaml
database:
  type: postgres
  dsn: postgres://mediastation:mediastation@127.0.0.1:5432/mediastation?sslmode=disable
  db_path: ""
```

Do not delete `./postgres`. After migration, it is the real primary database. Keep `./data` too, because it stores the JWT secret and runtime files.

### Choose an image source

Both image sources are supported. Pick one and put it in `image:`:

| Source | Image | Best for |
| --- | --- | --- |
| GitHub Container Registry (GHCR) | `ghcr.io/shukebta/mediastation-go:latest` | Recommended default, follows repository releases |
| Docker Hub | `shukbet/mediastationgo:latest` | Backup source when GHCR is slow or unavailable |

To pin a version, first confirm the tag exists on the repository Packages page. Use this format:

```yaml
image: ghcr.io/shukebta/mediastation-go:<version-tag>
# If GHCR does not have that tag, use Docker Hub as the backup:
# image: shukbet/mediastationgo:MediaStationGo-v0.0.72
```

For the simplest setup, keep GHCR `latest`.

Manual pull examples:

```bash
# GitHub Container Registry
docker pull ghcr.io/shukebta/mediastation-go:latest

# Docker Hub backup
docker pull shukbet/mediastationgo:latest
```

Focus on this part:

```yaml
volumes:
  - ./data:/data
  - ./cache:/cache
  - ./media:/media
  - ./downloads:/downloads
```

Meaning:

| Host path | Container path | Purpose |
| --- | --- | --- |
| `./data` | app `/data` | Settings, JWT secret, old SQLite migration source; the primary DB is under `./postgres` |
| `./cache` | app `/cache` | Cache; safe to clean when needed |
| `./media` | `/media` | Media libraries; use `/media/...` in the web UI |
| `./downloads` | `/downloads` | Download directory and organization source |
| `./postgres` | PostgreSQL `/var/lib/postgresql/data` | New default primary database; back this up |
| `./redis` | Redis `/data` | Used only in standard mode; hot cache, rebuildable |
| `./opensearch` | OpenSearch `/usr/share/opensearch/data` | Used only in search-enhanced mode; higher memory use |

If your NAS paths are:

```text
/vol1/1000/Media
/vol1/1000/Downloads
```

change the compose file to:

```yaml
volumes:
  - ./data:/data
  - ./cache:/cache
  - /vol1/1000/Media:/media
  - /vol1/1000/Downloads:/downloads

environment:
  MEDIASTATION_MEDIA_DIR: /vol1/1000/Media
  MEDIASTATION_DOWNLOAD_DIR: /vol1/1000/Downloads
```

Rules:

- The left side of `volumes` is the real path on your host/NAS.
- The right side is the container path. Keep `/media` and `/downloads` unless you know why you are changing them.
- In the web UI, create libraries with container paths such as `/media/Movies` or `/media/TV`.
- Do not write NAS absolute paths as `./vol1/...`; `./` means a folder under the current compose directory.
- On Windows Docker Desktop, paths like `D:/Media:/media` and `D:/Downloads:/downloads` are fine.
- If you only scan/play existing media and never organize into the library, you may add `:ro`; if you use organize/rename/ingest, the media mount must stay writable.

### Minimal compose example

The root `docker-compose.yml` follows this style:

```yaml
services:
  mediastation-go:
    # Pick one image source:
    # GitHub Container Registry (GHCR):
    image: ghcr.io/shukebta/mediastation-go:latest
    # Docker Hub backup:
    # image: shukbet/mediastationgo:latest

    restart: unless-stopped
    init: true
    depends_on:
      postgres:
        condition: service_healthy

    # Browser: http://SERVER_IP:18080
    ports:
      - "18080:8080"

    # Let the container reach qBittorrent running on the host:
    # qB URL example: http://host.docker.internal:8085
    extra_hosts:
      - "host.docker.internal:host-gateway"

    volumes:
      # Application data. Back this up before upgrades.
      - ./data:/data
      - ./cache:/cache

      # Beginners can create ./media and ./downloads.
      # NAS users should replace source with real absolute paths.
      # create_host_path=false prevents Docker from silently creating an empty
      # folder when the host path is wrong.
      - type: bind
        source: ./media
        target: /media
        bind:
          create_host_path: false
      - type: bind
        source: ./downloads
        target: /downloads
        bind:
          create_host_path: false

    environment:
      TZ: Asia/Shanghai
      PUID: "1000"
      PGID: "1000"

      MEDIASTATION_APP_HOST: 0.0.0.0
      MEDIASTATION_APP_PORT: 8080
      MEDIASTATION_APP_WEB_DIR: /app/web/dist
      MEDIASTATION_APP_DATA_DIR: /data

      # Lightweight mode uses PostgreSQL by default.
      # Old SQLite data migrates from this path on first start.
      MEDIASTATION_DATABASE_TYPE: postgres
      MEDIASTATION_DATABASE_DSN: postgres://mediastation:mediastation@postgres:5432/mediastation?sslmode=disable
      # After migration, change this to /data/disabled-sqlite-migration.db to disable the SQLite migration source.
      MEDIASTATION_DATABASE_DB_PATH: /data/mediastation.db
      MEDIASTATION_CACHE_CACHE_DIR: /cache

      # Use /media and /downloads in the web UI and downloader by default.
      # Only set MEDIASTATION_*_DIR to real host paths when migrating old
      # libraries/tasks that already stored host paths.
      MEDIASTATION_MEDIA_DIR: /media
      MEDIASTATION_MEDIA_CONTAINER_DIR: /media
      MEDIASTATION_DOWNLOAD_DIR: /downloads
      MEDIASTATION_DOWNLOAD_CONTAINER_DIR: /downloads

  postgres:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: mediastation
      POSTGRES_USER: mediastation
      POSTGRES_PASSWORD: mediastation
    volumes:
      - ./postgres:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -h 127.0.0.1 -U mediastation -d mediastation"]
      interval: 10s
      timeout: 5s
      retries: 10

```

> Note: PostgreSQL is the primary database. Lightweight mode still has short in-process caching. Redis is a cross-process hot cache, and OpenSearch is a search enhancement layer; neither is a source database.

---

## First-time Setup

1. **Create a library**
   - Go to the library page.
   - Use a container path such as `/media/Movies`.
   - Start a scan.

2. **Connect qBittorrent**
   - Go to download client settings.
   - If qBittorrent runs on the host, try `http://host.docker.internal:8085`.

3. **Configure metadata providers**
   - Go to system settings / external APIs.
   - Add TMDb, Bangumi, TheTVDB, Fanart, Douban, or other providers when needed.

4. **Use external players**
   - Add the server as an Emby/Jellyfin-compatible server.
   - Server URL: `http://SERVER_IP:18080`.
   - Use the username and password created in MediaStationGo. No separate Emby server is required.
   - Admins can create regular users in the web UI or Bot so each person can log in with their own account.

5. **Use cloud playback**
   - Configure OpenList, CloudDrive2, WebDAV, or another provider in storage settings.
   - Choose STRMURL or 302 redirect playback in the admin settings.
   - The enabled option takes priority. If both are disabled, playback falls back to the normal server playback path.

---

## Update, Backup, Logs

### Update

```bash
docker compose pull mediastation-go
docker compose up -d --no-deps mediastation-go
```

### Logs

```bash
docker compose logs -f mediastation-go
tail -f ./data/logs/app.log
tail -f ./data/logs/error.log
```

The compose templates keep full application logs in `./data/logs/app.log` and split warnings/errors into `warn.log` and `error.log`. Keep `MEDIASTATION_LOGGING_LEVEL=info` while diagnosing subscription, site search, organizer, or STRM generation issues; temporarily switch to `debug` only when deeper tracing is needed.

Use a writable container path for STRM output, such as `/data/strm` or a mounted media path. Deployments that previously saved `/app/data/strm` are migrated automatically to the configured `MEDIASTATION_APP_DATA_DIR`, which defaults to `/data`.

### Backup

For the default PostgreSQL deployment, back up:

```text
data/
postgres/
```

`postgres/` is the primary database and contains users, libraries, settings, and media metadata. `data/` stores the JWT secret, runtime files, and optional old SQLite migration source.

If you enabled the extended modes, these are optional:

```text
redis/        # hot cache, safe to rebuild
opensearch/   # search index, rebuildable; backing it up can save reindex time on huge libraries
```

`cache/` is usually not important. If you explicitly still use `database.type=sqlite`, the primary database remains `data/mediastation.db`.

### Stop

```bash
docker compose down
```

---

## FAQ

### 1. The web page does not open

Check the container:

```bash
docker ps
docker compose logs --tail=100 mediastation-go
```

Then open:

```text
http://SERVER_IP:18080
```

### 2. The library cannot find files

Most cases are path mistakes.

- Docker maps media to `/media`.
- In the web UI, use `/media/Movies`, not the original NAS path.
- Docker maps downloads to `/downloads`; use `/downloads` as the organization source when possible.

### 3. qBittorrent cannot connect

If qBittorrent is on the host, try:

```text
http://host.docker.internal:8085
```

If qBittorrent is on another machine, use that machine's LAN IP.

### 4. NAS CPU usage is high

Suggested settings:

- Set `ffprobe.max_concurrent` to `1`.
- Enable automatic organization, scrape-after-scan, and boot cloud scan only when you really need them.
- Avoid frequent full-library scans on large libraries. Prefer manual scan or scheduled night sync.

### 5. Should I use `.env`?

Beginners should not. Editing `docker-compose.yml` directly is easier to understand.

`.env` is not required by the provided deployment templates. For the single-image template, edit `docker-compose.simple.yml` directly and only adjust the port, volume paths, and optional hardware device mapping.

---

## Features

| Area | Features |
| --- | --- |
| Libraries | Movies, TV shows, anime, variety, music, adult content |
| Metadata | NFO, local artwork, TMDb, TheTVDB, Bangumi, Douban, Fanart, JavBus/JavDB |
| Playback | Web playback, HTTP Range, HLS transcoding, direct links, STRMURL, 302 redirect |
| External clients | Emby-protocol compatible APIs; MediaStationGo accounts can log in to third-party players |
| User management | Multi-user accounts, admin/regular users, expiry dates, device management, Bot registration and redeem codes |
| Downloads | qBittorrent, site search, subscriptions, post-download organization |
| File manager | Browse, organize, copy, move, hardlink, symlink |
| Operations | Task queue, recycle bin, duplicate files, notifications, logs |
| AI | OpenAI-compatible API, AI search, recommendations, assistant |

Directory hardlinks are handled by recreating the directory tree and hardlinking
each contained file. Linux cannot hardlink a directory itself. Hardlinks still
require the source and target files to be on the same filesystem/subvolume from
inside the container; if media and downloads are separate bind mounts, disks,
btrfs subvolumes, or cloud mounts, use copy or symlink instead.

## Development

Regular users should use Docker. Developers can run:

```bash
go run ./cmd/server
```

Frontend:

```bash
cd web
npm install
npm run dev
```

Tests:

```bash
go test ./...
cd web && npm run build
```

---

## Community and Friends

- Telegram group: <https://t.me/MediaStationGo>
- NodeSeek: [https://www.nodeseek.com/](https://www.nodeseek.com/)
- LINUX DO: [https://linux.do/](https://linux.do/)

---

## Donation

If MediaStationGo saves you time, feel free to buy the author a bowl of noodles.

<img width="200" height="200" alt="WeChat Donation QR" src="https://github.com/user-attachments/assets/d6077de5-8305-400d-8b82-470ef05d926e" />

---

## Star History

<a href="https://www.star-history.com/?repos=ShukeBta%2FMediaStationGo&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=ShukeBta/MediaStationGo&type=date&theme=dark&legend=top-left" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=ShukeBta/MediaStationGo&type=date&legend=top-left" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=ShukeBta/MediaStationGo&type=date&legend=top-left" />
 </picture>
</a>

---

## License and Non-Commercial Statement

This project uses `GPL-3.0` as its base license. See [LICENSE](LICENSE).

The maintainers also state and request:

- The project is intended for personal learning, home NAS, self-hosted media, non-commercial research, and community collaboration.
- Without explicit written permission from the author, do not use this project or derivative versions for commercial resale, paid hosting, paid SaaS, pre-installed commercial devices, closed-source redistribution, or other profit-oriented commercial use.
- For commercial cooperation, enterprise deployment, custom development, integrated redistribution, or commercial authorization, contact the author first.
- If there is any interpretive difference between this README and the formal `GPL-3.0` license text, the code license is governed by [LICENSE](LICENSE); commercial usage should additionally obtain author permission.

---

<p align="center">Made with ❤️ by ShukeBta</p>
