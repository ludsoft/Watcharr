<!--
	Single series column chart, one column per month.
	Long ranges scroll horizontally (starting at the most recent month).
-->
<script lang="ts">
	import { formatMonth, monthAbbr, plural } from "./util";

	interface Props {
		/** Chart title, also names the series. */
		title: string;
		months: { month: string; value: number }[];
		/** Unit used in tooltips/labels, eg ["film", "films"]. */
		unit: [string, string];
		/** CSS color of the bars (a --viz-series-* var). */
		color: string;
	}

	let { title, months, unit, color }: Props = $props();

	const LEFT = 40;
	const TOP = 18;
	const PLOT_H = 140;
	const BOTTOM = 34;
	const HEIGHT = TOP + PLOT_H + BOTTOM;
	const MIN_SLOT = 28;
	const MAX_BAR = 24;

	let clientWidth = $state(600);
	let hovered: number | undefined = $state();

	let n = $derived(months.length);
	let max = $derived(months.reduce((m, v) => Math.max(m, v.value), 0));
	let maxIdx = $derived(months.findIndex((m) => m.value === max));
	// clientWidth is of the scroll area, which excludes the pinned y axis.
	let plotW = $derived(Math.max(clientWidth, n * MIN_SLOT));
	let slot = $derived(plotW / Math.max(n, 1));
	let barW = $derived(Math.min(MAX_BAR, slot * 0.65));

	/** Clean integer ticks from 0 to a nice max. */
	let ticks = $derived.by(() => {
		if (max <= 4)
			return Array.from({ length: Math.max(max, 1) + 1 }, (_, i) => i);
		const raw = max / 4;
		const mag = 10 ** Math.floor(Math.log10(raw));
		const step = [1, 2, 5, 10].map((s) => s * mag).find((s) => s >= raw) ?? raw;
		const top = Math.ceil(max / step) * step;
		const t: number[] = [];
		for (let v = 0; v <= top; v += step) t.push(v);
		return t;
	});
	let yMax = $derived(ticks[ticks.length - 1] || 1);

	const y = (v: number) => TOP + PLOT_H - (v / yMax) * PLOT_H;

	/** Column path: 4px rounded top, square at the baseline. */
	function barPath(x: number, v: number) {
		const top = y(v);
		const h = TOP + PLOT_H - top;
		const r = Math.min(4, h, barW / 2);
		return `M${x},${top + h} V${top + r} Q${x},${top} ${x + r},${top} H${x + barW - r} Q${x + barW},${top} ${x + barW},${top + r} V${top + h} Z`;
	}

	/** Which months get an x label, depends on how many there are. */
	function showMonthLabel(i: number) {
		if (n <= 12) return true;
		const mo = Number(months[i].month.split("-")[1]);
		if (n <= 24) return mo % 3 === 1;
		return mo === 1;
	}

	function showYearLabel(i: number) {
		return i === 0 || months[i].month.endsWith("-01");
	}

	function scrollToEnd(node: HTMLElement) {
		node.scrollLeft = node.scrollWidth;
	}
</script>

<figure>
	<figcaption>{title}</figcaption>
	{#if max === 0}
		<p class="empty">Nothing in this period.</p>
	{:else}
		<div class="chart">
			<!-- Y axis is outside the scroll area, so it stays visible when scrolling. -->
			<svg class="y-axis" width={LEFT} height={HEIGHT} aria-hidden="true">
				{#each ticks as t (t)}
					<text class="axis tick" x={LEFT - 6} y={y(t) + 4}
						>{t.toLocaleString()}</text
					>
				{/each}
			</svg>
			<div class="scroll" bind:clientWidth use:scrollToEnd>
				<div class="inner" style:width="{plotW}px">
					<svg
						width={plotW}
						height={HEIGHT}
						role="img"
						aria-label="{title} per month, up to {plural(
							max,
							unit[0],
							unit[1],
						)}"
					>
						{#each ticks as t (t)}
							<line class="grid" x1={0} x2={plotW} y1={y(t)} y2={y(t)} />
						{/each}
						{#each months as m, i (m.month)}
							{@const x = i * slot + (slot - barW) / 2}
							{#if m.value > 0}
								<path
									d={barPath(x, m.value)}
									fill={color}
									class:dim={hovered !== undefined && hovered !== i}
								/>
							{/if}
							{#if i === maxIdx}
								<text class="value" x={x + barW / 2} y={y(m.value) - 5}>
									{m.value.toLocaleString()}
								</text>
							{/if}
							{#if showMonthLabel(i)}
								<text
									class="axis"
									x={x + barW / 2}
									y={TOP + PLOT_H + 15}
									text-anchor="middle"
								>
									{monthAbbr[Number(m.month.split("-")[1]) - 1]}
								</text>
							{/if}
							{#if showYearLabel(i)}
								<text
									class="axis year"
									x={x + barW / 2}
									y={TOP + PLOT_H + 29}
									text-anchor="middle"
								>
									{m.month.split("-")[0]}
								</text>
							{/if}
							<!-- Hit target: the whole column slot, bigger than the bar. -->
							<rect
								class="hit"
								x={i * slot}
								y={TOP}
								width={slot}
								height={PLOT_H}
								role="presentation"
								onpointerenter={() => (hovered = i)}
								onpointerleave={() => (hovered = undefined)}
							/>
						{/each}
					</svg>
					{#if hovered !== undefined}
						{@const m = months[hovered]}
						<div
							class="tip"
							style:left="{hovered * slot + slot / 2}px"
							style:top="{Math.min(y(m.value), TOP + PLOT_H - 10) - 8}px"
						>
							<strong>{plural(m.value, unit[0], unit[1])}</strong>
							<span>{formatMonth(m.month)}</span>
						</div>
					{/if}
				</div>
			</div>
		</div>
	{/if}
</figure>

<style lang="scss">
	figure {
		margin: 0;
		display: flex;
		flex-flow: column;
		gap: 8px;
	}

	figcaption {
		font-weight: bold;
		font-size: 15px;
	}

	.empty {
		color: $text-color-accent;
		font-size: 14px;
		padding: 20px 0;
	}

	.chart {
		display: flex;
		align-items: flex-start;
	}

	.y-axis {
		flex-shrink: 0;
	}

	.scroll {
		flex: 1;
		min-width: 0;
		overflow-x: auto;
		/* Room for the tooltip above the tallest bar. */
		padding-top: 40px;
		margin-top: -40px;
	}

	.inner {
		position: relative;
	}

	svg {
		display: block;
	}

	.grid {
		stroke: var(--viz-grid);
		stroke-width: 1px;
		shape-rendering: crispEdges;
	}

	.axis {
		font-size: 11px;
		fill: $text-color-accent;
		font-variant-numeric: tabular-nums;
		user-select: none;

		&.tick {
			text-anchor: end;
		}

		&.year {
			font-weight: bold;
		}
	}

	.value {
		font-size: 12px;
		font-weight: bold;
		fill: $text-color;
		text-anchor: middle;
	}

	path {
		transition: opacity 120ms ease;

		&.dim {
			opacity: 0.45;
		}
	}

	.hit {
		fill: transparent;
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
</style>
