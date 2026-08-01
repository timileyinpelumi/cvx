"use client";

import { useId } from "react";

import styles from "./Stitch.module.css";

const STROKE = 2;
const MID = STROKE / 2;

type StitchProps = {
  width: number | string;
  animate?: boolean;
  className?: string;
};

export function Stitch({ width, animate = false, className }: StitchProps) {
  // useId emits characters that are not safe inside a url(#...) reference.
  const maskId = `stitch-${useId().replace(/[^a-zA-Z0-9]/g, "")}`;

  return (
    <svg
      className={className}
      width={width}
      height={STROKE}
      aria-hidden="true"
      focusable="false"
      fill="none"
    >
      <mask
        id={maskId}
        maskUnits="userSpaceOnUse"
        x="0"
        y="0"
        width="100%"
        height={STROKE}
      >
        <line
          className={animate ? styles.draw : undefined}
          x1="0"
          y1={MID}
          x2="100%"
          y2={MID}
          stroke="#fff"
          strokeWidth={STROKE}
          pathLength="1"
          strokeDasharray="1"
        />
      </mask>
      <line
        x1="0"
        y1={MID}
        x2="100%"
        y2={MID}
        stroke="var(--chalk, #2244D9)"
        strokeWidth={STROKE}
        strokeDasharray="6 4"
        mask={`url(#${maskId})`}
      />
    </svg>
  );
}
