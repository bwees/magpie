import { describe, expect, it } from 'vitest';
import type { PipelineRowResponse } from '$lib/sdk';
import {
	Stage,
	SubsState,
	TranscodeState,
	phoneStageLabel,
	stageProgress,
	subsChipText,
	summarize,
	transcodeChipText
} from './pipeline';

const TITLE = '01TITLE';
const UPDATED = '2026-10-11T12:00:00Z';
const SAVED_PERCENT = 62;
const LIVE_PERCENT = 41;
const EPISODE_SAVED_PERCENTS = [60, 40];
const AVERAGE_SAVED_PERCENT = 50;

function row(overrides: Partial<PipelineRowResponse> = {}): PipelineRowResponse {
	return {
		titleId: TITLE,
		episodeId: null,
		season: 0,
		number: 0,
		downloadId: null,
		stage: Stage.REQUESTED,
		failed: false,
		detail: '',
		updatedAt: UPDATED,
		...overrides
	};
}

describe('stage bar', () => {
	it('labels the stage for phones as a step of five', () => {
		expect(phoneStageLabel(Stage.DOWNLOADING)).toBe('Downloading · step 3 of 5');
		expect(phoneStageLabel(Stage.AVAILABLE)).toBe('Available · step 5 of 5');
	});

	it('fills done segments and half-fills the current one', () => {
		expect(stageProgress(Stage.DOWNLOADING)).toEqual(['DONE', 'DONE', 'CURRENT', 'TODO', 'TODO']);
		expect(stageProgress(Stage.AVAILABLE)).toEqual(['DONE', 'DONE', 'DONE', 'DONE', 'DONE']);
	});
});

describe('chips', () => {
	it('reads subtitle coverage', () => {
		expect(subsChipText({ state: SubsState.PENDING, satisfied: [], missing: [] })).toBe('Subs …');
		expect(subsChipText({ state: SubsState.SETTLED, satisfied: ['en', 'es'], missing: [] })).toBe(
			'Subs ✓ en, es'
		);
		expect(subsChipText({ state: SubsState.SETTLED, satisfied: ['en'], missing: ['es'] })).toBe(
			'Subs ✓ en · es missing'
		);
	});

	it('reads the transcode state, with the live percent while running', () => {
		const chip = (state: TranscodeState, savedPercent: number | null = null) => ({
			state,
			savedPercent,
			reason: '',
			jobId: null
		});
		expect(transcodeChipText(chip(TranscodeState.QUEUED))).toBe('Transcode queued');
		expect(transcodeChipText(chip(TranscodeState.RUNNING), LIVE_PERCENT)).toBe('Transcode 41%');
		expect(transcodeChipText(chip(TranscodeState.RUNNING))).toBe('Transcoding');
		expect(transcodeChipText(chip(TranscodeState.DONE, SAVED_PERCENT))).toBe('Transcoded −62%');
		expect(transcodeChipText(chip(TranscodeState.SKIPPED))).toBe('Original kept');
		expect(transcodeChipText(chip(TranscodeState.FAILED))).toBe('Transcode failed');
	});
});

describe('summarize', () => {
	it('is empty without rows', () => {
		expect(summarize([])).toBeUndefined();
	});

	it('takes the least advanced episode, first in order on ties', () => {
		const summary = summarize([
			row({ episodeId: 'e1', stage: Stage.AVAILABLE }),
			row({ episodeId: 'e2', stage: Stage.SEARCHING, failed: true, detail: 'No accepted release' }),
			row({ episodeId: 'e3', stage: Stage.SEARCHING })
		]);
		expect(summary?.row.episodeId).toBe('e2');
		expect(summary?.row.failed).toBe(true);
	});

	it('summarises subtitle chips across episodes', () => {
		const settled = (satisfied: string[], missing: string[]) => ({
			state: SubsState.SETTLED,
			satisfied,
			missing
		});
		expect(
			summarize([
				row({ episodeId: 'e1', subs: settled(['en', 'es'], []) }),
				row({ episodeId: 'e2', subs: settled(['en'], ['es']) })
			])?.subs
		).toEqual(settled(['en'], ['es']));
		expect(
			summarize([
				row({ episodeId: 'e1', subs: settled(['en'], []) }),
				row({ episodeId: 'e2', subs: { state: SubsState.PENDING, satisfied: [], missing: [] } })
			])?.subs?.state
		).toBe(SubsState.PENDING);
		expect(summarize([row()])?.subs).toBeUndefined();
	});

	it('summarises transcode chips by the most pressing state', () => {
		const chip = (state: TranscodeState, savedPercent: number | null = null) => ({
			state,
			savedPercent,
			reason: '',
			jobId: null
		});
		const [first, second] = EPISODE_SAVED_PERCENTS;
		expect(
			summarize([
				row({ episodeId: 'e1', transcode: chip(TranscodeState.DONE, first) }),
				row({ episodeId: 'e2', transcode: chip(TranscodeState.RUNNING) })
			])?.transcode?.state
		).toBe(TranscodeState.RUNNING);
		expect(
			summarize([
				row({ episodeId: 'e1', transcode: chip(TranscodeState.DONE, first) }),
				row({ episodeId: 'e2', transcode: chip(TranscodeState.DONE, second) })
			])?.transcode
		).toEqual(chip(TranscodeState.DONE, AVERAGE_SAVED_PERCENT));
	});
});
