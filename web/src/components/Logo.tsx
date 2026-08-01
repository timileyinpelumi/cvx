const VIEW_W = 60;
const VIEW_H = 28;

// "cv" outlined from Bricolage Grotesque 700 at a 17.5px x-height on a 22.75
// baseline, so the wordmark is identical before and after the webfont loads.
const C_PATH =
  "M10.3 23.22Q8.03 23.22 6.33 22.55Q4.63 21.88 3.5 20.68Q2.37 19.48 1.8 17.83Q1.23 16.18 1.23 14.22Q1.23 12.15 1.82 10.43Q2.4 8.72 3.53 7.45Q4.67 6.18 6.33 5.48Q8.0 4.78 10.13 4.78Q12.27 4.78 13.85 5.48Q15.43 6.18 16.42 7.43Q17.4 8.68 17.77 10.38L13.33 11.82Q13.23 10.78 12.78 10.03Q12.33 9.28 11.58 8.9Q10.83 8.52 9.9 8.52Q8.93 8.52 8.23 8.9Q7.53 9.28 7.05 10.0Q6.57 10.72 6.32 11.73Q6.07 12.75 6.07 14.02Q6.07 15.82 6.55 17.07Q7.03 18.32 7.98 18.97Q8.93 19.62 10.33 19.62Q11.67 19.62 12.47 19.1Q13.27 18.58 13.65 17.77Q14.03 16.95 14.13 16.08L18.33 17.02Q18.17 18.32 17.62 19.45Q17.07 20.58 16.07 21.42Q15.07 22.25 13.65 22.73Q12.23 23.22 10.3 23.22Z";
const V_PATH =
  "M25.07 22.75 19.23 5.28H24.53L28.23 18.95H28.77L32.47 5.28H37.57L31.7 22.75Z";

type LogoProps = {
  height?: number;
  className?: string;
};

export function Logo({ height = VIEW_H, className }: LogoProps) {
  return (
    <svg
      className={className}
      role="img"
      aria-label="cvx"
      width={(height * VIEW_W) / VIEW_H}
      height={height}
      viewBox={`0 0 ${VIEW_W} ${VIEW_H}`}
      fill="none"
    >
      <title>cvx</title>
      <path d={C_PATH} fill="var(--ink)" />
      <path d={V_PATH} fill="var(--ink)" />
      <g stroke="var(--chalk)" strokeWidth="3.4" strokeLinecap="round">
        <line x1="41.27" y1="6.1" x2="57.07" y2="21.9" />
        <line x1="57.07" y1="6.1" x2="41.27" y2="21.9" />
      </g>
    </svg>
  );
}
