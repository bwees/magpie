# Magpie

Self-hosted server that finds, downloads, imports, subtitles and transcodes movies and TV for a Jellyfin library. It replaces Sonarr, Radarr, Bazarr, Seerr and Tdarr. qBittorrent (5.2 or newer) and Prowlarr stay as they are. GPU work runs on agents.

Two images:

| Image | Platforms | Contents |
|---|---|---|
| `ghcr.io/bwees/magpie` | amd64, arm64 | Server, web UI, ffmpeg/ffprobe for probing. Never transcodes. |
| `ghcr.io/bwees/magpie-agent` | amd64 | Agent, jellyfin-ffmpeg, whisper.cpp (Vulkan). One tag for NVIDIA, Intel and AMD. |

## Quick start

1. Copy [`deploy/compose.yaml`](deploy/compose.yaml) and [`deploy/.env.example`](deploy/.env.example) (as `.env`) into a folder.
2. In `.env`, set `MAGPIE_AGENT_SECRET` (`openssl rand -hex 32`) and `MEDIA_DIR`. Downloads and the library must sit under `MEDIA_DIR` so imports are hardlinks.
3. In `compose.yaml`, uncomment the agent's GPU block for your vendor (see [GPUs](#gpus)).
4. `docker compose up -d`
5. Open `http://<host>:3000`. The setup wizard asks for:
   1. Local admin
   2. TMDB key
   3. qBittorrent (`http://qbittorrent:8080`, an API key from qBittorrent's Web UI settings)
   4. Prowlarr (`http://prowlarr:9696`, its API key)
   5. Jellyfin
   6. Library paths (`/media/library/movies`, `/media/library/tv`)
   7. Quality preset
   8. Wanted languages
   9. OpenSubtitles login
   10. Transcode target
   11. Optional import from Sonarr/Radarr

The agent in the example connects on its own. It shows up under Settings → Agents.

## Environment

Only these are set on the containers. Everything else is in the UI.

| Variable | Image | Default | Use |
|---|---|---|---|
| `MAGPIE_PORT` | server | `3000` | HTTP port |
| `MAGPIE_DATA_DIR` | server | `/data` | SQLite, backups, image cache. Must be local disk, not NFS/SMB. |
| `MAGPIE_BASE_URL` | server | none | Public URL for links |
| `MAGPIE_AGENT_SECRET` | both | required | Shared secret, at least 32 characters. The server won't start without it. |
| `MAGPIE_URL` | agent | required | Server URL the agent dials |
| `MAGPIE_AGENT_NAME` | agent | hostname | Name in the UI (`a-z`, `0-9`, `-`) |
| `MAGPIE_SCRATCH_DIR` | agent | `/scratch` | Working space and the agent's ID. Mount it as a volume. |
| `MAGPIE_AGENT_CPU` | agent | `false` | Also offer the software encoders, for testing without a GPU |

## Adding an agent on another machine

Settings → Agents has the secret and a ready-made `docker run`. Agents need to reach the server over the LAN or a tailnet; they read and write media only through it.

NVIDIA (needs the [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html)):

```sh
docker run -d --name magpie-agent --restart unless-stopped \
  --gpus all \
  -e MAGPIE_URL=http://magpie.lan:3000 \
  -e MAGPIE_AGENT_SECRET=<secret> \
  -v magpie-agent-scratch:/scratch \
  ghcr.io/bwees/magpie-agent:latest
```

Intel or AMD:

```sh
docker run -d --name magpie-agent --restart unless-stopped \
  --device /dev/dri \
  --group-add "$(getent group render | cut -d: -f3)" \
  -e MAGPIE_URL=http://magpie.lan:3000 \
  -e MAGPIE_AGENT_SECRET=<secret> \
  -v magpie-agent-scratch:/scratch \
  ghcr.io/bwees/magpie-agent:latest
```

On start the agent test-encodes on every GPU it sees and reports what passed. Settings → Agents shows each device's results.

### GPUs

- **NVIDIA:** the image sets `NVIDIA_DRIVER_CAPABILITIES=compute,video,utility,graphics`. `video` is needed for NVENC/NVDEC and `graphics` for Whisper's Vulkan backend. If you override it, keep both.
- **Intel/AMD:** pass `/dev/dri` (or one `renderD*` node) and add the host's `render` group. The group ID differs between hosts.
- Linux only. Docker Desktop on macOS can't pass a GPU through.

## Backups and upgrades

- A snapshot of the database goes to `<data>/backups` every day (03:00, keep 7, both changeable in Settings → General) and before every migration.
- Settings → General can back up now, list snapshots and restore one. The server restarts after a restore.
- Upgrading is pulling a newer image. Migrations run at startup after a snapshot. If one fails, the server doesn't start and the snapshot stays.

## Development

Tools come from [mise](https://mise.jdx.dev): `mise install`, then `mise run install:web`.

| Task | Does |
|---|---|
| `mise run dev` | Server with live reload plus the web dev server |
| `mise run lint`, `check`, `test`, `build` | The CI checks |
| `mise run openapi` | Regenerate `server/openapi.json` and the web client |
| `mise run release:snapshot` | Build binaries and load both images locally |
| `mise run smoke` | Run the snapshot images with a CPU agent and check it comes online |

## Releases

[release-please](https://github.com/googleapis/release-please) keeps a release PR open on `main` from the conventional commits. Merging it tags the version and creates the GitHub release; GoReleaser then attaches the binaries and pushes `ghcr.io/bwees/magpie` and `ghcr.io/bwees/magpie-agent`.

## Licence

GPL-3.0. The agent image bundles [jellyfin-ffmpeg](https://github.com/jellyfin/jellyfin-ffmpeg) (GPL-3.0) and [whisper.cpp](https://github.com/ggml-org/whisper.cpp) (MIT).
