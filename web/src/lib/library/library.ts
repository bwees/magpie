import type {
	EpisodeRefResponse,
	ImportResponse,
	MediaFileResponse,
	StreamResponse
} from '$lib/sdk';

/** Mirrors the Go `library.ImportStatus` values. */
export enum ImportStatus {
	PENDING = 'PENDING',
	IMPORTED = 'IMPORTED',
	FAILED = 'FAILED',
	MANUAL = 'MANUAL'
}

/** The statuses Activity → Imports lists. */
export const STOPPED_IMPORTS = [ImportStatus.FAILED, ImportStatus.MANUAL];

/** Mirrors the Go `library.FileKind` values. */
export enum FileKind {
	VIDEO = 'VIDEO',
	SUBTITLE = 'SUBTITLE'
}

/** Mirrors the Go `library.LinkMode` values of the hardlink check. */
export enum LinkMode {
	HARDLINK = 'HARDLINK',
	COPY = 'COPY',
	UNKNOWN = 'UNKNOWN'
}

/** Mirrors the Go `domain.MediaKind` values. */
export enum MediaKind {
	MOVIE = 'MOVIE',
	SERIES = 'SERIES'
}

/** Mirrors the Go `api.StreamKind` values. */
export enum StreamKind {
	VIDEO = 'VIDEO',
	AUDIO = 'AUDIO',
	SUBTITLE = 'SUBTITLE'
}

/** Mirrors the Go `api.HDRFormat` values. */
export enum HdrFormat {
	NONE = 'NONE',
	HDR10 = 'HDR10',
	HDR10_PLUS = 'HDR10_PLUS',
	DOLBY_VISION = 'DOLBY_VISION',
	HLG = 'HLG'
}

export enum LibraryEvent {
	FILE_IMPORTED = 'library.FileImported',
	IMPORT_FAILED = 'library.ImportFailed'
}

export const IMPORT_STATUS_LABELS: Record<string, string> = {
	[ImportStatus.PENDING]: 'Importing',
	[ImportStatus.IMPORTED]: 'Imported',
	[ImportStatus.FAILED]: 'Failed',
	[ImportStatus.MANUAL]: 'Needs mapping'
};

export const KIND_LABELS: Record<string, string> = {
	[MediaKind.MOVIE]: 'Movies',
	[MediaKind.SERIES]: 'TV'
};

export const LINK_LABELS: Record<string, string> = {
	[LinkMode.HARDLINK]: 'Hardlinks',
	[LinkMode.COPY]: 'Copies',
	[LinkMode.UNKNOWN]: 'Unknown'
};

export const HDR_LABELS: Record<string, string> = {
	[HdrFormat.HDR10]: 'HDR10',
	[HdrFormat.HDR10_PLUS]: 'HDR10+',
	[HdrFormat.DOLBY_VISION]: 'Dolby Vision',
	[HdrFormat.HLG]: 'HLG'
};

/** The fixed Jellyfin naming, from 12-settings › Library. */
export const NAMING_EXAMPLES = [
	'Movies/Title (Year) [tmdbid-N]/Title (Year).mkv',
	'TV/Show (Year) [tmdbid-N]/Season 01/Show S01E02.mkv'
];

const SECONDS_PER_MINUTE = 60;
const MINUTES_PER_HOUR = 60;
const UNDETERMINED_LANGUAGE = 'und';
const TWO_DIGITS = 2;

function pad(n: number): string {
	return String(n).padStart(TWO_DIGITS, '0');
}

/** "S01E02", or "S01E02–E03" for a multi-episode file. */
export function episodeLabel(episodes: EpisodeRefResponse[]): string {
	if (episodes.length === 0) {
		return '';
	}
	const first = episodes[0];
	const last = episodes[episodes.length - 1];
	const start = `S${pad(first.season)}E${pad(first.number)}`;
	return last === first ? start : `${start}–E${pad(last.number)}`;
}

/** "2h 35m" or "42m". */
export function formatDuration(seconds: number): string {
	const minutes = Math.round(seconds / SECONDS_PER_MINUTE);
	const hours = Math.floor(minutes / MINUTES_PER_HOUR);
	const rest = minutes % MINUTES_PER_HOUR;
	return hours > 0 ? `${hours}h ${rest}m` : `${rest}m`;
}

function languageOf(stream: StreamResponse): string {
	return stream.language && stream.language !== UNDETERMINED_LANGUAGE
		? stream.language.toUpperCase()
		: '?';
}

/** "HEVC 2160p · 10-bit · HDR10". */
export function videoSummary(file: MediaFileResponse): string {
	const video = file.mediaInfo.streams.find((s) => s.kind === StreamKind.VIDEO);
	if (!video) {
		return '—';
	}
	const parts = [video.codec.toUpperCase()];
	if (file.mediaInfo.height > 0) {
		parts[0] += ` ${file.mediaInfo.height}p`;
	}
	if (video.bitDepth > 0) {
		parts.push(`${video.bitDepth}-bit`);
	}
	const hdr = HDR_LABELS[file.mediaInfo.hdr];
	if (hdr) {
		parts.push(hdr);
	}
	return parts.join(' · ');
}

/** One line per audio track: "EN · EAC3 5.1". */
export function audioTracks(file: MediaFileResponse): string[] {
	return file.mediaInfo.streams
		.filter((s) => s.kind === StreamKind.AUDIO)
		.map((s) => {
			const channels = s.channels > 0 ? ` ${channelLayout(s.channels)}` : '';
			return `${languageOf(s)} · ${s.codec.toUpperCase()}${channels}`;
		});
}

const STEREO_CHANNELS = 2;
const LFE_CHANNELS = 1;

/** 6 channels read as 5.1, 8 as 7.1, 2 as 2.0. */
function channelLayout(channels: number): string {
	return channels > STEREO_CHANNELS
		? `${channels - LFE_CHANNELS}.${LFE_CHANNELS}`
		: `${channels}.0`;
}

/** One line per subtitle track: "EN · SUBRIP · forced". */
export function subtitleTracks(file: MediaFileResponse): string[] {
	return file.mediaInfo.streams
		.filter((s) => s.kind === StreamKind.SUBTITLE)
		.map((s) => {
			const parts = [languageOf(s), s.codec.toUpperCase()];
			if (s.forced) {
				parts.push('forced');
			}
			if (s.image) {
				parts.push('image');
			}
			return parts.join(' · ');
		});
}

/** The videos of an import, which the mapping picker lists. */
export function importVideos(imp: ImportResponse) {
	return imp.files.filter((f) => f.kind === FileKind.VIDEO);
}

/** The file name without its folders. */
export function baseName(path: string): string {
	return path.split('/').at(-1) ?? path;
}
