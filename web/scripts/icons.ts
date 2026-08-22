/**
 * Renders the app icon to the PNG sizes that browsers, iOS and Android ask
 * for. Run once and commit the output:
 *
 *   bun run scripts/icons.ts
 *
 * Deliberately not part of the build. The source of truth is icon.svg, the
 * PNGs change perhaps twice a year, and rasterising on every deploy would
 * put an image toolchain in the container for nothing.
 */
import { mkdir, writeFile } from "node:fs/promises";
import { readFileSync } from "node:fs";
import sharp from "sharp";

const SOURCE = "src/app/icon.svg";
const OUT = "public";

/** The sizes that are actually requested, and by whom. */
const SIZES = [
  { file: "icon-32.png", size: 32 },
  { file: "icon-192.png", size: 192 },
  { file: "icon-512.png", size: 512 },
  { file: "apple-touch-icon.png", size: 180 },
];

/** Android masks icons to a circle and crops ~10% on every edge, so a
 *  maskable icon needs its artwork inset or the corners get shaved off. */
const MASKABLE = { file: "icon-maskable-512.png", size: 512, padding: 0.1 };

const svg = readFileSync(SOURCE);

await mkdir(OUT, { recursive: true });

for (const { file, size } of SIZES) {
  const png = await sharp(svg, { density: 384 }).resize(size, size).png().toBuffer();
  await writeFile(`${OUT}/${file}`, png);
  console.log(`${file} ${size}x${size} ${png.length}b`);
}

const inner = Math.round(MASKABLE.size * (1 - MASKABLE.padding * 2));
const margin = Math.round((MASKABLE.size - inner) / 2);
const maskable = await sharp(svg, { density: 384 })
  .resize(inner, inner)
  .extend({
    top: margin,
    bottom: margin,
    left: margin,
    right: margin,
    background: "#2244D9",
  })
  .png()
  .toBuffer();
await writeFile(`${OUT}/${MASKABLE.file}`, maskable);
console.log(`${MASKABLE.file} ${MASKABLE.size}x${MASKABLE.size} ${maskable.length}b`);
