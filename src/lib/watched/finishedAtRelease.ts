// "Finished at release date": mark content (or a season/episode) as
// finished with the watch dated at its release date instead of today.

import type { SupportedMedia, Watched, WatchedStatus } from "@/types";
import { req } from "../util/api";
import { notify } from "../util/notify";
import { ReqerError } from "../util/fetch";

/** Local `YYYY-MM-DD` of today. */
function today(): string {
	const d = new Date();
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

/**
 * Why "Finished at release date" can't be used, or undefined if it can.
 * @param releaseDate Release/air date (`YYYY-MM-DD` or ISO date time).
 */
export function finishedAtReleaseDisabledReason(
	status: WatchedStatus | undefined,
	releaseDate: string | undefined,
): string | undefined {
	if (status === "FINISHED") return "Already finished";
	const d = releaseDate?.slice(0, 10);
	if (!d || !/^\d{4}-\d{2}-\d{2}$/.test(d) || d.startsWith("0001")) {
		return "Release date unknown";
	}
	if (d > today()) return "Not released yet";
	return undefined;
}

async function run<T>(fn: () => Promise<T>): Promise<T> {
	const nid = notify({ text: "Saving", type: "loading" });
	try {
		const r = await fn();
		notify({ id: nid, text: "Finished at release date!", type: "success" });
		return r;
	} catch (err) {
		console.error("finishedAtRelease: Failed!", err);
		notify({
			id: nid,
			text: ReqerError.getMsg(err, "Failed to finish at release date"),
			type: "error",
		});
		throw err;
	}
}

/**
 * Finish a movie, show or game at its release date.
 * Shows get each released episode finished at its own air date.
 * @returns The full, updated watched entry.
 */
export function finishContentAtRelease(
	contentType: SupportedMedia,
	contentId: number,
): Promise<Watched> {
	return run(() =>
		req.post<Watched>("/watched/finished_at_release", {
			contentType,
			...(contentType === "game"
				? { igdbId: contentId }
				: { tmdbId: contentId }),
		}),
	);
}

/**
 * Copy the server's updated entry into the watched item we hold, so every
 * component using it (seasons, episodes, activity...) updates.
 */
function applyWatched(target: Watched, src: Watched) {
	target.status = src.status;
	target.activity = src.activity;
	target.watchedSeasons = src.watchedSeasons;
	target.watchedEpisodes = src.watchedEpisodes;
	target.plays = src.plays;
	target.updatedAt = src.updatedAt;
}

/** Finish a season (its released episodes) at their air dates. */
export async function finishSeasonAtRelease(
	watchedItem: Watched,
	seasonNumber: number,
) {
	const w = await run(() =>
		req.post<Watched>("/watched/season/finished_at_release", {
			watchedId: watchedItem.id,
			seasonNumber,
		}),
	);
	applyWatched(watchedItem, w);
}

/** Finish an episode at its air date. */
export async function finishEpisodeAtRelease(
	watchedItem: Watched,
	seasonNumber: number,
	episodeNumber: number,
) {
	const w = await run(() =>
		req.post<Watched>("/watched/episode/finished_at_release", {
			watchedId: watchedItem.id,
			seasonNumber,
			episodeNumber,
		}),
	);
	applyWatched(watchedItem, w);
}
