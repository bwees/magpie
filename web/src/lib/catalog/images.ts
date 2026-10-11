/** Widths the Catalog image endpoint resizes to; it rounds any other width up to one of these. */
export enum ImageWidth {
	THUMB = 92,
	CARD = 185,
	POSTER = 342,
	BACKDROP = 1280
}

const WIDTH_PARAM = 'w';

/** A cached Catalog image URL resized to width, or undefined when the image isn't cached. */
export function sizedImage(url: string | null | undefined, width: ImageWidth): string | undefined {
	if (!url) {
		return undefined;
	}
	const [path, query = ''] = url.split('?');
	const params = new URLSearchParams(query);
	params.set(WIDTH_PARAM, String(width));
	return `${path}?${params}`;
}

/** The route of a Title's page. */
export function titlePath(id: string): string {
	return `/titles/${id}`;
}
