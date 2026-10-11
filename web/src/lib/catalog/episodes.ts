/** TMDB's season for specials. */
export const SPECIALS_SEASON = 0;

/** Shown for a season's available count until Library reports coverage. */
export const AVAILABLE_UNKNOWN = '—';

const airDateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeZone: 'UTC' });

export function seasonLabel(number: number): string {
	return number === SPECIALS_SEASON ? 'Specials' : `Season ${number}`;
}

/** Air dates are calendar dates, so they are shown without a time zone shift. */
export function airDateLabel(airDate: string | null): string {
	return airDate ? airDateFormat.format(new Date(airDate)) : 'TBA';
}
