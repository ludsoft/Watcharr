/**
 * Fork (ludsoft): Netflix viewing history import.
 *
 * Parses `NetflixViewingHistory.csv` (columns `Title,Date`, one row per movie
 * or episode watched) into one import item per movie/show. The server does
 * the matching (see server/feature/imprt/import_netflix.go).
 *
 * Title formats:
 * - Movie: `Rush Hour`
 * - Episode: `Stranger Things: Season 1: Chapter One: The Vanishing...`
 *   (the season part can also be `Part 2`, `Limited Series`, ...)
 * - Ambiguous: `Wrath: Ghost` (episode of a show without seasons) or
 *   `Gran Turismo: Based on a True Story` (movie with a subtitle).
 */
import papa from "papaparse";
import type { ImportedList, NetflixEpisode } from "@/types";

export interface NetflixParseResult {
	items: ImportedList[];
	rows: number;
	movies: number;
	shows: number;
	episodes: number;
	// Rows that couldn't be used (no show name, invalid date).
	skipped: number;
	// True when no date tells if days or months come first (assumed M/D).
	dateOrderAmbiguous: boolean;
}

interface Row {
	title: string;
	date: Date;
}

const SEASON_RE =
	/^(?:season|series|part|volume|vol\.?|chapter|book|collection)\s+(?:\d+|[ivx]+)$/i;
// "(Dubbed Version)", "(Extended Version)": not part of the TMDB title.
const VERSION_SUFFIX_RE =
	/\s*\((?:[^()]*\s)?(?:version|dubbed|subtitled)\)\s*$/i;
const LIMITED_RE = /^(?:limited series|mini ?series)$/i;

function isSeasonLabel(part: string, showName: string): boolean {
	const p = part.trim();
	if (SEASON_RE.test(p) || LIMITED_RE.test(p)) {
		return true;
	}
	// A bare number, but not a show name part like "Physical: 100".
	if (/^\d+$/.test(p)) {
		return Number(p) < 50;
	}
	// "Stranger Things: Stranger Things 2: ..."
	const show = showName.trim().toLowerCase();
	const lp = p.toLowerCase();
	return lp.startsWith(show + " ") && /^\d+$/.test(lp.slice(show.length + 1));
}

/**
 * Netflix writes dates in the account's locale (eg: 11/18/26 or 18/11/2026).
 * Decide the order over the whole file.
 */
function detectDayFirst(rawDates: string[]): {
	dayFirst: boolean;
	ambiguous: boolean;
} {
	let firstOver12 = false;
	let secondOver12 = false;
	for (const d of rawDates) {
		const p = d.split(/[/.-]/);
		if (p.length !== 3 || p[0].length === 4) continue;
		if (Number(p[0]) > 12) firstOver12 = true;
		if (Number(p[1]) > 12) secondOver12 = true;
	}
	if (firstOver12 && secondOver12) {
		throw new Error("dates are in an unknown format");
	}
	return {
		dayFirst: firstOver12,
		ambiguous: !firstOver12 && !secondOver12,
	};
}

function parseDate(raw: string, dayFirst: boolean): Date | undefined {
	const p = raw.trim().split(/[/.-]/);
	if (p.length !== 3) return undefined;
	let y: number, m: number, d: number;
	if (p[0].length === 4) {
		[y, m, d] = p.map(Number);
	} else {
		const [a, b, c] = p.map(Number);
		[d, m] = dayFirst ? [a, b] : [b, a];
		y = c < 100 ? 2000 + c : c;
	}
	if (!y || !m || !d || m > 12 || d > 31) return undefined;
	// Noon local time so the day doesn't shift with time zones.
	return new Date(y, m - 1, d, 12);
}

