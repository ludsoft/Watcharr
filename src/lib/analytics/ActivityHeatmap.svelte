<!--
	Activity heatmap: one cell per day, one row per year (newest first).
	Columns are weeks (Monday first). Cell color = total watch events that day,
	on 4 levels scaled to the busiest day of the period.
-->
<script lang="ts">
	import type { AnalyticsCounts, AnalyticsDay } from "@/types";
	import { describeCounts, heatmapLayout, parseDay } from "./util";

	interface Props {
		/** First day of the period (YYYY-MM-DD). */
		start: string;
		/** Last day of the period (YYYY-MM-DD). */
		end: string;
		days: AnalyticsDay[];
	}

	let { start, end, days }: Props = $props();

	const CELL = 12;
	const GAP = 3;
	const STEP = CELL + GAP;
	const LEFT = 34;
	const TOP = 18;
	const HEIGHT = TOP + 7 * STEP;
	const MIN_MONTH_LABEL_GAP = 30;
	const weekdays = ["Mon", "", "Wed", "", "Fri", "", "Sun"];

	interface Cell {
		date: string;
		x: number;
		y: number;
		level: number;
		counts: AnalyticsCounts;
	}

	interface YearRow {
		year: number;
		width: number;
		cells: Cell[];
		months: { x: number; label: string }[];
	}

	let hovered: (Cell & { year: number }) | undefined = $state();

	const total = (c: AnalyticsCounts) => c.movies + c.episodes + c.games;

	let byDate = $derived(new Map(days.map((d) => [d.date, d])));
	/**
	 * Level thresholds: quartiles of the active days, so a single huge day
	 * (eg: a bulk import of a whole show on one date) doesn't wash out the
	 * rest of the grid.
	 */
	let thresholds = $derived.by(() => {
		const totals = days
			.map(total)
			.filter((t) => t > 0)
			.sort((a, b) => a - b);
		const q = (p: number) => totals[Math.floor(p * (totals.length - 1))] ?? 0;
		return [q(0.25), q(0.5), q(0.75)];
	});

	function level(t: number) {
		if (t <= 0) return 0;
		return 1 + thresholds.filter((th) => t > th).length;
	}

	let years: YearRow[] = $derived(
		heatmapLayout(start, end).map((y) => {
			const months: YearRow["months"] = [];
			for (const m of y.months) {
				const x = LEFT + m.col * STEP;
				// If the previous label (a partial month) is too close, drop it.
				const prev = months[months.length - 1];
				if (prev && x - prev.x < MIN_MONTH_LABEL_GAP) months.pop();
				months.push({ x, label: m.label });
			}
			return {
				year: y.year,
				width: LEFT + y.weeks * STEP,
				months,
				cells: y.days.map((d) => {
					const counts = byDate.get(d.date) ?? {
						movies: 0,
						episodes: 0,
						games: 0,
					};
					return {
						date: d.date,
						x: LEFT + d.col * STEP,
						y: TOP + d.row * STEP,
						level: level(total(counts)),
						counts,
					};
				}),
			};
		}),
	);

	/** Start scrolled to the most recent end. */
	function scrollToEnd(node: HTMLElement) {
		node.scrollLeft = node.scrollWidth;
	}

	function formatDay(date: string) {
		return parseDay(date).toLocaleDateString(undefined, {
			weekday: "short",
			day: "numeric",
			month: "short",
			year: "numeric",
		});
	}
</script>

<div class="heatmap">
	{#each years as row (row.year)}
		{#if years.length > 1}
			<h4 class="norm">{row.year}</h4>
		{/if}
		<div class="scroll" use:scrollToEnd>
			<div class="inner" style:width="{row.width}px">
				<svg
					width={row.width}
					height={HEIGHT}
					role="img"
					aria-label="Watch activity per day in {row.year}"
				>
					{#each row.months as m (m.x)}
						<text class="axis" x={m.x} y={11}>{m.label}</text>
					{/each}
					{#each weekdays as w, i (i)}
						{#if w}
							<text class="axis" x={0} y={TOP + i * STEP + CELL - 2}>{w}</text>
						{/if}
					{/each}
					{#each row.cells as c (c.date)}
						<rect
							class="cell l{c.level}"
							class:hovered={hovered?.date === c.date}
							x={c.x}
							y={c.y}
							width={CELL}
							height={CELL}
							rx={2}
							role="presentation"
							onpointerenter={() => (hovered = { ...c, year: row.year })}
							onpointerleave={() => (hovered = undefined)}
						/>
					{/each}
				</svg>
				{#if hovered && hovered.year === row.year}
					<div
						class="tip"
						style:left="{hovered.x + CELL / 2}px"
						style:top="{hovered.y - 6}px"
					>
						<strong>{describeCounts(hovered.counts)}</strong>
						<span>{formatDay(hovered.date)}</span>
					</div>
				{/if}
			</div>
		</div>
	{/each}

	<div class="legend" aria-hidden="true">
		<span>Less</span>
		{#each [0, 1, 2, 3, 4] as l (l)}
			<svg width={CELL} height={CELL}>
				<rect class="cell l{l}" width={CELL} height={CELL} rx={2} />
			</svg>
		{/each}
		<span>More</span>
	</div>
</div>

<style lang="scss">
	.heatmap {
		display: flex;
		flex-flow: column;
		gap: 6px;
	}

	h4 {
		margin-top: 6px;
		font-size: 14px;
		color: $text-color-accent;
	}

	.scroll {
		overflow-x: auto;
		/* Room for the tooltip above the top row. */
		padding-top: 44px;
		margin-top: -44px;
	}

	.inner {
		position: relative;
	}

	svg {
		display: block;
		overflow: visible;
	}

	.axis {
		font-size: 11px;
		fill: $text-color-accent;
		user-select: none;
	}

	.cell {
		fill: $accent-color;

		&.l1 {
			fill: var(--viz-heat-1);
		}
		&.l2 {
			fill: var(--viz-heat-2);
		}
		&.l3 {
			fill: var(--viz-heat-3);
		}
		&.l4 {
			fill: var(--viz-heat-4);
		}
		&.hovered {
			stroke: $text-color;
			stroke-width: 1.5px;
		}
	}

	.tip {
		position: absolute;
		z-index: 5;
		display: flex;
		flex-flow: column;
		pointer-events: none;
		transform: translate(-50%, -100%);
		white-space: nowrap;
		padding: 6px 9px;
		font-size: 13px;
		background-color: $bg-color;
		color: $text-color;
		border: 1px solid $bg-color-accent;
		border-radius: 6px;
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);

		span {
			color: $text-color-accent;
			font-size: 12px;
		}
	}

	.legend {
		display: flex;
		align-items: center;
		align-self: flex-end;
		gap: 4px;
		font-size: 12px;
		color: $text-color-accent;

		svg {
			display: inline-block;
		}
	}
</style>
