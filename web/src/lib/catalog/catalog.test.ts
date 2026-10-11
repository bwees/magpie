import { describe, expect, it } from 'vitest';
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
