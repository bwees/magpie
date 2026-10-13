import type { PipelineRowResponse, SubsChipResponse, TranscodeChipResponse } from '$lib/sdk';
import { JobState } from '$lib/transcode/gpu-jobs';

// Mirrors server/internal/activity/api: stage, chip and severity values.

export enum Stage {
	REQUESTED = 'REQUESTED',
	SEARCHING = 'SEARCHING',
	DOWNLOADING = 'DOWNLOADING',
	IMPORTING = 'IMPORTING',
	AVAILABLE = 'AVAILABLE'
}

export const STAGES: Stage[] = [
	Stage.REQUESTED,
	Stage.SEARCHING,
	Stage.DOWNLOADING,
	Stage.IMPORTING,
	Stage.AVAILABLE
];

export const STAGE_LABELS: Record<Stage, string> = {
	[Stage.REQUESTED]: 'Requested',
	[Stage.SEARCHING]: 'Searching',
	[Stage.DOWNLOADING]: 'Downloading',
	[Stage.IMPORTING]: 'Importing',
	[Stage.AVAILABLE]: 'Available'
};

export enum SubsState {
	PENDING = 'PENDING',
	SETTLED = 'SETTLED'
}

export enum TranscodeState {
	QUEUED = 'QUEUED',
	RUNNING = 'RUNNING',
	DONE = 'DONE',
	SKIPPED = 'SKIPPED',
	FAILED = 'FAILED'
}

export enum Severity {
	INFO = 'INFO',
	WARN = 'WARN',
	ERROR = 'ERROR'
}

export enum SegmentState {
	DONE = 'DONE',
	CURRENT = 'CURRENT',
	TODO = 'TODO'
}

/** Severities shown with the failure styling: rejections, failures and nothing-accepted searches. */
export const ALERT_SEVERITIES: string[] = [Severity.WARN, Severity.ERROR];

const LANGUAGE_SEPARATOR = ', ';
const MISSING_SEPARATOR = ' · ';

function stageIndex(stage: string): number {
	return STAGES.indexOf(stage as Stage);
}

/** "Downloading · step 3 of 5", the phone stage bar's heading. */
export function phoneStageLabel(stage: Stage): string {
	return `${STAGE_LABELS[stage]} · step ${stageIndex(stage) + 1} of ${STAGES.length}`;
}

/** Each segment's fill. Available is the last stage, so reaching it completes the bar. */
export function stageProgress(stage: Stage): SegmentState[] {
	const current = stageIndex(stage);
	return STAGES.map((_, index) => {
		if (index < current || stage === Stage.AVAILABLE) return SegmentState.DONE;
		return index === current ? SegmentState.CURRENT : SegmentState.TODO;
	});
}

export function subsChipText(chip: SubsChipResponse): string {
	if (chip.state === SubsState.PENDING) return 'Subs …';
	const satisfied = chip.satisfied.join(LANGUAGE_SEPARATOR);
	const missing = chip.missing.length ? `${chip.missing.join(LANGUAGE_SEPARATOR)} missing` : '';
	return `Subs ✓ ${[satisfied, missing].filter(Boolean).join(MISSING_SEPARATOR)}`;
}

/** A running job's live progress: its state and the percent through that state. */
export interface LiveJob {
	state: string;
	percent: number;
}

/** live overlays the running job's progress, which is never stored. */
export function transcodeChipText(chip: TranscodeChipResponse, live?: LiveJob): string {
	switch (chip.state) {
		case TranscodeState.QUEUED:
			return 'Transcode queued';
		case TranscodeState.RUNNING:
			if (!live) return 'Transcoding';
			return live.state === JobState.CHECKING
				? `Transcode check ${live.percent}%`
				: `Transcode ${live.percent}%`;
		case TranscodeState.DONE:
			return chip.savedPercent === null ? 'Transcoded' : `Transcoded −${chip.savedPercent}%`;
		case TranscodeState.SKIPPED:
			return 'Original kept';
		default:
			return 'Transcode failed';
	}
}

export interface PipelineSummary {
	/** The least-advanced row, first in episode order on ties. */
	row: PipelineRowResponse;
	subs?: SubsChipResponse;
	transcode?: TranscodeChipResponse;
}

/** The most pressing transcode state first: what a series' chip shows when its episodes differ. */
const TRANSCODE_PRIORITY: string[] = [
	TranscodeState.RUNNING,
	TranscodeState.FAILED,
	TranscodeState.QUEUED,
	TranscodeState.DONE,
	TranscodeState.SKIPPED
];

function summarizeSubs(chips: SubsChipResponse[]): SubsChipResponse | undefined {
	if (chips.length === 0) return undefined;
	if (chips.some((chip) => chip.state === SubsState.PENDING)) {
		return { state: SubsState.PENDING, satisfied: [], missing: [] };
	}
	const missing = [...new Set(chips.flatMap((chip) => chip.missing))];
	const satisfied = [...new Set(chips.flatMap((chip) => chip.satisfied))].filter(
		(language) => !missing.includes(language)
	);
	return { state: SubsState.SETTLED, satisfied, missing };
}

function summarizeTranscode(chips: TranscodeChipResponse[]): TranscodeChipResponse | undefined {
	const [first] = [...chips].sort(
		(a, b) => TRANSCODE_PRIORITY.indexOf(a.state) - TRANSCODE_PRIORITY.indexOf(b.state)
	);
	if (!first || first.state !== TranscodeState.DONE) return first;
	const saved = chips
		.filter((chip) => chip.state === TranscodeState.DONE && chip.savedPercent !== null)
		.map((chip) => chip.savedPercent as number);
	const savedPercent = saved.length
		? Math.round(saved.reduce((sum, percent) => sum + percent, 0) / saved.length)
		: null;
	return { ...first, savedPercent };
}

/** One movie's row, or a series summarised across its episodes. */
export function summarize(rows: PipelineRowResponse[]): PipelineSummary | undefined {
	if (rows.length === 0) return undefined;
	const row = rows.reduce((least, candidate) =>
		stageIndex(candidate.stage) < stageIndex(least.stage) ? candidate : least
	);
	return {
		row,
		subs: summarizeSubs(rows.flatMap((r) => (r.subs ? [r.subs] : []))),
		transcode: summarizeTranscode(rows.flatMap((r) => (r.transcode ? [r.transcode] : [])))
	};
}
