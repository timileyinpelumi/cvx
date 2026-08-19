import { cx } from "@/lib/format";

/** The mark stays quiet — the ink lands on the x, and nowhere else in the
 *  chassis. It is the same ink the resume prints in. */
export function Wordmark({ className }: { className?: string }) {
  return (
    <span
      className={cx(
        "font-display text-[19px] font-extrabold leading-none tracking-[-0.05em] select-none",
        className,
      )}
    >
      cv<span className="text-ink">x</span>
    </span>
  );
}
