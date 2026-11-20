<script lang="ts">
	import { goto } from "$app/navigation";
	import { page } from "$app/state";
	import { resolve } from "$app/paths";
	import Error from "@/lib/Error.svelte";
	import Spinner from "@/lib/Spinner.svelte";
	import Stat from "@/lib/stats/Stat.svelte";
	import Stats from "@/lib/stats/Stats.svelte";
	import ActivityHeatmap from "@/lib/analytics/ActivityHeatmap.svelte";
	import MonthlyBars from "@/lib/analytics/MonthlyBars.svelte";
	import { formatMinutes, formatMonth, plural } from "@/lib/analytics/util";
	import { req } from "@/lib/util/api";
	import type { Analytics, AnalyticsCounts, AnalyticsPeriod } from "@/types";

	const periods: { id: AnalyticsPeriod; label: string }[] = [
		{ id: "current_month", label: "This month" },
		{ id: "current_year", label: "This year" },
		{ id: "last_year", label: "Last year" },
		{ id: "last_12_months", label: "Last 12 months" },
		{ id: "all_time", label: "All time" },
	];
	const defaultPeriod: AnalyticsPeriod = "last_12_months";

	let period: AnalyticsPeriod = $derived.by(() => {
		const p = page.url.searchParams.get("period");
		return periods.find((v) => v.id === p)?.id ?? defaultPeriod;
	});

	type MediaType = keyof AnalyticsCounts;
	const mediaTypes: MediaType[] = ["movies", "episodes", "games"];

	/**
	 * Media types shown in the activity history, from the `types` url param
	 * (eg: `types=movies,games`). Missing/invalid means all of them.
	 */
	let include: Record<MediaType, boolean> = $derived.by(() => {
		const raw = (page.url.searchParams.get("types") ?? "").split(",");
		const picked = mediaTypes.filter((t) => raw.includes(t));
		const all = picked.length === 0;
		return {
			movies: all || picked.includes("movies"),
			episodes: all || picked.includes("episodes"),
			games: all || picked.includes("games"),
		};
	});

	let data: Analytics | undefined = $state();
	let error: unknown = $state();
	let loading = $state(false);
	let reqId = 0;

	async function load(p: AnalyticsPeriod) {
		const id = ++reqId;
		loading = true;
		error = undefined;
		try {
			const resp = await req.get<Analytics>("/analytics", {
				params: {
					period: p,
					tz: Intl.DateTimeFormat().resolvedOptions().timeZone,
				},
			});
			// Ignore responses of periods that are no longer selected.
			if (id === reqId) data = resp;
		} catch (err) {
			console.error("analytics: Failed to load", err);
			if (id === reqId) error = err;
		} finally {
			if (id === reqId) loading = false;
		}
	}

	$effect(() => {
		load(period);
	});

	/**
	 * Update the url with new filters. Replaces the current history entry, so
	 * filter changes don't pile up in history, but going back to this page
	 * restores them.
	 */
	function navigate(p: AnalyticsPeriod, inc: Record<MediaType, boolean>) {
		const types = mediaTypes.filter((t) => inc[t]);
		const typesParam =
			types.length === mediaTypes.length ? "" : `&types=${types.join(",")}`;
		goto(resolve(`/analytics?period=${p}${typesParam}`), {
			replaceState: true,
			keepFocus: true,
			noScroll: true,
		});
	}

	function setPeriod(p: AnalyticsPeriod) {
		if (p === period) return;
		navigate(p, include);
	}

	function toggleType(t: MediaType) {
		// Always keep at least one type selected.
		if (include[t] && mediaTypes.filter((m) => include[m]).length === 1) {
			return;
		}
		navigate(period, { ...include, [t]: !include[t] });
	}

	let monthly = $derived(data?.months ?? []);

	let undatedText = $derived.by(() => {
		const u = data?.undated;
		if (!u) return "";
		const parts: string[] = [];
		if (u.movies) parts.push(plural(u.movies, "film", "films"));
		if (u.shows) parts.push(plural(u.shows, "show", "shows"));
		if (u.games) parts.push(plural(u.games, "game", "games"));
		return parts.join(", ");
	});
</script>

<svelte:head>
	<title>Analytics</title>
</svelte:head>

