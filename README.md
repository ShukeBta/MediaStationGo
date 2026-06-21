# MediaStationGo

<p align="center">
  <img src="web/public/brand/mgo-emby-icon.svg" width="96" height="96" alt="MediaStationGo Logo" />
</p>

<h3 align="center">适合 NAS、家庭共享和多端播放的私人媒体中心</h3>

<p align="center">
  <strong>Docker 一键部署 · PostgreSQL 主库 · Redis 热缓存 · OpenSearch 搜索增强 · Emby 协议兼容 · Bot 通知</strong>
</p>

<p align="center">
  <a href="#快速开始">快速开始</a> ·
  <a href="#三挡部署">三挡部署</a> ·
  <a href="#路径映射">路径映射</a> ·
  <a href="#旧-sqlite-迁移">旧 SQLite 迁移</a> ·
  <a href="#开发构建">开发构建</a> ·
  <a href="CONTRIBUTING.md">贡献规范</a> ·
  <a href="https://mgo.3jzs.com">在线演示</a>
</p>

<p align="center">
  <img alt="Go" src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat-square&logo=go&logoColor=white" />
  <img alt="React" src="https://img.shields.io/badge/React-18-61DAFB?style=flat-square&logo=react&logoColor=111827" />
  <img alt="Docker" src="https://img.shields.io/badge/Docker-ready-2496ED?style=flat-square&logo=docker&logoColor=white" />
  <img alt="License" src="https://img.shields.io/badge/License-GPL--3.0-blue?style=flat-square" />
</p>

---

## 项目简介

MediaStationGo 是一个自托管媒体管理系统，面向 NAS、小主机、家庭影音和多用户共享场景。它把媒体库、刮削、下载整理、订阅、网盘播放、Emby 协议兼容、用户权限和 Bot 通知放在一个后台里，目标是让用户只维护一套服务，就能给网页端、手机端、电视端和第三方播放器使用。

核心能力：

- **媒体库管理**：电影、电视剧、动漫、综艺、音乐和自定义媒体库统一管理。
- **Emby 协议兼容**：Infuse、VidHub、SenPlayer、Fileball 等客户端可按 Emby/Jellyfin 方式添加服务器。
- **本地 + 网盘**：支持本地硬盘、下载目录、OpenList、CloudDrive2、WebDAV、STRMURL 和 302 反代播放。
- **订阅下载入库**：连接 qBittorrent 后支持搜索、订阅、下载完成整理、刮削和入库通知。
- **多用户与权限**：管理员/普通用户、有效期、成人内容开关、设备管理、注册码和 Telegram Bot 绑定。
- **三挡部署**：按规模选择 PostgreSQL、Redis、OpenSearch，低配 NAS 到大库检索都能覆盖。

## 社区与友链

