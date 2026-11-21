---
sidebar_position: 4
description: Importing your Netflix viewing history, with the dates you watched each movie and episode.
---

# Netflix

:::info Backup?
You may consider a backup of your server before starting any import. They are not easily reversible, though we do our best to ensure they are accurate and bug free!
:::

Watcharr can import the viewing history of a Netflix profile, keeping the date you watched each movie and episode.

## Getting your viewing history

1. On the Netflix website, open **Account**, pick the profile, then **Viewing activity**.
2. At the bottom of the page, click **Download all**. You get a `NetflixViewingHistory.csv` file, with a `Title` and a `Date` column.

## Importing

1. In Watcharr, open the **Import** page and drop the file on **Netflix History**.
2. Review the list, then press **Start Import**.
3. When several titles match an item, you are asked to pick the right one, like with the other importers. Netflix gives no release year, so for titles shared by several movies or shows (like `Rush Hour`, 1998 and 2018), Watcharr picks automatically when it can:
   - Titles released after the first time you watched it are left out.
   - If one is left, it's picked. Otherwise the one with the most TMDB votes is picked, if it has at least 10 times the votes of each other one.
   - Otherwise you are asked.

Netflix gives titles in your profile's language. Matching is done against English titles, so a profile in English gives the best results.

## What is imported

- **Movies** are added as finished. Each row is a play, on the date it was watched.
- **Shows** are added as finished, with each episode watched set as watched on its date. The show's play is dated on the last episode watched.
  - Episodes are matched with the show's episodes by title, so Netflix season names like `Part 2` or `Limited Series` don't matter. Episodes that can't be matched are shown in the import list (and in the browser console), and are skipped.
- Rows like `Name: Something` can be a movie with a subtitle or an episode of a show without seasons. Watcharr checks which one it is with the titles found on TMDB.
- Rows without a title (Netflix sometimes exports ` : Episode 4`) are skipped.

## Items already on your list

Items already on your list are updated instead of skipped:

- They are set as finished.
- An existing play less than a year from an imported date is moved to that date. Otherwise the imported date is added as a new play.
- Episodes already set as watched are left as they are.

Importing the same file again doesn't add anything new.
