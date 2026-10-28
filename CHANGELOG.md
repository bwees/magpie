# Changelog

## [0.8.0](https://github.com/bwees/magpie/compare/v0.7.2...v0.8.0) (2026-10-28)


### Features

* add manage files permission for requesters ([21f7993](https://github.com/bwees/magpie/commit/21f7993b5c67788bc287c981126d965d4a3cd9f2))

## [0.7.2](https://github.com/bwees/magpie/compare/v0.7.1...v0.7.2) (2026-10-21)


### Bug Fixes

* **activity:** reset the pipeline when an episode's air date moves later ([e5008d9](https://github.com/bwees/magpie/commit/e5008d9f4332aab4dd2fac0c9733804cdb76d530))

## [0.7.1](https://github.com/bwees/magpie/compare/v0.7.0...v0.7.1) (2026-10-21)


### Bug Fixes

* **activity:** stop flagging searches that found no releases as failed ([16189b5](https://github.com/bwees/magpie/commit/16189b5ed17bdbaa89e08542a4806e7ed878a7e8))
* **catalog:** unrelease episodes whose air date moves into the future ([030fc40](https://github.com/bwees/magpie/commit/030fc405cd8e1665afcf582ad29ef30e170703dc))

## [0.7.0](https://github.com/bwees/magpie/compare/v0.6.0...v0.7.0) (2026-10-16)


### Features

* **web:** name embedded subtitle languages and show the list on hover ([74926f5](https://github.com/bwees/magpie/commit/74926f501a6bcb816f2fbfbfb509f0b280a82464))


### Bug Fixes

* **acquisition:** add torrents to qBittorrent started ([2f6597d](https://github.com/bwees/magpie/commit/2f6597d6d564a0b98cda017b1b3e95792ac93ded))
* **web:** stop deletes erroring on 204 responses ([e49eaaf](https://github.com/bwees/magpie/commit/e49eaafea95c6323a96b8cd9dfd9cbb9f34c8955))

## [0.6.0](https://github.com/bwees/magpie/compare/v0.5.0...v0.6.0) (2026-10-13)


### Features

* **transcode:** dismiss failed jobs ([142dd41](https://github.com/bwees/magpie/commit/142dd4116985fc8ea4ab6fbf23f9a08a29c6fa3c))
* **web:** collapse the title timeline to its latest four events ([cb2b9b5](https://github.com/bwees/magpie/commit/cb2b9b52bd25b51c5c094d8adc207252feeacd90))
* **web:** split GPU jobs into queued and failed tabs with paging and a skipped popover ([8816cd6](https://github.com/bwees/magpie/commit/8816cd6eec714e7e0d538e2c79548c0581bb6635))

## [0.5.0](https://github.com/bwees/magpie/compare/v0.4.0...v0.5.0) (2026-10-13)


### Features

* **web:** show episode files in a dialog and compact the file card ([4f58813](https://github.com/bwees/magpie/commit/4f58813883baad363688ae3cc67dd28aaa284f33))


### Bug Fixes

* **activity:** stop counting unmonitored episodes without a file as failed ([b717258](https://github.com/bwees/magpie/commit/b71725808ad6355d239f3f21329b23b39a3df359))
* **web:** reconnect live sockets that hang on connect or go silent ([746fe8c](https://github.com/bwees/magpie/commit/746fe8c293f177cad4f4cf77ad7fca3eefcd0884))

## [0.4.0](https://github.com/bwees/magpie/compare/v0.3.1...v0.4.0) (2026-10-13)


### Features

* **web:** clear the Needs you list from Home ([118745f](https://github.com/bwees/magpie/commit/118745f1df07d913b6f25ea2722374dd38e19791))


### Bug Fixes

* **library:** leave the device out of file ETags so NFS remounts don't refuse swaps ([ca5b01a](https://github.com/bwees/magpie/commit/ca5b01a4b0e3fb9c040882feb62e3894f9993e6b))
* **transcode:** give QSV a target just under the cap so quality and the cap both apply ([d05180e](https://github.com/bwees/magpie/commit/d05180e58cd9dce754f1d6e876333ab51462f14a))

## [0.3.1](https://github.com/bwees/magpie/compare/v0.3.0...v0.3.1) (2026-10-13)


### Bug Fixes

* **acquisition:** move a deleted profile's titles to the default and fall back when one is missing ([7bff6d4](https://github.com/bwees/magpie/commit/7bff6d43a0f1c43ad25038f00f4a432392c7a788))
* **transcode:** check encoded output on the GPU and report it as its own stage ([0a08422](https://github.com/bwees/magpie/commit/0a084222224b218608a5ed3397bcec8c2c2fba83))
* **transcode:** fit the AMD VAAPI quality values to VMAF and use the driver's quality preset ([96901a4](https://github.com/bwees/magpie/commit/96901a481c70015674cc0bd3d0511aac85bc52cb))
* **transcode:** report encode progress from the frame count when ffmpeg has no output time ([42dac83](https://github.com/bwees/magpie/commit/42dac833e0e701646eab598ff003fccebcf21475))

## [0.3.0](https://github.com/bwees/magpie/compare/v0.2.0...v0.3.0) (2026-10-13)


### Features

* **web:** interactive search per season and episode ([533be06](https://github.com/bwees/magpie/commit/533be06073e58917ed1b85bdd3dca155831ae9d0))
* **web:** list a series' seasons newest first with specials last ([f8053b8](https://github.com/bwees/magpie/commit/f8053b8f3c74d5228ca38a6ac4ffe5520d52302d))


### Bug Fixes

* **acquisition:** grab well-seeded releases in a wanted language without duplicates ([0efe99d](https://github.com/bwees/magpie/commit/0efe99d0c2e6e7dfe4df9320fa43738ba42f6cea))
* **activity:** clear rows left on rejected downloads and retried transcodes ([cc97115](https://github.com/bwees/magpie/commit/cc97115cd33721579886b3e78754aed5f7e6d330))
* **activity:** follow cancelled transcodes and removed downloads ([86a19df](https://github.com/bwees/magpie/commit/86a19df03621ab33032c805c7114777f4ab88068))
* **live:** reset a lagging browser socket instead of dropping it ([41b2830](https://github.com/bwees/magpie/commit/41b28307f9b35bf6ecae2423bfd3323c0bbede92))
* time out hung requests and keep long agent transfers alive ([281a1ae](https://github.com/bwees/magpie/commit/281a1ae0a7ab27112c5bf65caf632dab2eb0a065))
* **transcode:** detect dead agent sockets and stop offering to busy devices ([331db32](https://github.com/bwees/magpie/commit/331db32df9e4c7d53cca0693f6d2cf8e20bf70c0))
* **transcode:** give VAAPI QVBR encodes the target bitrate they require ([3d51e06](https://github.com/bwees/magpie/commit/3d51e064a48f5e314125550feeac7e7efb173876))

## [0.2.0](https://github.com/bwees/magpie/compare/v0.1.1...v0.2.0) (2026-10-13)


### Features

* **library:** count files available once imported ([10561d0](https://github.com/bwees/magpie/commit/10561d0b03d249df5261f7210b1d20d2e1684581))


### Bug Fixes

* **library:** apply the qBittorrent path mapping when an import runs ([a129a17](https://github.com/bwees/magpie/commit/a129a1758b2ca2fbf7c1101e2fc66f3091212653))
* **web:** keep title page state per title so leaving it can't stall the next page ([c8a9121](https://github.com/bwees/magpie/commit/c8a9121507b094e9ee9fd9336fbcd000580a3607))

## [0.1.1](https://github.com/bwees/magpie/compare/v0.1.0...v0.1.1) (2026-10-12)


### Bug Fixes

* **transcode:** never drop an agent socket mid-write when the server stops ([3d43638](https://github.com/bwees/magpie/commit/3d436387fe56f6e17c7825d845b1eca7a8032d45))

## 0.1.0 (2026-10-12)


### Features

* **acquisition:** add api contract, importer, reader fake and prowlarr, qbittorrent and trash ports ([63bfdec](https://github.com/bwees/magpie/commit/63bfdec06f540fcb73e4d74520d596b8ae279828))
* **acquisition:** add interactive search with per-indexer fan-out, result cache and search endpoints ([cba961c](https://github.com/bwees/magpie/commit/cba961c877471763346ac3a14230992c14770aa6))
* **acquisition:** add privacy, health and web url to the prowlarr port ([37b281a](https://github.com/bwees/magpie/commit/37b281adff702a559f37d7ad1f27d18d8451f901))
* **acquisition:** add quality model, custom formats, presets and trash sync ([8f6636c](https://github.com/bwees/magpie/commit/8f6636c6b198ea24477d58de3e45ec4580ee6f5d))
* **acquisition:** add quality profile, custom format, trash and settings endpoints with the trash adapter ([d845da0](https://github.com/bwees/magpie/commit/d845da07cafc6ad36404390d49c5bd463371bc89))
* **acquisition:** add the release decision engine and ranking ([0c1ff73](https://github.com/bwees/magpie/commit/0c1ff731c423e19faf938456d3c77dcdd3eb3338))
* **acquisition:** automatic search with settle window, delay queue, RSS and backlog sweeps ([f40b932](https://github.com/bwees/magpie/commit/f40b9327fcbd4bf855cbf5ccc9cd82ed4a59c3d4))
* **acquisition:** blocklist and re-grab a reported file on an issue's REGRAB action ([4afcff0](https://github.com/bwees/magpie/commit/4afcff092c95d35db06404abfb3bf538b07479ce))
* **acquisition:** grab into qBittorrent with blocked-file checks, timeouts and download polling ([de908fd](https://github.com/bwees/magpie/commit/de908fde2e0021861ab26ccdcff236324737edbf))
* **acquisition:** import migrated formats, profiles, delays and blocklist rows ([6bd1304](https://github.com/bwees/magpie/commit/6bd13046788a787f55e1ebde6f1e4f55afa34f2f))
* **acquisition:** keep a manual import's download until the user maps or removes it ([f2740c6](https://github.com/bwees/magpie/commit/f2740c6b783c839a9bf380bc18ef5baa7071f324))
* **acquisition:** keep file scores and publish CutoffMet from library file events ([0121d0b](https://github.com/bwees/magpie/commit/0121d0b0d3680024f77a4a597d368868e43f2ddc))
* **acquisition:** mark imported downloads, drop failed imports and check seeding goals ([a15c33f](https://github.com/bwees/magpie/commit/a15c33f3202058f0ecc0c92878091a4077b88c2e))
* **acquisition:** read the default profile through acquisition.Reader for requests ([11450b9](https://github.com/bwees/magpie/commit/11450b9f2ad22a099b043101396ac8cdc22c3f58))
* **acquisition:** rescore files when a profile or custom format changes ([f94b78d](https://github.com/bwees/magpie/commit/f94b78d1bfc18de4a05c3255ad8d5680b678e216))
* **acquisition:** run an issue's REGRAB once and look past the waiting downloads the admin removed ([c80fbb6](https://github.com/bwees/magpie/commit/c80fbb6824750bddc6cc3668d3d18860ac05eac9))
* **acquisition:** serve each file's cutoff status for the title page ([b3f1160](https://github.com/bwees/magpie/commit/b3f1160a3b03174e483519a5e8e12dffc209748f))
* **acquisition:** sweep cutoff-unmet files from file scores ([84cd743](https://github.com/bwees/magpie/commit/84cd743b0523d77eec7da18fe381491586bef6d3))
* **acquisition:** sync indexers from prowlarr with per-indexer settings and endpoints ([45930db](https://github.com/bwees/magpie/commit/45930dbaa1c9f5cf55eedade3b9c609fc1d83462))
* **activity:** add api contract, importer, reader fake and notification sender port ([9eba40b](https://github.com/bwees/magpie/commit/9eba40b7880f8e028f58e4a093b14a95056c8955))
* **activity:** add in-app notifications, notification urls and the send job ([13106dd](https://github.com/bwees/magpie/commit/13106dd984df6a98bb2569dbd605e90a386b8f05))
* **activity:** import Sonarr/Radarr history onto timelines ([ee14366](https://github.com/bwees/magpie/commit/ee1436657cd5e581f3e4e06302b62c13233171cf))
* **activity:** link issue notifications to the issue's thread ([e463ca3](https://github.com/bwees/magpie/commit/e463ca3ac63d8b3fd35a3742db3a0e816e6499d4))
* **activity:** project events into timelines, history, pipeline stages and attention items ([60c24b6](https://github.com/bwees/magpie/commit/60c24b6e6f6012d9050c2dfb06220db5cf94bba0))
* **activity:** recently active titles and episode numbering on pipeline rows ([b378eff](https://github.com/bwees/magpie/commit/b378eff122a93f5f78dab4bcff47583e677023c4))
* **activity:** show FileRenamed on the title timeline ([af100b0](https://github.com/bwees/magpie/commit/af100b08ca816d30819b19093eb842720ff34c1d))
* **adapters:** add health probes for tmdb, qbittorrent, prowlarr, jellyfin and opensubtitles ([d2f756a](https://github.com/bwees/magpie/commit/d2f756ac607be9ee41b61b7859f0d4656dc45f6f))
* **adapters:** add jellyfin auth adapter for sign-in and quick connect ([46d4346](https://github.com/bwees/magpie/commit/46d43464309c34d7c02ddafa69b73773b24490d0))
* **adapters:** add prowlarr adapter with indexer caps, torznab search and magnet redirects ([94e18c1](https://github.com/bwees/magpie/commit/94e18c189cf78cd6e7b0dbf3c2c6b91910c7889b))
* **adapters:** add the disk, ffprobe and qBittorrent save path adapters for imports ([9421277](https://github.com/bwees/magpie/commit/9421277d2aa7aa9dcd6448f57157fc03a4719c33))
* **adapters:** add the Jellyfin library adapter for path refresh, item lookup, LibraryChanged and reconcile ([d7eb929](https://github.com/bwees/magpie/commit/d7eb9290cdcb034c117686f2db7e12bf07d5e906))
* **adapters:** add the shoutrrr notification sender ([9e40498](https://github.com/bwees/magpie/commit/9e40498eda58de5c60dd7b7b5e1a35b9dfc52a6f))
* **adapters:** let dev builds point OpenSubtitles at a stand-in server ([2a9aaf9](https://github.com/bwees/magpie/commit/2a9aaf9c24632a288241737323e19f67f6dc9a97))
* **adapters:** search and download OpenSubtitles within the rate limit and quota ([2388523](https://github.com/bwees/magpie/commit/23885231105851bd25ed15ff1a78e6629c8f3d81))
* **adapters:** write and remove subtitle sidecars on disk ([be69f90](https://github.com/bwees/magpie/commit/be69f902c44c4e48df0a5caa22b0ea9e1f0e49ea))
* **agent:** accept, run, heartbeat and report transcode jobs ([b8c8886](https://github.com/bwees/magpie/commit/b8c888690fc756d7a4a3f525cc3d166d609f54e8))
* **agent:** add agent binary with capability probe and reconnecting control session ([cc605af](https://github.com/bwees/magpie/commit/cc605afc5452d9d47d5edfc5f7540e0fb5d6b4fe))
* **agent:** encode on the device's pipeline with a CPU-decode fallback and verify the output ([6abfa24](https://github.com/bwees/magpie/commit/6abfa24172de7f482978de188e585a1daa23f410))
* **agent:** offer the software encoders as a CPU device with MAGPIE_AGENT_CPU ([c39464c](https://github.com/bwees/magpie/commit/c39464c2c51e8a39db26fd7892997c8a73f4e0f3))
* **agentproto:** add job byte paths, upload headers, the CPU vendor and VERIFY_FAILED ([9c12955](https://github.com/bwees/magpie/commit/9c1295565b1087f567dad2f79fadc3d45c22c125))
* **agent:** resumable source download and output upload ([309cd4a](https://github.com/bwees/magpie/commit/309cd4abc6afba5c2f1f6d517f7b500a64d5e3ec))
* **agent:** run Whisper jobs with whisper.cpp and shift cues by the audio offset ([f174e2e](https://github.com/bwees/magpie/commit/f174e2e4d5b4be1e90ea0935e90b3c63adcc2d0c))
* **app:** wire the OpenSubtitles client and the subsync Syncer into Subtitles ([0ccb0e4](https://github.com/bwees/magpie/commit/0ccb0e4a2f19eac5e61d3970557dfb31c551edf8))
* **catalog,library:** keep imported aliases and rename migrated files on request ([0cdd52c](https://github.com/bwees/magpie/commit/0cdd52cd69e797a2c1e9e33fbe91ca6a1f4e0f5b))
* **catalog:** add api contract, importer, reader fake and tmdb port ([795af52](https://github.com/bwees/magpie/commit/795af5278aaea93dfa01627ece86cd1fed0937ca))
* **catalog:** add movie titles from approved requests with release clock, refresh, image cache and HTTP routes ([0fa4be7](https://github.com/bwees/magpie/commit/0fa4be7a6a16cf4248b04645c345891bfbd273ee))
* **catalog:** add the discover detail route for request rows and the season picker ([9100a97](https://github.com/bwees/magpie/commit/9100a970f823a43c06966b097be222fdc88c95f2))
* **catalog:** add the episodes tab, series settings and wanted-languages editors ([4754cdb](https://github.com/bwees/magpie/commit/4754cdb41321308a1564ca5483f336b426a083e8))
* **catalog:** import migrated titles idempotently on their source keys ([31908b0](https://github.com/bwees/magpie/commit/31908b061a7f851712a9b3b070164629b5a2bf74))
* **catalog:** list titles by ID or year with series seasons, wanted episodes and next airing ([d81ab6e](https://github.com/bwees/magpie/commit/d81ab6ed6ed32dbfcddc211eda589746d5274ada))
* **catalog:** match library-scan folders to TMDB with a review queue ([cf38342](https://github.com/bwees/magpie/commit/cf38342c2960fb845851e45f64df9c4d6ff876ca))
* **catalog:** track series with episode groups, monitoring and wanted languages ([28a8d00](https://github.com/bwees/magpie/commit/28a8d006c5b42b9eddc23bae79ffc98d92e3c59e))
* **config:** refuse to start without MAGPIE_AGENT_SECRET ([0552557](https://github.com/bwees/magpie/commit/05525575a537d1099773a05185f46b74870361c3))
* **deploy:** add the example compose and a CI smoke test with a CPU agent ([4fc4ad0](https://github.com/bwees/magpie/commit/4fc4ad0a53668cf6cb8acc5311a92b164c89ba17))
* **deploy:** build the agent image with jellyfin-ffmpeg and Vulkan whisper.cpp ([5e3b93c](https://github.com/bwees/magpie/commit/5e3b93cd952e636fbe9f5d31ef16e1882e39c1e2))
* **dev:** add seed-request command that publishes RequestApproved ([97b9a73](https://github.com/bwees/magpie/commit/97b9a7318b2611e5e4f759a748f2803f8e759177))
* **identity:** add api contract, reader fake and jellyfin auth port ([9d31156](https://github.com/bwees/magpie/commit/9d3115617e398501d6badd1d19ee5ad5dce9acff))
* **identity:** add jellyfin and quick connect sign-in, users admin and defaults ([1c4420b](https://github.com/bwees/magpie/commit/1c4420bdae7548c63476d62559ee51935241f4d6))
* **identity:** add local admin setup, sign-in, sessions, setup gate and the user reader ([84b721b](https://github.com/bwees/magpie/commit/84b721b5443a137f0a75f8587f380d592677133b))
* **identity:** let self-authenticated paths past the session gate ([d5f97b7](https://github.com/bwees/magpie/commit/d5f97b720dfa145078168d4992e8197c4abe7013))
* **library:** add api contract, importer, reader fake and filesystem, prober, hasher and jellyfin ports ([eea4f1a](https://github.com/bwees/magpie/commit/eea4f1a66eaa318202860690fa57cdd19921a884))
* **library:** batch files by titles and coverage endpoints ([be3e216](https://github.com/bwees/magpie/commit/be3e2166f4d1a6295fffd587ea85ba592135ea73))
* **library:** delete files from the title page and drop a removed title's files ([e9c99cd](https://github.com/bwees/magpie/commit/e9c99cda21f1bb98e5c5477da1d58083405181c9))
* **library:** flag manual import failures and publish ImportRemoved on Remove & blocklist ([c4e5f82](https://github.com/bwees/magpie/commit/c4e5f82c1c0335b80e2ee729e9d3082aca4191b6))
* **library:** import finished downloads into the library ([0e4406f](https://github.com/bwees/magpie/commit/0e4406f1214f60bbd4e3f1868579a21027d1e108))
* **library:** link migrated root folders and files in place ([004302a](https://github.com/bwees/magpie/commit/004302ae2a3bf364733d896736f4fed9dd3d3252))
* **library:** publish FileRenamed with moved sidecars on episode-group renames ([7dce1c9](https://github.com/bwees/magpie/commit/7dce1c915990e3a7df74b52a8b1c1ff418b548a5))
* **library:** record each stream's start time from ffprobe ([94a6d9f](https://github.com/bwees/magpie/commit/94a6d9fc7fdd1e4a3eeb27be9dd7cd73131dbf6c))
* **library:** refresh Jellyfin after imports, confirm files from LibraryChanged and the reconcile, publish ItemAvailable ([ed81aec](https://github.com/bwees/magpie/commit/ed81aec0177a27f46c3424fd2a1c70bfe0cc5b21))
* **library:** replace an old file only when the import covers all its episodes ([806edcc](https://github.com/bwees/magpie/commit/806edcc18097694ca9797233f94a8c4762a8b35c))
* **library:** replace the old file when an import upgrades its episodes ([7559758](https://github.com/bwees/magpie/commit/75597581461d8edf672d5af557038ae76380c6d4))
* **library:** resolve Jellyfin items by ID, since Jellyfin 12 can't look items up by path ([59ea519](https://github.com/bwees/magpie/commit/59ea519ef0d026e1fb834ae96fbdbea2428a3ae9))
* **library:** scan root folders, link matched folders in place and rename files on episode-group changes ([bb3a073](https://github.com/bwees/magpie/commit/bb3a07354de0b42345059d1dd409c22375bcb8d3))
* **library:** swap verified transcodes in and reject changed or mismatched ones ([c785131](https://github.com/bwees/magpie/commit/c785131e9e9b7ebc1139eed2c326cfbdbdc2681d))
* **live:** filter by identity and requests readers with view-others toggle ([3ddefea](https://github.com/bwees/magpie/commit/3ddefeadfa8fdeb6be08a65bccf2e78805725b7a))
* **live:** push personal events like NotificationCreated only to their recipient ([6b36f3b](https://github.com/bwees/magpie/commit/6b36f3b46bf91b4a327fd2623b9a1f30b1dd69b7))
* **live:** send search topic events to subscribed admins only ([0558534](https://github.com/bwees/magpie/commit/05585341ee362deb78edafacd2c85b43ab3b94ff))
* **migrate:** plan, review and commit the Sonarr/Radarr import as a platform job ([a89f708](https://github.com/bwees/magpie/commit/a89f708a92c715ab372292b0275155310f58eb9a))
* **parse:** port sonarr and radarr release parser ([a51b46b](https://github.com/bwees/magpie/commit/a51b46bfae39e51bbb79ccfe6091a30ee92def26))
* **parse:** read language codes and subtitle file languages ([c625061](https://github.com/bwees/magpie/commit/c6250618a640e0983c964a03c0cbc84f224dff5b))
* **platform:** add a back up now endpoint with manual snapshots ([d307b24](https://github.com/bwees/magpie/commit/d307b24c8973445f6db01b8b9d2437acae27e00f))
* **platform:** add an SRT reader and writer ([726934c](https://github.com/bwees/magpie/commit/726934cd3568c81a15a6f0873cc5f4b6245d7679))
* **platform:** add connection settings endpoints and a health checker that publishes flips ([ca49de9](https://github.com/bwees/magpie/commit/ca49de976ca35ec9634ec5b07c4e23e478d96f48))
* **platform:** enqueue a job in a transaction to run at a later time ([0906df6](https://github.com/bwees/magpie/commit/0906df680e7843a0ee2475f347c7a54d099ed279))
* **platform:** read MAGPIE_AGENT_SECRET into the bootstrap env ([be219e5](https://github.com/bwees/magpie/commit/be219e5b90ad0f9285a3963c0b6be088ebafa372))
* **qbittorrent:** implement the WebAPI 5.2 adapter with remote path mapping ([3a9c2f4](https://github.com/bwees/magpie/commit/3a9c2f4b0845c97781dbb510bb9fa681075ac63d))
* **requests:** add api contract and reader fake ([10c0e5b](https://github.com/bwees/magpie/commit/10c0e5b1b3bb2d40cef07e8ee7d8b35f1278de10))
* **requests:** add RequestCancelled so a withdrawn request clears its attention item ([b384d2c](https://github.com/bwees/magpie/commit/b384d2c2201862323b4dd39fa4aceb5f509d795b))
* **requests:** add requests with approval, catalog follow-through and the real reader ([edf725b](https://github.com/bwees/magpie/commit/edf725b7818456b1b8b47255bd7f78d3736c8edd))
* **requests:** mark items available from ItemAvailable and publish RequestAvailable once per request ([f54121c](https://github.com/bwees/magpie/commit/f54121cc5163558b42ad602cd1cdc581ae29c329))
* **requests:** store issues with comments, fix actions and auto-resolve, behind /api/requests/issues ([15493fd](https://github.com/bwees/magpie/commit/15493fdec4240c11728efd66fb301496c4635c6b))
* **server:** add daily backups with admin list and restore endpoints ([a187396](https://github.com/bwees/magpie/commit/a18739612838549790e4bc1555e305aa6ab4f965))
* **server:** add live websocket gateway with resume, topics, heartbeats and role filter ([51d8060](https://github.com/bwees/magpie/commit/51d8060d19154684b1812990ae4e9df7574e66c4))
* **server:** add message interface, source key and scripted fake helper ([7885b0c](https://github.com/bwees/magpie/commit/7885b0c4deef9aae4d56ea7d31da6f72130fe335))
* **server:** add settings store and backlite jobs with periodic schedules and tickers ([6b5d810](https://github.com/bwees/magpie/commit/6b5d8102609d595e6271000b69b296bde8f90abc))
* **server:** add shared kernel ids, value types and principal ([a8da675](https://github.com/bwees/magpie/commit/a8da67557b5801a4b7ff86cd9deaff5ef99b97d1))
* **server:** add transactional outbox, per-handler dispatcher and in-process bus ([07e7d44](https://github.com/bwees/magpie/commit/07e7d4452ff8fa1e50a52eed796c72e4a1b7b958))
* **server:** let modules provide http middleware for every route ([00abb84](https://github.com/bwees/magpie/commit/00abb84c7276fa88422a7d9aa9ff3bb87eb548db))
* **server:** scaffold go server with fuego, fx, gormlite and goose ([a73647a](https://github.com/bwees/magpie/commit/a73647a0a5cebaacef94c9bf7cff851f5c2d5906))
* **server:** split db into writer and reader pools with per-owner migrations and backups ([2dc60ec](https://github.com/bwees/magpie/commit/2dc60ec10d136e0483bcf9b69516b995c77794a6))
* **server:** wire context modules, enforce context import boundaries and add flow-test harness ([a9c9a95](https://github.com/bwees/magpie/commit/a9c9a95783959212aab13e3b335b2aca9e72ab69))
* **server:** wire port fakes, check the event catalog and publish message schemas ([f3f20ec](https://github.com/bwees/magpie/commit/f3f20ec47e5fb90dfe9a15a9884a37e3239eab62))
* **subsync:** port ffsubsync timing sync to go ([bf6cadb](https://github.com/bwees/magpie/commit/bf6cadbd905b3d2758b35e22d6a3286d4bc3e5bc))
* **subtitles:** add api contract, reader fake and opensubtitles and syncer ports ([ac53790](https://github.com/bwees/magpie/commit/ac5379061313a64bab4f26ce2c1dd9b3d45fb9c6))
* **subtitles:** ask Whisper for original-language tracks and store its subtitles ([bd262d2](https://github.com/bwees/magpie/commit/bd262d2d6fe80d74a35a329d246adc8bd502d55e))
* **subtitles:** carry the file's title and episodes on SubtitleAdded and rerun an issue once ([115a483](https://github.com/bwees/magpie/commit/115a48379ea07a47c6a905057ef511b7840b2ec9))
* **subtitles:** find subtitles for imported files from embedded tracks, release sidecars and OpenSubtitles ([9633093](https://github.com/bwees/magpie/commit/963309382c362936d094895bc28db0e53a933884))
* **subtitles:** follow FileRenamed to keep stored sidecar paths current ([5e63f39](https://github.com/bwees/magpie/commit/5e63f3943b0e827c429bf3f07152d14110701ed9))
* **subtitles:** log in from connection config, add the sidecar port and Retry-After error ([b0c051d](https://github.com/bwees/magpie/commit/b0c051dca8774399f1d86a4883fc8f6cc6f2b77d))
* **subtitles:** turn Whisper off by default ([552cb3f](https://github.com/bwees/magpie/commit/552cb3f1d8ee31cf499b6e7f888c2406f906595f))
* **tmdb:** add rate-limited TMDB adapter with recorded fixtures ([362ae77](https://github.com/bwees/magpie/commit/362ae77f1ed9a530b4d739f6e40c4eacae894b01))
* **transcode:** add Activity agents cards and Settings agents section ([f1a7904](https://github.com/bwees/magpie/commit/f1a7904d73b8f5df25370d3849c134b933901c11))
* **transcode:** add api contract, run-agent-job command, agent protocol and file stat port ([70f4fff](https://github.com/bwees/magpie/commit/70f4fff77f27f64068802b855e1f5d589e18982f))
* **transcode:** add control path and draining to the agent protocol ([7495d04](https://github.com/bwees/magpie/commit/7495d0488582905f334ab470d65e9b97d1bde25d))
* **transcode:** calibrated policy, skip rules, exclusions and the backlog sweep ([c86e616](https://github.com/bwees/magpie/commit/c86e61645ab5c22d401f29096c8ac43c622e0dbe))
* **transcode:** queue, offer and verify Whisper jobs from RunAgentJob ([6dbf5d5](https://github.com/bwees/magpie/commit/6dbf5d539b006800ee7f1e016d387b03ec33eac9))
* **transcode:** queue, schedule, lease, transfer and verify transcode jobs ([4f8c10e](https://github.com/bwees/magpie/commit/4f8c10e18964c69e0f357ef7f7911e6d209248db))
* **transcode:** register agents over the control socket with liveness, retests, overrides and slot model ([ce5144d](https://github.com/bwees/magpie/commit/ce5144d08ee015d9d6926d585a71a40d2de30737))
* **transcode:** report prefetched jobs and a source missing from disk ([2d1fd56](https://github.com/bwees/magpie/commit/2d1fd5663d47c6d8f2a698e3e496a5e90f2e1f85))
* **transcode:** show agents in the activity tabs and settings layout ([48d2bee](https://github.com/bwees/magpie/commit/48d2bee7b80944140668005603847557731e5e88))
* **transcode:** sweep orphaned staged files at startup ([6ac987a](https://github.com/bwees/magpie/commit/6ac987ade1b70a9e35a7db793492decd3f42a9b5))
* **web:** 40 px buttons and rail items on touch screens ([152e5b8](https://github.com/bwees/magpie/commit/152e5b8eb46e083d46c71c82278a8ae70752bab9))
* **web:** add first-run wizard and settings general and connections pages ([edb3436](https://github.com/bwees/magpie/commit/edb3436485f3072649ca2d4b3808163ad054b9fe))
* **web:** add grab states, Activity downloads and blocklist tabs, and download settings ([746b60a](https://github.com/bwees/magpie/commit/746b60a9042658f04ced63ea77dfcfeacb9a91fa))
* **web:** add jellyfin and quick connect sign-in ([7134dcb](https://github.com/bwees/magpie/commit/7134dcbe519deb83bae7b63a4d75d6f6c620a0d5))
* **web:** add live socket client with resume, backoff and reconnecting indicator ([f3bf3b5](https://github.com/bwees/magpie/commit/f3bf3b5255f737867d98a030aa56607408d32154))
* **web:** add requester shell with role-based nav and admin-only redirects ([bba9e7f](https://github.com/bwees/magpie/commit/bba9e7f1f56a3ac60c0535b58cc3d3cf508abe27))
* **web:** add session gate, local admin sign-in and sign-out ([4b3f9d1](https://github.com/bwees/magpie/commit/4b3f9d148dbc86babac5a408d4467ee797adfb4c))
* **web:** add settings indexers page with inline per-indexer settings and sync ([3d793b4](https://github.com/bwees/magpie/commit/3d793b40b727f33398c1c9421b0029ef72870301))
* **web:** add settings quality page, wizard quality preset step and profile select ([bb94711](https://github.com/bwees/magpie/commit/bb94711989e7c3b629fb52d25d8a966a7c846a80))
* **web:** add settings users page with defaults and jellyfin sign-in switch ([5617f6d](https://github.com/bwees/magpie/commit/5617f6d58c401771dc55c6c5e10b8fc1095ed348))
* **web:** add shadcn table, select, alert, card and label components ([6747bcb](https://github.com/bwees/magpie/commit/6747bcb3035efd1e5be7c3626583bcc51ef2f5f8))
* **web:** add the Files tab, Activity imports with a mapping picker, and library and seeding settings ([88b4f6e](https://github.com/bwees/magpie/commit/88b4f6ee57d7c5e7e2d78dfff6594eb2ae225f47))
* **web:** add the interactive search dialog with quality buckets and tracker pill ([d6c469a](https://github.com/bwees/magpie/commit/d6c469a6d3e8678849f13d68e2a7a3df27d6f8ee))
* **web:** add the notification bell and settings notifications page ([ccc4258](https://github.com/bwees/magpie/commit/ccc425839ffb3f374308cb02bd49fa59405edb61))
* **web:** add the pipeline stage bar, title timeline and activity history tab ([1abbde9](https://github.com/bwees/magpie/commit/1abbde983379707caaabe9271a2cd812b2d79f36))
* **web:** add the requests page, search add and request with a season picker, and the pending badge ([eb54037](https://github.com/bwees/magpie/commit/eb54037795561e1fe3c3078e3de6a8d5f6b20dcd))
* **web:** add top-bar title search, title page shell and about page ([870ac94](https://github.com/bwees/magpie/commit/870ac94af174813799ab2351c6be86ea3904c0ff))
* **web:** automatic search buttons, search-on-add switch and acquisition schedules ([276b570](https://github.com/bwees/magpie/commit/276b5702c3e05a04831671c93ba1786e0a0af066))
* **web:** cutoff state and file delete on the title page's Files tab ([8d24c83](https://github.com/bwees/magpie/commit/8d24c83a10f4f729b2a96ac99ef672a2d9001d3d))
* **web:** exclude a title from transcoding on its Settings tab ([aa8cbea](https://github.com/bwees/magpie/commit/aa8cbea080b7b44213b79bb615f6b5256d78d7c6))
* **web:** GPU jobs tab with running, queued and failed jobs, live progress, cancel and retry ([f7665b7](https://github.com/bwees/magpie/commit/f7665b751c2d56968f83f2285b51005cb412b01b))
* **web:** Home, Movies and TV pages with live blocks and nav badges ([5f07bd4](https://github.com/bwees/magpie/commit/5f07bd46850d895d997842f3a2d423f6184baa89))
* **web:** Issues tab with threads and fixes, report issue on the title page, and the waiting-download warning on re-grab ([aa0c6dc](https://github.com/bwees/magpie/commit/aa0c6dcc2c8f3b3036076eeb65c5b0a5f12c99e9))
* **web:** label Whisper jobs with their language and show tracks waiting for Whisper ([b0e283a](https://github.com/bwees/magpie/commit/b0e283ae1fb943f048643fb4d8edfebf272f890a))
* **web:** library scan and review queue in Settings → Library, rename problems on the Files tab ([3a9f7e7](https://github.com/bwees/magpie/commit/3a9f7e7764263e94845445d8a71d47a81c1e956b))
* **web:** live request availability with progress, Jellyfin status on the Files tab and the reconcile schedule ([194d608](https://github.com/bwees/magpie/commit/194d608918d1df76a3c1187a3bb5e3f40e2c4849))
* **web:** mount the activity stage bar and timeline on the title page ([3e28c8b](https://github.com/bwees/magpie/commit/3e28c8bfe12d236957e24a1c2ef1513cf2a6f6d9))
* **web:** open the phone search overlay from the requester search tab ([592e7b5](https://github.com/bwees/magpie/commit/592e7b520be458dff2816ae2243252931d8471b4))
* **web:** pick the title's quality profile on the title page settings tab ([f9ad284](https://github.com/bwees/magpie/commit/f9ad2842ca8faf3b0aaca807687db4289b70fcd7))
* **web:** Remove & blocklist with a confirm and show downloads waiting for mapping ([0998a61](https://github.com/bwees/magpie/commit/0998a618bd851a881cfa703c696992a790eb6992))
* **web:** scaffold sveltekit spa with dark teal app shell ([b8bdb44](https://github.com/bwees/magpie/commit/b8bdb4426693267bac013678a64431e9690f263b))
* **web:** Settings → Transcode and skipped files on GPU jobs ([909cac7](https://github.com/bwees/magpie/commit/909cac77905fa3d941994109aa3ec9452485df81))
* **web:** show each episode's pipeline stage and available count on the title page ([c298ef3](https://github.com/bwees/magpie/commit/c298ef3467afaa09d013d6a9d9a38911120cbe11))
* **web:** show the title's quality profile in the title hero ([3538681](https://github.com/bwees/magpie/commit/3538681469397211b30127c5b20cee16dfad44bb))
* **web:** Sonarr/Radarr import wizard in first run and Settings → General ([aa0d2fe](https://github.com/bwees/magpie/commit/aa0d2fe4524aab30e5585c93d70b6e375c1977dc))
* **web:** stack the title hero, requests, downloads, releases and settings nav on tablets ([77e8ac1](https://github.com/bwees/magpie/commit/77e8ac14b6b1276309beedf9d6fbb113a7d3709a))
* **web:** subtitle tracks and a candidate picker on the Files tab, and subtitle settings with the quota badge ([cf1a20d](https://github.com/bwees/magpie/commit/cf1a20db6656277f1bb3b2dee0a2b540deceeb3c))


### Bug Fixes

* **acquisition:** answer a started search with 200 so its body goes out as JSON ([2f85607](https://github.com/bwees/magpie/commit/2f856079d8975eb52a72dfdbad2d647a696b5719))
* **acquisition:** answer POST /searches with 201 ([3a5e446](https://github.com/bwees/magpie/commit/3a5e446a990afd4c4b3a5d471e1bcb5ba797a349))
* **acquisition:** show the root cause when prowlarr is unreachable ([dcb5a6e](https://github.com/bwees/magpie/commit/dcb5a6e01b0d6e04bc38a74d4b2f7f51681ef5bc))
* **activity:** mark pipeline chips optional in the openapi schema ([3b83dcf](https://github.com/bwees/magpie/commit/3b83dcf258c17abe9ab09e3adf4a787f69298c87))
* **activity:** name services as their products spell them ([fc9b45b](https://github.com/bwees/magpie/commit/fc9b45bc6902c4796a7521b6c2f1513b38cc8661))
* **adapters:** never call OpenSubtitles from a build without the app key ([0f2c45e](https://github.com/bwees/magpie/commit/0f2c45ee4f5de99c68ae9ed82efff5f05b3d2de1))
* **adapters:** word qbittorrent probe errors like the other probes ([2382bd7](https://github.com/bwees/magpie/commit/2382bd738a4ad2f67a9316deeae0ff39872af430))
* **bus:** bound dispatcher stop by the stop context ([3fa253f](https://github.com/bwees/magpie/commit/3fa253f5bd0c4ef095b560d254e6b281ffeb1708))
* **db:** let a write waiting for the writer give up when its context ends ([c5573a6](https://github.com/bwees/magpie/commit/c5573a6e9e1613eabbdea279cb364bfae05a036e))
* **httpserver:** hold the response status until the body so non-200 routes keep their JSON content type ([c9fe27d](https://github.com/bwees/magpie/commit/c9fe27d665d896fec049963ce64843265fe86b2c))
* **jobs:** cancel running jobs at stop and bound backlite's shutdown ([c35c580](https://github.com/bwees/magpie/commit/c35c580131820d6b9a2d0764a859fff85d4b6be3))
* **jobs:** register queues after construction so event handlers can enqueue ([c282fc4](https://github.com/bwees/magpie/commit/c282fc406c5bcf3ec24b654e8d1d7783d14563e1))
* **library:** guess a scanned video's quality from its width when its name gives none ([a9740f2](https://github.com/bwees/magpie/commit/a9740f2a0f97218a0fb46e0c10d27fc04e398d00))
* **library:** keep the episodes of mapped videos when a pack needs manual mapping ([48ff3ee](https://github.com/bwees/magpie/commit/48ff3ee81ca2036a0b5ce4bb4600a7a46711e7c1))
* **prowlarr:** read pubDate in the GMT form Prowlarr writes ([f2d3adf](https://github.com/bwees/magpie/commit/f2d3adfdb56c2eee74875dbff0a528905b1ef905))
* **requests:** leave specials out of the whole-series monitored check ([519e34e](https://github.com/bwees/magpie/commit/519e34e4f1039288a413e3c7f1d089f2d7f9f766))
* **subtitles:** settle coverage for a file its import already covers ([fc556c7](https://github.com/bwees/magpie/commit/fc556c770635093618e33f59ad49aadcd3d689d5))
* **transcode:** attach an agent to the hub before sending its Welcome ([4720b75](https://github.com/bwees/magpie/commit/4720b7562fef926d2b797002d0379710a2022efe))
* **transcode:** close agent sockets when the server stops ([f25f14d](https://github.com/bwees/magpie/commit/f25f14d4725784849af4ee0397a7d704f957ff96))
* **transcode:** declare the agent socket self-authenticated ([468dcc3](https://github.com/bwees/magpie/commit/468dcc31e9df3d6eb5d4d2d70dfd1f0d2e483589))
* **transcode:** exclude an agent only for failed attempts, not lost leases ([7407e90](https://github.com/bwees/magpie/commit/7407e90c9028f35b458d8ddfc405e52f64f55c8f))
* **transcode:** make Lifetime safe to stop while agent requests are in flight ([1b139fd](https://github.com/bwees/magpie/commit/1b139fd98329efd4c6502d92850d3dc0310c8294))
* **transcode:** serve the source to an agent whose accept is still in flight ([7037de3](https://github.com/bwees/magpie/commit/7037de3354f9d5b618c8c0cbcc5958ce677efc3b))
* **transcode:** word the no-capable-agent skip detail ([699dd1e](https://github.com/bwees/magpie/commit/699dd1edf8bf3d66bfc7935ca39ab5909ebc0c1a))
* **web:** add the graphics capability to the sample agent docker run ([724a056](https://github.com/bwees/magpie/commit/724a056a1e79347f6047280fc8d4972cf18681ff))
* **web:** drop the duplicate Downloads heading in settings ([e02eb45](https://github.com/bwees/magpie/commit/e02eb45de4bceda375b120257133a06770e2b82e))
* **web:** name the series in the episode search toast ([ac856bd](https://github.com/bwees/magpie/commit/ac856bdaa46530a75fe03589b4e890b5c374d9e0))
* **web:** wrap the download path in Settings → Library on phones ([70148ee](https://github.com/bwees/magpie/commit/70148ee0c1e983a1702f6bcca2205ae5ab2a7546))
