#!/usr/bin/env node
// scripts/friends-linux-tokens.mjs <links-file> [--only <name>] [--rotate] [--dry-run]
//
// Gives each friend on the friends' page a Linux token of their own, which the page
// puts into that friend's install line — and only that friend's page can read it.
//
// A terminal has no browser cookie, so the line unlocks the page itself (POST /unlock)
// and needs a viewer token. The page never sees the token in the friend's link (the
// platform spends it before the page runs), and every friend reads the same /d/
// store, so per friend this mints a separate viewer token, labelled
// friend-<name>-linux, and stores it on the page ENCRYPTED with that friend's voice key:
//   record  linux.<id>   schema linux-token@1, data {iv, ct}  (base64url)
//   id      first 32 hex of SHA-256("mega-speak linux id\n" + key)
//   cipher  AES-256-GCM, key SHA-256("mega-speak linux key\n" + key), plaintext the token
// The page derives the same id and key from the k in the friend's link (index.html,
// linuxToken()). A friend reading another friend's record learns nothing without
// that friend's voice key. A separate token can be revoked (page-token revoke) without
// touching the friend's link.
//
// The links file is the 0600 owner file: one "== <name> (<lang>)" header per friend,
// the friend's https link with #t=…&k=… under it. Friends whose link has no k are
// skipped. An existing record is kept unless --rotate, which revokes the friend's
// live friend-<name>-linux tokens, mints a new one and rewrites the record.
// Runs on the page owner's host (page-token, page-data). Never prints a token or key.
import { readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { webcrypto as crypto } from "node:crypto";

const SLUG = "mega-friends-a020102a11bf95c8";
const BIN = process.env.PAGE_BIN || `${process.env.HOME}/.local/share/ucc/shared/skills/ccc-pages/bin`;
const args = process.argv.slice(2);
const file = args.find((a) => !a.startsWith("--") && args[args.indexOf(a) - 1] !== "--only");
const only = args.includes("--only") ? args[args.indexOf("--only") + 1] : "";
const rotate = args.includes("--rotate"), dry = args.includes("--dry-run");
if (!file) { console.error("usage: friends-linux-tokens.mjs <links-file> [--only <name>] [--rotate] [--dry-run]"); process.exit(2); }

const enc = new TextEncoder();
const b64u = (b) => Buffer.from(b).toString("base64url");
const sha = async (s) => new Uint8Array(await crypto.subtle.digest("SHA-256", enc.encode(s)));
export async function recordId(key) { return Buffer.from(await sha("mega-speak linux id\n" + key)).toString("hex").slice(0, 32); }
async function seal(key, token) {
  const k = await crypto.subtle.importKey("raw", await sha("mega-speak linux key\n" + key), "AES-GCM", false, ["encrypt"]);
  const iv = crypto.getRandomValues(new Uint8Array(12));
  return { iv: b64u(iv), ct: b64u(await crypto.subtle.encrypt({ name: "AES-GCM", iv }, k, enc.encode(token))) };
}
const run = (cmd, a, opts = {}) => execFileSync(`${BIN}/${cmd}`, a, { encoding: "utf8", stdio: ["pipe", "pipe", "pipe"], ...opts });

const friends = [];
let name = "";
for (const line of readFileSync(file, "utf8").split("\n")) {
  const h = line.match(/^== (.+?) \(/);
  if (h) { name = h[1]; continue; }
  if (name && line.startsWith("https://")) {
    const frag = new URLSearchParams(new URL(line.trim()).hash.slice(1));
    if (frag.get("k")) friends.push({ name, key: frag.get("k") });
    name = "";
  }
}
const tokens = JSON.parse(run("page-token", ["list", "--slug", SLUG, "--json"]));
const rows = tokens.tokens;

for (const f of friends.filter((f) => !only || f.name === only)) {
  const label = `friend-${f.name}-linux`, rid = `linux.${await recordId(f.key)}`;
  const live = rows.filter((t) => t.label === label && t.status === "live");
  let have = false;
  try { run("page-data", ["get", SLUG, rid]); have = true; } catch {}
  if (have && live.length && !rotate) { console.log(`${f.name}: kept (${rid}, token ${live.map((t) => t.id).join(",")})`); continue; }
  if (dry) { console.log(`${f.name}: would ${live.length ? "revoke " + live.length + " and " : ""}mint ${label}, write ${rid}`); continue; }
  for (const t of live) run("page-token", ["revoke", t.id]);
  const r = execFileSync(`${BIN}/page-token`, ["mint", SLUG, "--role", "viewer", "--label", label, "--user", f.name], { encoding: "utf8", stdio: ["pipe", "pipe", "pipe"] });
  const secret = new URL(r.trim()).hash.replace(/^#t=/, "");
  if (!secret) throw new Error(`${f.name}: mint printed no link`);
  run("page-data", ["put", SLUG, rid, "--schema", "linux-token@1", "--force", "--notify", "none", "--data", JSON.stringify(await seal(f.key, secret))]);
  console.log(`${f.name}: minted ${label}, wrote ${rid}`);
}
