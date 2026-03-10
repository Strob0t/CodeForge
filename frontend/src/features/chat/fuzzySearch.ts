/**
 * Fuzzy search for chat slash-command and mention autocomplete.
 *
 * Matching strategy (tiered):
 *   1. Prefix match   — query is a prefix of the label
 *   2. Substring match — query appears anywhere in the label
 *   3. Levenshtein     — edit distance <= 2
 *
 * Within each tier items are sorted by frequency (descending),
 * then alphabetically by label.
 */

export interface Item {
  id: string;
  label: string;
  category?: string;
}

/** Compute the Levenshtein (edit) distance between two strings. */
function levenshtein(a: string, b: string): number {
  const aLen = a.length;
  const bLen = b.length;

  // Fast-path: one string is empty.
  if (aLen === 0) return bLen;
  if (bLen === 0) return aLen;

  // Use a single-row DP approach to keep memory O(min(aLen, bLen)).
  // Ensure `b` is the shorter string so the row is as small as possible.
  let short: string;
  let long: string;
  if (aLen < bLen) {
    short = a;
    long = b;
  } else {
    short = b;
    long = a;
  }

  const shortLen = short.length;
  const longLen = long.length;

  let prev = new Array<number>(shortLen + 1);
  let curr = new Array<number>(shortLen + 1);

  for (let j = 0; j <= shortLen; j++) {
    prev[j] = j;
  }

  for (let i = 1; i <= longLen; i++) {
    curr[0] = i;
    for (let j = 1; j <= shortLen; j++) {
      const cost = long[i - 1] === short[j - 1] ? 0 : 1;
      curr[j] = Math.min(
        prev[j] + 1, // deletion
        curr[j - 1] + 1, // insertion
        prev[j - 1] + cost, // substitution
      );
    }
    // Swap rows.
    [prev, curr] = [curr, prev];
  }

  return prev[shortLen];
}

const enum MatchTier {
  Prefix = 0,
  Substring = 1,
  Fuzzy = 2,
}

interface ScoredItem {
  item: Item;
  tier: MatchTier;
  frequency: number;
}

/**
 * Return items that match `query`, ranked by match quality and usage frequency.
 *
 * - Empty query returns all items sorted by frequency then alphabetically.
 * - Matching is case-insensitive.
 */
export function fuzzyMatch(
  query: string,
  items: readonly Item[],
  frequencyMap: Map<string, number>,
): Item[] {
  const q = query.toLowerCase().trim();

  // Empty query: return everything, sorted by frequency desc then alpha.
  if (q.length === 0) {
    return [...items].sort((a, b) => {
      const freqA = frequencyMap.get(a.id) ?? 0;
      const freqB = frequencyMap.get(b.id) ?? 0;
      if (freqB !== freqA) return freqB - freqA;
      return a.label.localeCompare(b.label);
    });
  }

  const scored: ScoredItem[] = [];

  for (const item of items) {
    const label = item.label.toLowerCase();
    let tier: MatchTier | undefined;

    if (label.startsWith(q)) {
      tier = MatchTier.Prefix;
    } else if (label.includes(q)) {
      tier = MatchTier.Substring;
    } else if (levenshtein(q, label) <= 2) {
      tier = MatchTier.Fuzzy;
    }

    if (tier !== undefined) {
      scored.push({
        item,
        tier,
        frequency: frequencyMap.get(item.id) ?? 0,
      });
    }
  }

  scored.sort((a, b) => {
    // 1. Match tier (prefix < substring < fuzzy — lower is better).
    if (a.tier !== b.tier) return a.tier - b.tier;
    // 2. Frequency descending within the same tier.
    if (b.frequency !== a.frequency) return b.frequency - a.frequency;
    // 3. Alphabetical tiebreaker.
    return a.item.label.localeCompare(b.item.label);
  });

  return scored.map((s) => s.item);
}
