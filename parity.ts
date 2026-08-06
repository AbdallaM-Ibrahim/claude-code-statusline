#!/usr/bin/env bun
/**
 * Parity harness: diffs the Go binary against the bun/TypeScript status line it
 * replaces, across the payload shapes that actually vary.
 *
 *   bun ~/.claude/statusline-go/parity.ts
 *
 * Line 1 must match byte for byte — the native git reader is meant to be a
 * faithful replacement.
 *
 * Line 2 is compared segment by segment, and some differences are EXPECTED and
 * listed below rather than treated as failures. Run this before repointing
 * settings.json, and again after any change to the cost module.
 */

import { execFileSync } from "node:child_process";
import { homedir } from "node:os";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const HOME = homedir();
const GO = join(HOME, ".claude", "statusline.exe");
const JS = join(HOME, ".claude", "statusline.js");
const TRANSCRIPT = join(
  HOME, ".claude", "projects",
  "C--Users-abdo-workspace-software-engineer-website",
  "741a0639-b2eb-42f0-8f7e-5827c5932a1f.jsonl",
);
const REPO = "C:/Users/abdo/workspace/software-engineer-website";

/**
 * Differences we have decided are correct, with the reason.
 *
 * Caveat on the oracle: ccusage caches its own status line output for 1s by
 * default, so across a fast run every case tends to report the same money
 * figures regardless of the payload. Treat the JS money columns as indicative,
 * not authoritative — the real cost validation is the token-level cross-check
 * recorded in the commit history, where our deduped totals matched ccusage's own
 * per-day token attribution exactly.
 */
const EXPECTED_DIFFS = [
  {
    match: /💰|🔥/,
    why: "ccusage's offline pricing table has no claude-opus-5 entry, so it prices today's traffic at $0. Verified: its own daily row for today carries 44.9M tokens at totalCost 0. Ours prices from the LiteLLM table.",
  },
  {
    match: /🔥/,
    why: "burn-rate emoji uses our own documented threshold; ccusage's marker did not track the rate monotonically in observed output.",
  },
];