export function parseNetflixCsv(text: string): NetflixParseResult {
	const parsed = papa.parse<string[]>(text.replace(/^\uFEFF/, "").trim(), {
		skipEmptyLines: true,
	});
	const [header, ...data] = parsed.data;
	if (
		!header ||
		header[0]?.trim().toLowerCase() !== "title" ||
		header[1]?.trim().toLowerCase() !== "date"
	) {
		throw new Error(
			"not a Netflix viewing history file (expected Title,Date columns)",
		);
	}

	const { dayFirst, ambiguous } = detectDayFirst(data.map((r) => r[1] ?? ""));
	let skipped = 0;
	const rows: Row[] = [];
	for (const r of data) {
		const date = parseDate(r[1] ?? "", dayFirst);
		const title = (r[0] ?? "").replace(VERSION_SUFFIX_RE, "").trim();
		if (!date || !title) {
			skipped++;
			continue;
		}
		rows.push({ title, date });
	}

	// Movies (title -> dates), shows with a season label (name -> episodes),
	// and rows without a season label grouped by their prefix.
	const movies = new Map<string, Date[]>();
	const shows = new Map<string, NetflixEpisode[]>();
	const noLabel = new Map<string, { parts: string[]; row: Row }[]>();
	for (const row of rows) {
		const parts = row.title.split(": ");
		if (parts.length === 1) {
			movies.set(row.title, [...(movies.get(row.title) ?? []), row.date]);
			continue;
		}
		if (!parts[0].trim()) {
			// Netflix sometimes leaves the show name out (" : Episode 4").
			skipped++;
			continue;
		}
		let labelIdx = -1;
		for (let i = 1; i < parts.length - 1; i++) {
			if (isSeasonLabel(parts[i], parts.slice(0, i).join(": "))) {
				labelIdx = i;
				break;
			}
		}
		if (labelIdx > 0) {
			const name = parts.slice(0, labelIdx).join(": ");
			shows.set(name, [
				...(shows.get(name) ?? []),
				{
					seasonLabel: parts[labelIdx],
					title: parts.slice(labelIdx + 1).join(": "),
					date: row.date,
				},
			]);
			continue;
		}
		noLabel.set(parts[0], [...(noLabel.get(parts[0]) ?? []), { parts, row }]);
	}

	const items: ImportedList[] = [];
	for (const [name, dates] of movies) {
		items.push({
			name,
			type: "movie",
			status: "FINISHED",
			netflix: { dates },
		});
	}

	const toEpisode = (parts: string[], date: Date): NetflixEpisode =>
		parts.length >= 3
			? {
					seasonLabel: parts[1],
					title: parts.slice(2).join(": "),
					date,
				}
			: { seasonLabel: "", title: parts.slice(1).join(": "), date };

	for (const [prefix, list] of noLabel) {
		const episodes = list.map((l) => toEpisode(l.parts, l.row.date));
		// A show with season labels elsewhere in the file: same show.
		const labelled = shows.get(prefix);
		if (labelled) {
			labelled.push(...episodes);
			continue;
		}
		const rowTitles = [...new Set(list.map((l) => l.row.title))];
		const dates = list.map((l) => l.row.date);
		if (rowTitles.length >= 2) {
			// A show without seasons, or movies of a franchise: the server
			// checks for movies and asks us to split the group if needed.
			const second = list[0].parts[1];
			const sharedSecond = list.every(
				(l) => l.parts.length >= 3 && l.parts[1] === second,
			);
			const showNames = sharedSecond
				? [`${prefix}: ${second}`, prefix]
				: [prefix];
			items.push({
				name: showNames[0],
				type: "tv",
				status: "FINISHED",
				netflix: { group: true, rowTitles, showNames, episodes, dates },
				netflixRows: list.map((l) => l.row),
			});
			continue;
		}
		// Single title: a movie with a subtitle, or a one-episode show.
		items.push({
			name: rowTitles[0],
			type: "movie",
			status: "FINISHED",
			netflix: { dates, fallbackShow: { name: prefix, episodes } },
		});
	}

	let episodes = 0;
	for (const [name, eps] of shows) {
		episodes += eps.length;
		items.push({
			name,
			type: "tv",
			status: "FINISHED",
			netflix: { episodes: eps },
		});
	}

	return {
		items,
		rows: data.length,
		movies: items.filter((i) => i.type === "movie").length,
		shows: items.filter((i) => i.type === "tv").length,
		episodes:
			episodes +
			items
				.filter((i) => i.netflix?.group)
				.reduce((n, i) => n + (i.netflix?.episodes?.length ?? 0), 0),
		skipped,
		dateOrderAmbiguous: ambiguous,
	};
}

/**
 * The server found that a group's rows are movies ("Mission: Impossible -
 * Fallout", ...): one movie item per title.
 */
export function splitNetflixGroup(item: ImportedList): ImportedList[] {
	const byTitle = new Map<string, Date[]>();
	for (const r of item.netflixRows ?? []) {
		byTitle.set(r.title, [...(byTitle.get(r.title) ?? []), r.date]);
	}
	return [...byTitle].map(([name, dates]) => ({
		name,
		type: "movie",
		status: "FINISHED",
		netflix: { dates },
	}));
}
