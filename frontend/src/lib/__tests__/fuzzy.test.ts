import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { didYouMean, fuzzyFilter, highlight, normalize, rank } from "../fuzzy";

interface Case {
  name: string;
  q: string;
  targets: string[][];
  expect: string[];
  didYouMean?: string;
}

// The same cases the Go matcher is tested against: both sides must agree.
const fixture = JSON.parse(
  readFileSync(resolve(__dirname, "../../../../docs/fixtures/fuzzy-cases.json"), "utf8"),
) as { cases: Case[] };

describe("shared fuzzy cases", () => {
  for (const c of fixture.cases) {
    it(c.name, () => {
      const ranked = rank(c.targets, c.q, (t) => t);
      expect(ranked.map((r) => r.item[0])).toEqual(c.expect);
      expect(didYouMean(ranked)).toBe(c.didYouMean ?? "");
    });
  }
});

describe("normalize", () => {
  it("folds case, accents, punctuation and Arabic variants", () => {
    expect(normalize("  José  O'Neil–Smith ")).toBe("jose o neil smith");
    expect(normalize("أحمد")).toBe("احمد");
    expect(normalize("فاطمة")).toBe("فاطمه");
    expect(normalize("مـحـمـد")).toBe("محمد");
  });
});

describe("lists", () => {
  it("an empty query keeps the list and its order", () => {
    const names = ["Rami", "Jad", "Lara"];
    expect(fuzzyFilter(names, "  ", (n) => [n])).toEqual(names);
  });

  it("finds a match that sits far down a long list", () => {
    const rows = Array.from({ length: 1500 }, (_, i) => ({ id: i, name: `Member ${i}` }));
    rows.push({ id: 9999, name: "Jad Saliba" });
    const hit = fuzzyFilter(rows, "jda salba", (r) => [r.name]);
    expect(hit.map((r) => r.id)).toEqual([9999]);
  });
});

describe("highlight", () => {
  it("marks the matched prefix and whole typo'd words", () => {
    expect(highlight("Jad Saliba", "sal")).toEqual([
      { text: "Jad ", hit: false },
      { text: "Sal", hit: true },
      { text: "iba", hit: false },
    ]);
    expect(highlight("Boxing Gloves", "gloevs").filter((p) => p.hit).map((p) => p.text)).toEqual(["Gloves"]);
  });
  it("keeps accents in the displayed text", () => {
    expect(highlight("José Haddad", "jose").filter((p) => p.hit).map((p) => p.text)).toEqual(["José"]);
  });
});
