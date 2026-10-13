import { describe, expect, it } from 'vitest';
import { episodeCode, newestSeasonsFirst } from './episodes';
import { ImageWidth, sizedImage, titlePath } from './images';

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

describe('episodeCode', () => {
	it('pads the season and episode', () => {
		expect(episodeCode(1, 3)).toBe('S01E03');
		expect(episodeCode(12, 104)).toBe('S12E104');
	});
});

describe('newestSeasonsFirst', () => {
	it('puts the latest season first and specials last', () => {
		const seasons = [0, 1, 3, 2].map((number) => ({ number }));
		expect(newestSeasonsFirst(seasons).map((season) => season.number)).toEqual([3, 2, 1, 0]);
		expect(seasons.map((season) => season.number)).toEqual([0, 1, 3, 2]);
	});
});
