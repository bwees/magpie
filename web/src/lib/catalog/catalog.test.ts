import { describe, expect, it } from 'vitest';
import { ImageWidth, sizedImage, titlePath } from './images';
import {
	NO_ACTIVITY_LABEL,
	PipelineStage,
	SegmentState,
	segmentStates,
	stepLabel
} from './pipeline';

describe('sizedImage', () => {
	it('adds the width to a cached image URL', () => {
		expect(sizedImage('/api/catalog/images/T1/POSTER', ImageWidth.POSTER)).toBe(
			'/api/catalog/images/T1/POSTER?w=342'
		);
	});

	it('replaces a width already on the URL', () => {
		expect(sizedImage('/api/catalog/images/T1/POSTER?w=92', ImageWidth.CARD)).toBe(
			'/api/catalog/images/T1/POSTER?w=185'
		);
	});

	it('is undefined for an image that is not cached', () => {
		expect(sizedImage(null, ImageWidth.THUMB)).toBeUndefined();
	});
});

describe('titlePath', () => {
	it('routes to the title page', () => {
		expect(titlePath('01J')).toBe('/titles/01J');
	});
});

describe('pipeline bar', () => {
	it('labels the current step out of five', () => {
		expect(stepLabel(PipelineStage.DOWNLOADING)).toBe('Downloading · step 3 of 5');
	});

	it('marks earlier stages done and later ones pending', () => {
		expect(segmentStates(PipelineStage.SEARCHING)).toEqual([
			SegmentState.DONE,
			SegmentState.CURRENT,
			SegmentState.PENDING,
			SegmentState.PENDING,
			SegmentState.PENDING
		]);
	});

	it('is empty until Activity reports a stage', () => {
		expect(segmentStates(undefined)).toEqual(Array(5).fill(SegmentState.PENDING));
		expect(stepLabel(undefined)).toBe(NO_ACTIVITY_LABEL);
	});
});
