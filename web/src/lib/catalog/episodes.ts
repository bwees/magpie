/** TMDB's season for specials. */
export const SPECIALS_SEASON = 0;

/** Shown for a season's available count until Library reports coverage. */
export const AVAILABLE_UNKNOWN = '—';

const airDateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeZone: 'UTC' });

export function seasonLabel(number: number): string {
	return number === SPECIALS_SEASON ? 'Specials' : `Season ${number}`;
}

const EPISODE_NUMBER_DIGITS = 2;

function padded(number: number): string {
	return String(number).padStart(EPISODE_NUMBER_DIGITS, '0');
}

/** The release-name style code, e.g. "S01E03". */
export function episodeCode(season: number, number: number): string {
	return `S${padded(season)}E${padded(number)}`;
}

/** Air dates are calendar dates, so they are shown without a time zone shift. */
export function airDateLabel(airDate: string | null): string {
	return airDate ? airDateFormat.format(new Date(airDate)) : 'TBA';
}
