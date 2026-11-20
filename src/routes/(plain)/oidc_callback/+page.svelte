<script lang="ts">
	import { goto, replaceState } from "$app/navigation";
	import { page } from "$app/state";
	import { resolve } from "$app/paths";
	import { onMount } from "svelte";
	import { type AuthResponse } from "@/types";
	import { noAuthReq } from "@/lib/util/api";
	import { notify } from "@/lib/util/notify";
	import { ReqerError } from "@/lib/util/fetch";
	import Spinner from "@/lib/Spinner.svelte";

	let error: string | undefined = $state();

	onMount(async () => {
		const params = page.url.searchParams;
		const code = params.get("code");
		const state = params.get("state");
		const providerErr = params.get("error");

		// Remove code/state from the url bar & history, they are single use.
		replaceState(resolve("/oidc_callback"), {});

		if (providerErr) {
			console.error(
				"oidc_callback: Provider returned error:",
				providerErr,
				params.get("error_description"),
			);
			error = params.get("error_description") || providerErr;
			return;
		}
		if (!code || !state) {
			error = "Missing code or state from provider";
			return;
		}
		try {
			const resp = await noAuthReq.post<AuthResponse>("/auth/oidc/callback", {
				code,
				state,
			});
			if (!resp?.token) {
				throw new Error("no token returned");
			}
			console.log("Received token... logging in.");
			localStorage.setItem("token", resp.token);
			goto(resolve("/"));
			notify({ text: "Welcome!", type: "success" });
		} catch (err) {
			console.error("oidc_callback: Login failed!", err);
			error = ReqerError.getMsg(err, "Login failed");
		}
	});
</script>

<div>
	{#if error}
		<span class="error">{error}!</span>
		<a href={resolve("/login")}>Back to login</a>
	{:else}
		<Spinner />
		<p>Logging you in...</p>
	{/if}
</div>

<style lang="scss">
	div {
		display: flex;
		flex-flow: column;
		align-items: center;
		gap: 15px;
		margin: 0 35px;
	}

	.error {
		display: flex;
		justify-content: center;
		width: 100%;
		max-width: 400px;
		padding: 10px;
		background-color: rgb(221, 48, 48);
		text-transform: capitalize;
		color: white;
	}
</style>
