import { ImageResponse } from "next/og";

export const runtime = "edge";
export const alt = "cvx. Paste a job ad, get a resume.";
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

const INK = "#2244D9";

/** One typecase cell; the last one stays open — the gap the tool is honest about. */
function Cell({ open }: { open?: boolean }) {
  return (
    <div
      style={{
        width: 34,
        height: 34,
        borderRadius: 9,
        background: open ? "rgba(34,68,217,0.18)" : INK,
      }}
    />
  );
}

export default function OpenGraphImage() {
  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          flexDirection: "column",
          justifyContent: "space-between",
          background: "#f5f6f8",
          padding: 72,
          fontFamily: "sans-serif",
        }}
      >
        <div style={{ display: "flex", flexWrap: "wrap", gap: 12, width: 130 }}>
          {[0, 1, 2, 3, 4, 5, 6, 7].map((i) => (
            <Cell key={i} />
          ))}
          <Cell open />
        </div>

        <div style={{ display: "flex", flexDirection: "column" }}>
          <div style={{ fontSize: 96, fontWeight: 700, color: INK, letterSpacing: "-0.03em" }}>
            cvx
          </div>
          <div
            style={{
              marginTop: 16,
              fontSize: 44,
              fontWeight: 600,
              color: "#12161c",
              letterSpacing: "-0.02em",
            }}
          >
            Paste a job ad, get a resume.
          </div>
          <div style={{ marginTop: 14, fontSize: 26, color: "#5c6672" }}>
            One page, tailored to the role, built only from what you have actually done.
          </div>
        </div>
      </div>
    ),
    size,
  );
}
