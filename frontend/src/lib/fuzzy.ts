// The one matcher behind every search, mirroring backend/internal/fuzzy
// rule for rule. Both are checked against docs/fixtures/fuzzy-cases.json,
// so a list, a picker and global Search agree on what matches and in which
// order.
//
// Rules: normalize case, accents, punctuation, spacing (and Arabic letter
// variants); rank exact > prefix > word prefix > substring > typo; allow a
// one-letter typo in short words and two in long ones; words may come in any
// order; a few aliases (PT = private, no show = no-show); digits also match
// phone numbers and references. Every query word must match something.

export const TIER_EXACT = 1000;
export const TIER_PREFIX = 900;

const WORD_EXACT = 100;
const WORD_PREFIX = 85;
const WORD_DIGITS = 70;
const WORD_SUBSTRING = 60;
const WORD_TYPO = 45;
const WORD_TYPO_START = 40;

const JOINS: Record<string, string> = { "no show": "noshow", "t shirt": "tshirt", "drop in": "dropin" };
const ALIASES: Record<string, string[]> = {
  pt: ["private"],
  private: ["pt"],
  sub: ["subscription"],
  subscription: ["sub"],
  tee: ["tshirt"],
  tshirt: ["tee"],
};

const MARK = /\p{Mn}/u;
const WORDCHAR = /[\p{L}\p{N}]/u;

/** Lowercase, no accents or Arabic diacritics, letters and digits only, single spaces. */
export function normalize(s: string): string {
  let out = "";
  let space = true;
  for (let ch of (s ?? "").normalize("NFD")) {
    if (MARK.test(ch) || ch === "ـ") continue;
    if (ch === "ة") ch = "ه";
    else if (ch === "ى") ch = "ي";
    if (WORDCHAR.test(ch)) {
      out += ch.toLowerCase();
      space = false;
    } else if (!space) {
      out += " ";
      space = true;
    }
  }
  return out.replace(/ +$/, "");
}

export function tokens(normalized: string): string[] {
  if (!normalized) return [];
  const raw = normalized.split(" ");
  const out: string[] = [];
  for (let i = 0; i < raw.length; i++) {
    const j = i + 1 < raw.length ? JOINS[`${raw[i]} ${raw[i + 1]}`] : undefined;
    if (j) {
      out.push(j);
      i++;
    } else out.push(raw[i]);
  }
  return out;
}

export const digitsOf = (s: string) => (s ?? "").replace(/[^0-9]/g, "");
const isDigits = (s: string) => /^[0-9]+$/.test(s);

export interface Query {
  norm: string;
  words: string[];
  alts: string[][];
}

export function makeQuery(q: string): Query {
  const words = tokens(normalize(q));
  return { norm: words.join(" "), words, alts: words.map((w) => [w, ...(ALIASES[w] ?? [])]) };
}

export interface Target {
  primary: string;
  first: string;
  words: string[];
  digits: string[];
}

export function makeTarget(primary: string, ...others: (string | undefined | null)[]): Target {
  const pw = tokens(normalize(primary ?? ""));
  const t: Target = { primary: pw.join(" "), first: pw[0] ?? "", words: [...pw], digits: [] };
  const d0 = digitsOf(primary ?? "");
  if (d0) t.digits.push(d0);
  for (const o of others) {
    if (!o) continue;
    t.words.push(...tokens(normalize(o)));
    const d = digitsOf(o);
    if (d) t.digits.push(d);
  }
  return t;
}

export interface MatchResult {
  score: number;
  typo: boolean;
  /** The query with typo'd words corrected: the "Did you mean…" text. */
  corrected: string;
}

const NONE: MatchResult = { score: 0, typo: false, corrected: "" };

export function match(q: Query, t: Target): MatchResult {
  if (!q.words.length || (!t.words.length && !t.digits.length)) return NONE;
  if (t.primary === q.norm) return { score: TIER_EXACT, typo: false, corrected: "" };
  let best: MatchResult = NONE;
  if (t.primary.startsWith(q.norm)) {
    best = { score: TIER_PREFIX + (t.first === q.words[0] ? 50 : 0), typo: false, corrected: "" };
  }
  let sum = 0;
  let lastPos = -1;
  let ordered = true;
  let typo = false;
  const corrected: string[] = [];
  for (let i = 0; i < q.alts.length; i++) {
    let ws = 0;
    let pos = -1;
    let via = "";
    let isTypo = false;
    for (const a of q.alts[i]) {
      for (let j = 0; j < t.words.length; j++) {
        const [s, ty] = wordScore(a, t.words[j]);
        if (s > ws) [ws, pos, via, isTypo] = [s, j, t.words[j], ty];
      }
      if (Array.from(a).length >= 3 && isDigits(a)) {
        for (const d of t.digits) {
          if (d.includes(a) && WORD_DIGITS > ws) [ws, pos, via, isTypo] = [WORD_DIGITS, -1, a, false];
        }
      }
    }
    if (ws === 0) return best;
    sum += ws;
    if (pos >= 0) {
      if (pos <= lastPos) ordered = false;
      lastPos = pos;
    }
    if (isTypo) {
      typo = true;
      corrected.push(via);
    } else corrected.push(q.words[i]);
  }
  let s = 300 + Math.floor((4 * sum) / q.words.length);
  if (ordered && q.words.length > 1) s += 10;
  if (s > best.score) best = { score: s, typo, corrected: typo ? corrected.join(" ") : "" };
  return best;
}

