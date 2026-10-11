# Magpie

Self-hosted app that finds, downloads, organises, subtitles and transcodes movies and TV for a Jellyfin library.

## Language

### Catalog

**Title**:
A movie or a series that Magpie tracks.
_Avoid_: show, item, media

**Episode group**:
The TMDB episode ordering chosen for a series when release numbering differs from aired order.
_Avoid_: scene numbering, alternate order

**Wanted languages**:
The user's ordered language list, global with a per-Title override. It decides which subtitles to find and which audio/subtitle tracks a transcode keeps.

**Origin**:
How a Title or Media file entered Magpie: a request, a manual add, a library scan, a migration, or a download.

### Requests

**Request**:
A user asking for a movie, a whole series, or chosen seasons. Needs admin approval unless the user is auto-approved.

**Issue**:
A user's report that something is wrong with a Title's media (video, audio, subtitle, other).
_Avoid_: ticket, problem

### Acquisition

**Indexer**:
A tracker Magpie searches. Configured in Prowlarr; Magpie queries each one directly through Prowlarr.
_Avoid_: tracker, provider

**Release**:
One search result from an Indexer: a torrent that may contain media for a movie or episodes.
_Avoid_: result, torrent

**Quality profile**:
The rules for which Releases are acceptable for a Title, and when to stop upgrading.

**Cutoff**:
The point at which a file is good enough. Below it, Magpie keeps upgrading; once met, the file is transcoded and upgrades stop.

**Blocked file**:
A file inside a Release whose name matches the user's block list. Finding one rejects the Release.

**Download**:
A grabbed Release in qBittorrent, from grab until the torrent is removed.

### Library

**Media file**:
A video file in the library belonging to one movie or one or more episodes.

**Available**:
A movie or episode whose Media file Jellyfin has confirmed. The last main pipeline stage; subtitles and transcoding are tracked separately.

### Subtitles

**Subtitle coverage**:
Whether a Media file has a subtitle for every Wanted language, and where each one came from (embedded, provider, Whisper).

### Transcode

**Agent**:
A worker process on another machine that runs GPU jobs (transcodes, subtitle generation) for the server. Reads and writes media only through its connection to the server.
_Avoid_: node, worker

**Job**:
One unit of work assigned to an Agent: a transcode or a Whisper run on one Media file.

**Transcode target**:
The codec, quality preset and per-resolution bitrate caps every file is converted to.

### Activity

**Timeline**:
The per-Title history of what happened in its pipeline, including rejections and their reasons.
