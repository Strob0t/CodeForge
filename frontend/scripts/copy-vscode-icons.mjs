#!/usr/bin/env node
// Downloads VS Code file icons from the vscode-icons GitHub repository.
// Run via: node scripts/copy-vscode-icons.mjs
// Called automatically by npm postinstall.

import { createRequire } from "node:module";
import { existsSync, mkdirSync, writeFileSync, readdirSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const OUT_DIR = join(__dirname, "..", "public", "icons", "vscode");

// Collect all unique SVG filenames from vscode-icons-js mappings
const require = createRequire(import.meta.url);
const fe1 = require("vscode-icons-js/dist/generated/FileExtensions1ToIcon");
const fe2 = require("vscode-icons-js/dist/generated/FileExtensions2ToIcon");
const fn = require("vscode-icons-js/dist/generated/FileNamesToIcon");
const fol = require("vscode-icons-js/dist/generated/FolderNamesToIcon");
const lang = require("vscode-icons-js/dist/generated/LanguagesToIcon");

const allIcons = new Set([
  ...Object.values(fe1.FileExtensions1ToIcon),
  ...Object.values(fe2.FileExtensions2ToIcon),
  ...Object.values(fn.FileNamesToIcon),
  ...Object.values(fol.FolderNamesToIcon),
  ...Object.values(lang.LanguagesToIcon),
  "default_file.svg",
  "default_folder.svg",
  "default_folder_opened.svg",
  "default_root_folder.svg",
  "default_root_folder_opened.svg",
]);

// Add opened folder variants for every folder icon
for (const icon of [...allIcons]) {
  if (icon.startsWith("folder_type_") && !icon.includes("_opened")) {
    allIcons.add(icon.replace(".svg", "_opened.svg"));
  }
}

const BASE_URL =
  "https://raw.githubusercontent.com/vscode-icons/vscode-icons/master/icons";
const CONCURRENCY = 20;

async function downloadIcon(name) {
  try {
    const res = await fetch(`${BASE_URL}/${name}`);
    if (!res.ok) return false;
    const svg = await res.text();
    writeFileSync(join(OUT_DIR, name), svg);
    return true;
  } catch {
    return false;
  }
}

async function main() {
  mkdirSync(OUT_DIR, { recursive: true });

  // Skip if icons already downloaded
  const existing = readdirSync(OUT_DIR).filter((f) => f.endsWith(".svg"));
  if (existing.length > 600) {
    console.log(
      `[vscode-icons] ${existing.length} icons already present, skipping download.`,
    );
    return;
  }

  const icons = [...allIcons];
  console.log(`[vscode-icons] Downloading ${icons.length} icons...`);

  let downloaded = 0;
  let failed = 0;

  // Process in batches for controlled concurrency
  for (let i = 0; i < icons.length; i += CONCURRENCY) {
    const batch = icons.slice(i, i + CONCURRENCY);
    const results = await Promise.all(batch.map(downloadIcon));
    for (const ok of results) {
      if (ok) downloaded++;
      else failed++;
    }
  }

  console.log(
    `[vscode-icons] Done: ${downloaded} downloaded, ${failed} failed.`,
  );
}

main().catch(console.error);
