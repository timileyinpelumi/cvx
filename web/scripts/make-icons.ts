import sharp from "sharp";

// The 32px favicon mark scaled to 512 with maskable-safe padding: the X sits
// inside the center 60% so a circular mask never clips it.
const svg = Buffer.from(`<svg xmlns="http://www.w3.org/2000/svg" width="512" height="512" viewBox="0 0 512 512">
  <rect width="512" height="512" fill="#EEF0EF"/>
  <g stroke="#2244D9" stroke-width="72" stroke-linecap="round">
    <line x1="166" y1="166" x2="346" y2="346"/>
    <line x1="346" y1="166" x2="166" y2="346"/>
  </g>
</svg>`);

await sharp(svg).png().toFile("public/icon-512.png");
await sharp(svg).resize(192, 192).png().toFile("public/icon-192.png");
console.log("wrote public/icon-512.png and public/icon-192.png");
