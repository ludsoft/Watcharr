<!--
	What was watched on one day of the activity history.
	Fixed panel on the right, vertically centered in the viewport, so it stays
	visible while scrolling through the heatmap (bottom sheet on phones).
-->
<script lang="ts">
	import { resolve } from "$app/paths";
	import { fly } from "svelte/transition";
	import Error from "@/lib/Error.svelte";
	import Icon from "@/lib/Icon.svelte";
	import Spinner from "@/lib/Spinner.svelte";
	import { baseURL, req } from "@/lib/util/api";
	import type {
		AnalyticsCounts,
		AnalyticsDayDetails,
		AnalyticsDayEntry,
	} from "@/types";
	import { parseDay, plural } from "./util";

	interface Props {
		/** YYYY-MM-DD */
		date: string;
		/** Which media types are shown (same filters as the heatmap). */
		include: Record<keyof AnalyticsCounts, boolean>;
		onClose: () => void;
	}

	let { date, include, onClose }: Props = $props();

	let data: AnalyticsDayDetails | undefined = $state();
	let error: unknown = $state();
	let reqId = 0;

	async function load(d: string) {
		const id = ++reqId;
		data = undefined;
		error = undefined;
		try {
			const resp = await req.get<AnalyticsDayDetails>("/analytics/day", {
				params: {
					date: d,
					tz: Intl.DateTimeFormat().resolvedOptions().timeZone,
				},
			});
			if (id === reqId) data = resp;
		} catch (err) {
			console.error("analytics: Failed to load day", err);
			if (id === reqId) error = err;
		}
	}

	$effect(() => {
		load(date);
	});

	function shown(e: AnalyticsDayEntry) {
		if (e.type === "movie") return include.movies;
		if (e.type === "tv") return include.episodes;
		return include.games;
	}

	let entries = $derived(data?.entries.filter(shown) ?? []);
	let hidden = $derived((data?.entries.length ?? 0) - entries.length);

	/** Posters that failed to load (not cached locally). */
	let brokenPosters: Record<number, boolean> = $state({});

	const typeLabel = { movie: "Film", tv: "TV", game: "Game" };

	function describe(e: AnalyticsDayEntry) {
		if (e.episodes?.length) {
			const eps = e.episodes.map((ep) => `S${ep.season} E${ep.episode}`);
			return `${plural(e.episodes.length, "episode", "episodes")}: ${eps.join(", ")}`;
		}
		const plays = e.plays ?? 1;
		const label = typeLabel[e.type];
		if (plays > 1) {
			return `${label}, ${e.type === "game" ? "finished" : "watched"} ${plays} times`;
		}
		return label;
	}

	let title = $derived(
		parseDay(date).toLocaleDateString(undefined, {
			weekday: "long",
			day: "numeric",
			month: "long",
			year: "numeric",
		}),
	);
</script>

<svelte:window
	onkeydown={(e) => {
		if (e.key === "Escape") onClose();
	}}
/>

<aside
	class="day"
	aria-label="Watched on {title}"
	transition:fly={{ x: 40, duration: 150 }}
>
	<div class="head">
		<h4 class="norm">{title}</h4>
		<button
			class="close plain"
			onclick={onClose}
			aria-label="Close day details"
		>
			<Icon i="close" wh={18} />
		</button>
	</div>

	{#if error}
		<Error
			pretty="Failed to load this day!"
			{error}
			onRetry={() => void load(date)}
		/>
	{:else if !data}
		<Spinner />
	{:else if entries.length === 0}
		<p class="empty">Nothing watched with the selected media types.</p>
	{:else}
		<ul>
			{#each entries as e (e.watchedId)}
				<li>
					<a href={resolve(`/${e.type}/${e.id}`)}>
						{#if e.posterPath && !brokenPosters[e.watchedId]}
							<img
								src="{baseURL}/{e.posterPath}"
								alt=""
								loading="lazy"
								onerror={() => (brokenPosters[e.watchedId] = true)}
							/>
						{:else}
							<div class="no-poster" aria-hidden="true"></div>
						{/if}
						<div class="text">
							<span class="title">{e.title}</span>
							<span class="desc">{describe(e)}</span>
						</div>
					</a>
				</li>
			{/each}
		</ul>
	{/if}
	{#if data && hidden > 0}
		<p class="empty">
			{plural(hidden, "item", "items")} hidden by the media type filters.
		</p>
	{/if}
</aside>

<style lang="scss">
	.day {
		position: fixed;
		z-index: 50;
		/* Vertically centered (top/bottom 0 + auto margins + fit-content height),
		   without a transform, which the fly transition uses. */
		top: 0;
		bottom: 0;
		right: 16px;
		margin: auto 0;
		height: fit-content;
		max-height: calc(100vh - 160px);
		width: 340px;
		max-width: calc(100vw - 32px);
		display: flex;
		flex-flow: column;
		gap: 10px;
		padding: 12px 14px;
		border-radius: 10px;
		border: 1px solid $bg-color-accent;
		background-color: $bg-color;
		box-shadow: 0 4px 24px rgba(0, 0, 0, 0.25);

		@media screen and (max-width: 600px) {
			top: auto;
			left: 0;
			right: 0;
			margin: 0;
			width: auto;
			max-width: none;
			max-height: 60vh;
			border-radius: 10px 10px 0 0;
		}
	}

	.head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 10px;

		h4 {
			font-size: 15px;
		}

		.close {
			display: flex;
			width: auto;
			padding: 4px;
			color: $text-color-accent;
			fill: currentColor;

			&:hover {
				color: $text-color;
			}
		}
	}

	ul {
		display: flex;
		flex-flow: column;
		gap: 4px;
		min-height: 0;
		overflow-y: auto;
		list-style: none;
	}

	a {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 6px;
		border-radius: 6px;
		color: $text-color;
		text-decoration: none;

		&:hover,
		&:focus-visible {
			background-color: $accent-color;
		}
	}

	img,
	.no-poster {
		flex-shrink: 0;
		width: 40px;
		height: 60px;
		object-fit: cover;
		border-radius: 4px;
		background-color: $bg-color-accent;
	}

	.text {
		display: flex;
		flex-flow: column;
		gap: 2px;
		min-width: 0;

		.title {
			font-weight: bold;
			overflow: hidden;
			text-overflow: ellipsis;
			white-space: nowrap;
		}

		.desc {
			font-size: 13px;
			color: $text-color-accent;
		}
	}

	.empty {
		font-size: 13px;
		color: $text-color-accent;
	}
</style>
