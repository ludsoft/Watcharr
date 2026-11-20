import type { AnalyticsCounts } from "@/types";
/** Consistent 3 letter month names for chart axes. */
export const monthAbbr = [
	"Jan",
	"Feb",
	"Mar",
	"Apr",
	"May",
	"Jun",
	"Jul",
	"Aug",
	"Sep",
	"Oct",
	"Nov",
	"Dec",
];

/** Parse a `YYYY-MM-DD` day as a local date (no timezone shift). */
export function parseDay(d: string): Date {
	const [y, m, day] = d.split("-").map(Number);
	return new Date(y, m - 1, day);
}

/** Format a local date as `YYYY-MM-DD`. */
export function dayKey(d: Date): string {
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

/** `YYYY-MM` -> `Mar 2026`. */
export function formatMonth(m: string): string {
	const [y, mo] = m.split("-").map(Number);
	return `${monthAbbr[mo - 1]} ${y}`;
}

export function plural(n: number, one: string, many: string) {
	return `${n.toLocaleString()} ${n === 1 ? one : many}`;
}

/** Human readable summary of a days/months counts. */
export function describeCounts(c: AnalyticsCounts): string {
	const parts: string[] = [];
	if (c.movies) parts.push(plural(c.movies, "film", "films"));
	if (c.episodes) parts.push(plural(c.episodes, "episode", "episodes"));
	if (c.games) parts.push(plural(c.games, "game", "games"));
	return parts.length ? parts.join(", ") : "No activity";
}

/** Minutes -> `45m`, `3h 20m`, `43h`. */
export function formatMinutes(min: number): string {
	if (min < 60) return `${min}m`;
	const h = Math.floor(min / 60);
	const m = min % 60;
	// Minutes stop mattering (and stop fitting in a tile) past 10h.
	if (h >= 10 || m === 0) return `${h.toLocaleString()}h`;
	return `${h}h ${m}m`;
}

/** Monday first weekday index (0-6). */
export const weekday = (d: Date) => (d.getDay() + 6) % 7;

export interface HeatmapYear {
	year: number;
	/** Number of week columns. */
	weeks: number;
	days: { date: string; col: number; row: number }[];
	/** Month labels and the week column they start at. */
	months: { col: number; label: string }[];
}

/**
 * Lay out the days between `start` and `end` (YYYY-MM-DD, inclusive) as
 * one week-column grid per year, newest year first.
 */
export function heatmapLayout(start: string, end: string): HeatmapYear[] {
	const s = parseDay(start);
	const e = parseDay(end);
	const years: HeatmapYear[] = [];
	for (let y = e.getFullYear(); y >= s.getFullYear(); y--) {
		const from = new Date(Math.max(s.getTime(), new Date(y, 0, 1).getTime()));
		const to = new Date(Math.min(e.getTime(), new Date(y, 11, 31).getTime()));
		const gridStart = new Date(from);
		gridStart.setDate(from.getDate() - weekday(from));

		const row: HeatmapYear = { year: y, weeks: 0, days: [], months: [] };
		for (const d = new Date(from); d <= to; d.setDate(d.getDate() + 1)) {
			// Round, since DST makes some days 23/25 hours long.
			const idx = Math.round((d.getTime() - gridStart.getTime()) / 86400000);
			const col = Math.floor(idx / 7);
			row.weeks = col + 1;
			row.days.push({ date: dayKey(d), col, row: weekday(d) });
			if (d.getDate() === 1 || d.getTime() === from.getTime()) {
				row.months.push({ col, label: monthAbbr[d.getMonth()] });
			}
		}
		years.push(row);
	}
	return years;
}