const strip = (s: string) => s.replace(/\x1b\[[0-9;]*m/g, "");

function run(cmd: string[], payload: string): string {
  try {
    return execFileSync(cmd[0], cmd.slice(1), {
      input: payload,
      encoding: "utf8",
      timeout: 30000,
    }).replace(/\n$/, "");
  } catch (e: any) {
    return `<<ERROR: ${e?.message ?? e}>>`;
  }
}

const scratch = mkdtempSync(join(tmpdir(), "parity-"));
const emptyDir = join(scratch, "not-a-repo");
mkdirSync(emptyDir, { recursive: true });

// A repository with a commit but no upstream configured.
const noUpstream = join(scratch, "no-upstream");
mkdirSync(noUpstream, { recursive: true });
try {
  const git = (...a: string[]) => execFileSync("git", ["-C", noUpstream, ...a], { encoding: "utf8" });
  git("init", "-b", "main");
  git("config", "user.email", "t@example.com");
  git("config", "user.name", "t");
  writeFileSync(join(noUpstream, "f.txt"), "hi");
  git("add", "-A");
  git("commit", "-m", "solo commit");
} catch (e) {
  console.log("(could not build the no-upstream fixture:", e, ")");
}

const base = {
  session_id: "741a0639-b2eb-42f0-8f7e-5827c5932a1f",
  transcript_path: TRANSCRIPT,
  model: { display_name: "Opus 5" },
  context_window: { context_window_size: 200000, total_input_tokens: 68000, used_percentage: 34 },
};

const cases: Array<{ name: string; payload: any }> = [
  { name: "repo + full payload", payload: { ...base, cwd: REPO, cost: { total_cost_usd: 0.42 }, effort: { level: "xhigh" }, thinking: { enabled: true } } },
  { name: "repo, no cost field", payload: { ...base, cwd: REPO } },
  { name: "repo, fast mode", payload: { ...base, cwd: REPO, fast_mode: true, effort: { level: "low" } } },
  { name: "not a repository", payload: { ...base, cwd: emptyDir } },
  { name: "repo with no upstream", payload: { ...base, cwd: noUpstream } },
  { name: "workspace overrides cwd", payload: { ...base, cwd: emptyDir, workspace: { current_dir: REPO } } },
  { name: "worktree / PR / agent badges", payload: { ...base, cwd: REPO, worktree: { name: "wt" }, pr: { number: 12 }, agent: { name: "explore" } } },
  { name: "context from token breakdown", payload: { ...base, cwd: REPO, context_window: { context_window_size: 200000, current_usage: { input_tokens: 1000, cache_creation_input_tokens: 2000, cache_read_input_tokens: 47000 } } } },
  { name: "rate limits in payload", payload: { ...base, cwd: REPO, rate_limits: { five_hour: { used_percentage: 42, resets_at: Math.floor(Date.now() / 1000) + 3600 }, seven_day: { used_percentage: 18, resets_at: Math.floor(Date.now() / 1000) + 86400 } } } },
];

let line1Fail = 0;
let line2Diff = 0;

console.log("=".repeat(78));
console.log("PARITY: Go binary vs bun/TypeScript status line");
console.log("=".repeat(78));

for (const c of cases) {
  const payload = JSON.stringify(c.payload);
  const go = run([GO], payload).split("\n");
  const js = run(["bun", JS], payload).split("\n");

  const l1Same = go[0] === js[0];
  if (!l1Same) line1Fail++;

  console.log(`\n### ${c.name}`);
  console.log(`  line 1: ${l1Same ? "IDENTICAL" : "*** MISMATCH ***"}`);
  if (!l1Same) {
    console.log(`    go: ${JSON.stringify(strip(go[0] ?? ""))}`);
    console.log(`    js: ${JSON.stringify(strip(js[0] ?? ""))}`);
  }

  // Align segments by kind, not by index. When one side omits a segment
  // entirely — ccusage errors out on a payload with no total_input_tokens and
  // emits no cost at all — an index-wise compare reports every later segment as
  // different, which buries the real difference.
  const keyOf = (s: string) => {
    for (const k of ["🤖", "🧠", "💰", "🔥", "⏳", "[CAVEMAN"]) {
      if (s.startsWith(k)) return k;
    }
    return s.slice(0, 4);
  };
  const index = (segs: string[]) => {
    const m = new Map<string, string>();
    for (const s of segs) m.set(keyOf(s), s);
    return m;
  };
  const goMap = index(strip(go[1] ?? "").split(" | "));
  const jsMap = index(strip(js[1] ?? "").split(" | "));

  const diffs: string[] = [];
  for (const k of new Set([...goMap.keys(), ...jsMap.keys()])) {
    const g = goMap.get(k);
    const j = jsMap.get(k);
    if (g !== j) diffs.push(`      ${k}  go=${JSON.stringify(g)}  js=${JSON.stringify(j)}`);
  }
  if (diffs.length === 0) {
    console.log(`  line 2: IDENTICAL`);
  } else {
    line2Diff++;
    const expected = diffs.every((d) => EXPECTED_DIFFS.some((e) => e.match.test(d)));
    console.log(`  line 2: ${expected ? "differs (all EXPECTED)" : "*** UNEXPECTED DIFFERENCE ***"}`);
    for (const d of diffs) console.log(d);
  }
}

// Degradation cases the JS cannot be compared against directly.
console.log("\n### degradation (Go only)");
for (const [label, input] of [
  ["malformed stdin", "not json"],
  ["empty stdin", ""],
  ["JSON array, not object", "[1,2,3]"],
  ["BOM-prefixed payload", "\uFEFF" + JSON.stringify({ ...base, cwd: REPO })],
] as const) {
  const out = run([GO], input);
  const lines = out.split("\n").length;
  console.log(`  ${label.padEnd(24)} -> ${lines} line(s): ${JSON.stringify(strip(out).slice(0, 70))}`);
}

rmSync(scratch, { recursive: true, force: true });

console.log("\n" + "=".repeat(78));
console.log(`line 1 mismatches : ${line1Fail}  (must be 0)`);
console.log(`line 2 cases differing: ${line2Diff}`);
for (const e of EXPECTED_DIFFS) console.log(`\nEXPECTED: ${e.why}`);
process.exit(line1Fail === 0 ? 0 : 1);