- Telegram MediaStationGo交流群：<https://t.me/MediaStationGo>
- NodeSeek：[https://www.nodeseek.com/](https://www.nodeseek.com/)
- LINUX DO：[https://linux.do/](https://linux.do/)
- Mgo-Emby: [https://bbs.3jzs.com/](https://bbs.3jzs.com/)  #BUG反馈-更新日志-支持建议

## 在线演示

- 地址：[https://mgo.3jzs.com](https://mgo.3jzs.com)
- 账号：`admin`
- 密码：`admin123`

> 演示站只用于看功能，请不要填写私人 API Key、站点 Cookie 或真实隐私信息。

## 快速开始

最推荐使用 Docker Compose。默认模板不依赖 `.env`，复制后按自己的 NAS 路径改 `volumes` 和路径环境变量即可。

```bash
mkdir -p MediaStationGo
cd MediaStationGo
curl -fsSL https://raw.githubusercontent.com/ShukeBta/MediaStationGo/main/docker-compose.yml -o docker-compose.yml
docker compose up -d
```

启动后访问：

```text
http://服务器IP:18080
```

默认账号：

```text
admin / admin123
```

首次登录后请立刻修改管理员密码。

镜像地址：

```text
GHCR：ghcr.io/shukebta/mediastation-go:latest
Docker Hub 备用：shukbet/mediastationgo:latest
```

## 三挡部署

MediaStationGo 推荐按机器资源和用户规模选择部署档位。三挡都使用 PostgreSQL 作为主数据库；Redis 和 OpenSearch 是增强组件，不替代 PostgreSQL。

| 档位 | 组件 | 适合场景 | 启动命令 |
| --- | --- | --- | --- |
| 第一档 | MediaStationGo + PostgreSQL | 大多数 NAS、个人/家庭使用、低内存机器 | `docker compose up -d` |
| 第二档 | MediaStationGo + PostgreSQL + Redis | 多用户、Emby 客户端频繁刷新、首页/媒体列表访问较多 | `docker compose -f docker-compose.yml -f docker-compose.standard.yml up -d` |
| 第三档 | MediaStationGo + PostgreSQL + Redis + OpenSearch | 超大媒体库、复杂全文搜索、后续需要独立搜索索引 | `docker compose -f docker-compose.yml -f docker-compose.standard.yml -f docker-compose.search.yml up -d` |

### 第一档：PostgreSQL

第一档是默认推荐部署。它只启动主服务和 PostgreSQL，资源占用最低，适合绝大多数 NAS。

```bash
# 只拉取 MediaStationGo 主服务镜像，避免升级时动到 PostgreSQL / Redis
docker compose pull mediastation-go

# 启动第一档：MediaStationGo + PostgreSQL
docker compose up -d --no-deps mediastation-go
```

关键数据目录：

```text
./postgres   PostgreSQL 主数据库，必须备份
./data       JWT 密钥、运行配置、旧 SQLite 迁移源
./cache      海报、临时文件、转码缓存，可删除重建
```

### 第二档：PostgreSQL + Redis

第二档在第一档基础上叠加 Redis。Redis 用作热缓存，能减轻多用户和 Emby 客户端频繁刷新时的数据库压力。

```bash
# 启动第二档：基础 compose + Redis 叠加文件
docker compose -f docker-compose.yml -f docker-compose.standard.yml up -d
```

Redis 数据目录是 `./redis`。它主要保存缓存，通常可重建；真正需要备份的仍然是 `./postgres` 和 `./data`。

### 第三档：PostgreSQL + Redis + OpenSearch

第三档在第二档基础上叠加 OpenSearch，用于大库全文搜索和独立搜索索引。OpenSearch 常驻内存明显更高，低配 NAS 不建议开启。

```bash
# 启动第三档：基础 compose + Redis + OpenSearch
docker compose -f docker-compose.yml -f docker-compose.standard.yml -f docker-compose.search.yml up -d
```

OpenSearch 数据目录是 `./opensearch`。搜索索引可重建，但重建大库索引会花时间；机器资源足够时再开启第三档。

## 配置示例

仓库内提供三份推荐 Compose 文件：

```text
docker-compose.yml            第一档：MediaStationGo + PostgreSQL
docker-compose.standard.yml   第二档叠加：Redis 热缓存
docker-compose.search.yml     第三档叠加：OpenSearch 搜索增强
```

常用配置片段如下，注释保留为中文，方便直接复制到 NAS 上调整：

```yaml
services:
  mediastation-go:
    image: ghcr.io/shukebta/mediastation-go:latest
    ports:
      # 左边是宿主机访问端口，右边是容器内端口。
      - "18080:8080"
    volumes:
      # 运行数据：JWT 密钥、配置、旧 SQLite 迁移源。
      - ./data:/data

      # 缓存目录：海报、临时文件、转码缓存，可删除重建。
      - ./cache:/cache

      # 媒体库目录：自动整理/重命名/入库需要写权限。
      - /vol1/1000/Media:/media

      # 下载目录：qBittorrent 保存目录和自动整理源目录。
      - /vol1/1000/Downloads:/downloads
    environment:
      TZ: Asia/Shanghai

      # PostgreSQL 主数据库。
      MEDIASTATION_DATABASE_TYPE: postgres
      MEDIASTATION_DATABASE_DSN: postgres://mediastation:mediastation@postgres:5432/mediastation?sslmode=disable

      # 旧 SQLite 迁移源：只在从旧版 data/mediastation.db 导入时使用。
      MEDIASTATION_DATABASE_DB_PATH: /data/mediastation.db

      # 路径换算：宿主机路径和容器路径必须一一对应。
      MEDIASTATION_MEDIA_DIR: /vol1/1000/Media
      MEDIASTATION_MEDIA_CONTAINER_DIR: /media
      MEDIASTATION_DOWNLOAD_DIR: /vol1/1000/Downloads
      MEDIASTATION_DOWNLOAD_CONTAINER_DIR: /downloads
```

## 路径映射

路径映射是 Docker 部署里最容易填错的地方。原则是：`volumes` 左边是宿主机真实路径，右边是容器内路径；环境变量里也要保持对应关系。

NAS 示例：

```yaml
volumes:
  - /vol1/1000/Docker/moviepilot-v2/media:/vol1/1000/Docker/moviepilot-v2/media
  - /vol1/1000/qBittorrent/downloads:/vol1/1000/qBittorrent/downloads
environment:
  MEDIASTATION_MEDIA_DIR: /vol1/1000/Docker/moviepilot-v2/media
  MEDIASTATION_MEDIA_CONTAINER_DIR: /vol1/1000/Docker/moviepilot-v2/media
  MEDIASTATION_DOWNLOAD_DIR: /vol1/1000/qBittorrent/downloads
  MEDIASTATION_DOWNLOAD_CONTAINER_DIR: /vol1/1000/qBittorrent/downloads
```

Windows Docker Desktop 示例：

```yaml
volumes:
  - D:/Media:/media
  - D:/Downloads:/downloads
environment:
  MEDIASTATION_MEDIA_DIR: D:/Media
  MEDIASTATION_MEDIA_CONTAINER_DIR: /media
  MEDIASTATION_DOWNLOAD_DIR: D:/Downloads
  MEDIASTATION_DOWNLOAD_CONTAINER_DIR: /downloads
```

如果后台添加媒体库时填的是 `/vol1/...`，Compose 里也建议把同一个 `/vol1/...` 挂进容器，避免自动整理和下载入库时路径不可访问。

## 旧 SQLite 迁移

新版推荐 PostgreSQL 作为主数据库。`MEDIASTATION_DATABASE_DB_PATH` 不是主库路径，而是旧 SQLite 数据的迁移源。

迁移步骤：

1. 把旧版 `mediastation.db` 放到 `./data/mediastation.db`。
2. 保持 `MEDIASTATION_DATABASE_DB_PATH: /data/mediastation.db`。
3. 启动一次，确认日志显示迁移完成，网页数据正常。
4. 备份 `./postgres` 和 `./data`。
5. 确认不再需要 SQLite 后，把迁移源改成不存在的路径，例如：

```yaml
environment:
  # 已完成 SQLite 迁移后，建议改成不存在的路径，避免下次启动重复检查旧库。
  MEDIASTATION_DATABASE_DB_PATH: /data/no-sqlite-migration.db
```

不要删除 `./postgres`。PostgreSQL 已经是主数据库，删除它会丢失账号、媒体库、订阅、配置和历史数据。

## 更新与备份

更新镜像：

```bash
docker compose pull mediastation-go
docker compose up -d --no-deps mediastation-go
```

不要执行裸 `docker compose pull` 做日常更新。PostgreSQL / Redis 是数据与缓存基础组件，compose 已设置为 `pull_policy: never`；需要升级它们时，请先备份 `./postgres`，再手动修改镜像版本并单独拉取。

第二档和第三档更新时继续带上叠加文件：

```bash
# 第二档
docker compose -f docker-compose.yml -f docker-compose.standard.yml pull mediastation-go
docker compose -f docker-compose.yml -f docker-compose.standard.yml up -d --no-deps mediastation-go

# 第三档
docker compose -f docker-compose.yml -f docker-compose.standard.yml -f docker-compose.search.yml pull mediastation-go
docker compose -f docker-compose.yml -f docker-compose.standard.yml -f docker-compose.search.yml up -d --no-deps mediastation-go
```

必须备份：

```text
./postgres   PostgreSQL 主数据库
./data       JWT 密钥、运行配置、旧 SQLite 迁移源
```

可重建：

```text
./cache      图片缓存、临时文件、转码缓存
./redis      Redis 热缓存
./opensearch 搜索索引
```

## Bot 与通知

MediaStationGo 支持 Telegram Bot 绑定、用户菜单、群组管理菜单和事件通知。常见通知事件包括：

- 订阅命中新资源
- 下载任务完成
- 入库完成
- 刮削失败告警
- 系统异常通知

管理员可以在后台配置 Bot Token、Chat ID、通知频道和事件类型。群组里管理类命令只允许管理员执行，普通用户只能看到和使用用户命令。

## 常见问题

**启动后还是反复迁移 SQLite？**

确认旧数据已经迁移成功后，把 `MEDIASTATION_DATABASE_DB_PATH` 改成不存在的路径，例如 `/data/no-sqlite-migration.db`，然后重启容器。

**扫库或入库速度很慢？**

先确认数据库档位和路径映射正确。第一档已经足够大多数场景；第二档 Redis 能缓解频繁刷新造成的数据库压力；第三档主要增强搜索，不会替代媒体扫描本身。网盘扫描还会受网盘接口响应、目录数量和网络质量影响。

**qBittorrent 下载完成后无法整理？**

确认 qBittorrent 保存路径已经通过 `volumes` 挂载进 MediaStationGo 容器，并且 `MEDIASTATION_DOWNLOAD_DIR` 与 `MEDIASTATION_DOWNLOAD_CONTAINER_DIR` 对应正确。

**第三方播放器无法连接？**

确认播放器填写的是 `http://服务器IP:18080`，账号密码使用 MediaStationGo 用户账号。反代部署时需要正确设置外部访问地址和 HTTPS 头。

## 开发构建

本地开发需要 Go、Node.js 和 npm。

```bash
# 后端测试
go test ./...

# 前端依赖与构建
npm --prefix web install
npm --prefix web run build

# 本地运行后端
go run ./cmd/server

# 本地运行前端开发服务器
npm --prefix web run dev
```

前端开发服务器默认访问：

```text
http://127.0.0.1:3000
```

后端健康检查：

```text
http://127.0.0.1:8080/api/health
```

## 贡献与反馈

提交 Bug、功能建议或 Pull Request 前，请先阅读 [贡献规范](CONTRIBUTING.md)。

- Bug 反馈请使用 Issue 模板，并提供部署方式、复现步骤、日志和关键配置。
- 功能建议请说明使用场景、期望行为和可接受的替代方案。
- 安全漏洞请不要公开发 Issue，按 [安全策略](SECURITY.md) 使用私密渠道报告。
- Pull Request 请从独立分支或 fork 分支发起，不要直接向 `main` 推送。
- 分支名建议使用 `fix/...`、`feat/...`、`docs/...` 或 `test/...`，例如 `docs/contribution-guidelines`。
- 提交前按改动范围运行 `go test ./...`、`npm --prefix web run build` 或定向测试，并在 PR 中说明验证结果。

## 赞赏

如果这个项目节省了你的时间，欢迎请作者吃桶泡面。

<img width="200" height="200" alt="微信赞赏码" src="https://github.com/user-attachments/assets/d6077de5-8305-400d-8b82-470ef05d926e" />

## Star History

<a href="https://www.star-history.com/?repos=ShukeBta%2FMediaStationGo&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=ShukeBta/MediaStationGo&type=date&theme=dark&legend=top-left" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=ShukeBta/MediaStationGo&type=date&legend=top-left" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=ShukeBta/MediaStationGo&type=date&legend=top-left" />
 </picture>
</a>

## 许可证

本项目使用 GPL-3.0 License。详见 [LICENSE](LICENSE)。