function wordScore(q: string, w: string): [number, boolean] {
  if (q === w) return [WORD_EXACT, false];
  if (w.startsWith(q)) return [WORD_PREFIX, false];
  const qr = Array.from(q);
  const wr = Array.from(w);
  if (qr.length >= 3 && w.includes(q)) return [WORD_SUBSTRING, false];
  if (isDigits(q)) return [0, false]; // numbers match exactly or not at all
  const maxd = maxEdits(qr.length);
  if (maxd === 0 || (qr.length === 3 && qr[0] !== wr[0])) return [0, false];
  let d = osa(qr, wr, maxd);
  if (d <= maxd) return [WORD_TYPO - 5 * (d - 1), true];
  if (qr.length >= 4 && wr.length > qr.length) {
    d = osa(qr, wr.slice(0, qr.length), maxd);
    if (d <= maxd) return [WORD_TYPO_START - 5 * (d - 1), true];
  }
  return [0, false];
}

const maxEdits = (n: number) => (n < 3 ? 0 : n <= 7 ? 1 : 2);

/** Optimal-string-alignment distance, giving up past max. */
function osa(a: string[], b: string[], max: number): number {
  if (Math.abs(a.length - b.length) > max) return max + 1;
  let prev2 = new Array<number>(b.length + 1).fill(0);
  let prev = Array.from({ length: b.length + 1 }, (_, j) => j);
  let cur = new Array<number>(b.length + 1).fill(0);
  for (let i = 1; i <= a.length; i++) {
    cur[0] = i;
    let rowMin = cur[0];
    for (let j = 1; j <= b.length; j++) {
      const cost = a[i - 1] === b[j - 1] ? 0 : 1;
      let v = Math.min(prev[j] + 1, cur[j - 1] + 1, prev[j - 1] + cost);
      if (i > 1 && j > 1 && a[i - 1] === b[j - 2] && a[i - 2] === b[j - 1]) v = Math.min(v, prev2[j - 2] + 1);
      cur[j] = v;
      rowMin = Math.min(rowMin, v);
    }
    if (rowMin > max) return max + 1;
    [prev2, prev, cur] = [prev, cur, prev2];
  }
  return prev[b.length];
}

/** Match order shared with the API: score, then shorter, then code-point order. */
export function less(sa: number, pa: string, sb: number, pb: string): boolean {
  if (sa !== sb) return sa > sb;
  const la = Array.from(pa).length;
  const lb = Array.from(pb).length;
  if (la !== lb) return la < lb;
  return pa < pb;
}

export interface Ranked<T> extends MatchResult {
  item: T;
}

/**
 * Rank a fully loaded list: only matches, best first. `fields` returns the
 * main text first, then any other searchable text (phone, SKU, note…).
 * An empty query returns every item in its original order.
 */
export function rank<T>(items: readonly T[], q: string, fields: (item: T) => (string | undefined | null)[]): Ranked<T>[] {
  const query = makeQuery(q);
  if (!query.words.length) return items.map((item) => ({ item, score: 0, typo: false, corrected: "" }));
  const out: (Ranked<T> & { primary: string; i: number })[] = [];
  items.forEach((item, i) => {
    const [p, ...rest] = fields(item);
    const t = makeTarget(p ?? "", ...rest);
    const r = match(query, t);
    if (r.score > 0) out.push({ ...r, item, primary: t.primary, i });
  });
  out.sort((a, b) => (less(a.score, a.primary, b.score, b.primary) ? -1 : less(b.score, b.primary, a.score, a.primary) ? 1 : a.i - b.i));
  return out.map(({ item, score, typo, corrected }) => ({ item, score, typo, corrected }));
}

/** Convenience: the matching items, best first (all items when q is empty). */
export function fuzzyFilter<T>(items: readonly T[], q: string, fields: (item: T) => (string | undefined | null)[]): T[] {
  return rank(items, q, fields).map((r) => r.item);
}

/** "Did you mean…": the corrected query when every match needed a typo. */
export function didYouMean(ranked: MatchResult[]): string {
  return ranked.length && ranked.every((r) => r.typo) ? ranked[0].corrected : "";
}

/**
 * Pieces of `text` for highlighting what matched `q`: each word of the text
 * that a query word matched (exactly, as a prefix, or with a typo) is marked.
 */
export function highlight(text: string, q: string): { text: string; hit: boolean }[] {
  const query = makeQuery(q);
  if (!text || !query.words.length) return [{ text: text ?? "", hit: false }];
  const parts: { text: string; hit: boolean }[] = [];
  let last = 0;
  for (const m of text.matchAll(/[\p{L}\p{N}\p{Mn}]+/gu)) {
    const word = m[0];
    const nw = normalize(word);
    let hitLen = 0;
    for (const alts of query.alts) {
      for (const a of alts) {
        if (!nw) continue;
        if (nw === a || wordScore(a, nw)[0] > 0) {
          const n = nw.startsWith(a) && nw !== a ? Array.from(a).length : Array.from(word).length;
          hitLen = Math.max(hitLen, n);
        }
      }
    }
    if (!hitLen) continue;
    const start = m.index ?? 0;
    const chars = Array.from(word);
    const hitText = chars.slice(0, hitLen).join("");
    if (start > last) parts.push({ text: text.slice(last, start), hit: false });
    parts.push({ text: hitText, hit: true });
    last = start + hitText.length;
  }
  if (last < text.length) parts.push({ text: text.slice(last), hit: false });
  return parts.length ? parts : [{ text, hit: false }];
}