<div class="content">
	<div class="inner">
		<h2>Analytics</h2>

		<div class="periods" role="group" aria-label="Time period">
			{#each periods as p (p.id)}
				<button
					class="period"
					class:active={p.id === period}
					aria-pressed={p.id === period}
					onclick={() => setPeriod(p.id)}
				>
					{p.label}
				</button>
			{/each}
		</div>

		{#if error && !data}
			<Error
				pretty="Failed to load analytics!"
				{error}
				onRetry={() => void load(period)}
			/>
		{:else if !data}
			<Spinner />
		{:else}
			<div class="body" class:loading>
				{#if error}
					<Error
						pretty="Failed to refresh analytics!"
						{error}
						onRetry={() => void load(period)}
					/>
				{/if}

				<Stats>
					<Stat
						name="Films watched"
						value={data.movies.watched.toLocaleString()}
						large
					/>
					<Stat
						name="Time watching films"
						value={formatMinutes(data.movies.minutes)}
						large
					/>
					<Stat
						name="Shows finished"
						value={data.shows.finished.toLocaleString()}
						large
					/>
					<Stat
						name="Episodes watched"
						value={data.shows.episodes.toLocaleString()}
						large
					/>
					<Stat
						name="Time watching TV"
						value={`≈ ${formatMinutes(data.shows.minutesEstimate)}`}
						disc="Estimated from each show's typical episode length (30m when unknown)."
						large
					/>
					<Stat
						name="Games finished"
						value={data.games.finished.toLocaleString()}
						large
					/>
				</Stats>

				{#if undatedText}
					<p class="note">
						{undatedText} were imported without a watch date, so they aren't included
						here.
					</p>
				{/if}

				<section>
					<h3 class="norm">Activity History</h3>
					<p class="sub">
						Each cell is one day: films, episodes and games finished. Stronger
						color means more activity.
					</p>
					<ActivityHeatmap
						start={data.start}
						end={data.end}
						days={data.days}
						{include}
						onToggle={toggleType}
					/>
				</section>

				<section>
					<h3 class="norm">Per Month</h3>
					<div class="charts">
						<MonthlyBars
							title="Films watched"
							months={monthly.map((m) => ({ month: m.month, value: m.movies }))}
							unit={["film", "films"]}
							color="var(--viz-series-1)"
						/>
						<MonthlyBars
							title="Episodes watched"
							months={monthly.map((m) => ({
								month: m.month,
								value: m.episodes,
							}))}
							unit={["episode", "episodes"]}
							color="var(--viz-series-2)"
						/>
						<MonthlyBars
							title="Games finished"
							months={monthly.map((m) => ({ month: m.month, value: m.games }))}
							unit={["game", "games"]}
							color="var(--viz-series-3)"
						/>
					</div>
					<details>
						<summary>Show as table</summary>
						<table>
							<thead>
								<tr>
									<th scope="col">Month</th>
									<th scope="col">Films</th>
									<th scope="col">Episodes</th>
									<th scope="col">Games</th>
								</tr>
							</thead>
							<tbody>
								{#each [...monthly].reverse() as m (m.month)}
									<tr>
										<th scope="row">{formatMonth(m.month)}</th>
										<td>{m.movies}</td>
										<td>{m.episodes}</td>
										<td>{m.games}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</details>
				</section>
			</div>
		{/if}
	</div>
</div>

<style lang="scss">
	.content {
		display: flex;
		justify-content: center;
		padding: 0 16px 40px 16px;

		.inner {
			display: flex;
			flex-flow: column;
			gap: 18px;
			width: 100%;
			max-width: 1100px;
			min-width: 0;
		}
	}

	.periods {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;

		.period {
			width: auto;
			padding: 6px 14px;
			font-size: 14px;
			border-radius: 999px;

			&:not(.active) {
				background-color: transparent;
				color: $text-color;
				border: 1px solid $bg-color-accent;

				&:hover {
					background-color: $accent-color;
				}
			}
		}
	}

	.body {
		display: flex;
		flex-flow: column;
		gap: 28px;
		transition: opacity 150ms ease;

		/* Keep the previous render while refetching. */
		&.loading {
			opacity: 0.5;
		}

		:global(> div:first-of-type) {
			margin-top: 0;
		}
	}

	.note {
		margin-top: -14px;
		font-size: 13px;
		color: $text-color-accent;
	}

	section {
		display: flex;
		flex-flow: column;
		gap: 10px;
		padding: 18px 20px;
		border-radius: 10px;
		/* Charts sit on the page bg (their colors are validated against it). */
		border: 1px solid $bg-color-accent;
		min-width: 0;

		h3 {
			font-size: 18px;
		}

		.sub {
			margin-top: -6px;
			font-size: 13px;
			color: $text-color-accent;
		}
	}

	.charts {
		display: flex;
		flex-flow: column;
		gap: 26px;
	}

	details {
		font-size: 14px;

		summary {
			cursor: pointer;
			color: $text-color-accent;
		}

		table {
			margin-top: 10px;
			border-collapse: collapse;
			font-variant-numeric: tabular-nums;

			th,
			td {
				padding: 4px 14px 4px 0;
				text-align: right;
			}

			th[scope="row"],
			thead th:first-child {
				text-align: left;
			}
		}
	}
</style>
