"use client";

import { useRef, useState } from "react";
import { FileText, PenLine, Upload } from "lucide-react";
import { api } from "@/lib/api";
import { cx } from "@/lib/format";
import type { ProfileSummary } from "@/lib/types";
import { IntakeQuestions } from "./IntakeQuestions";
import { Button } from "./ui";
import { Wordmark } from "./Wordmark";
import { useSession } from "./Session";
import { useToast } from "./Toast";

type Door = "upload" | "paste" | "scratch";

/** The one thing that must exist before anything else works: the record every
 *  tailored resume is drawn from.
 *
 *  Three ways in, because the upload-only version was a wall for anyone who
 *  has never had a CV: students, career changers, and the many people whose
 *  history lives on LinkedIn or in a document they have never exported. All
 *  three produce the same thing, and the questions afterwards are what make
 *  a thin start into a usable profile. */
export function Onboarding() {
  const [door, setDoor] = useState<Door>("upload");
  // Set once an intake has saved a profile server-side. The app stays
  // gated until the questions are answered or skipped, because that step is
  // never coming back once someone is inside and composing.
  const [saved, setSaved] = useState<ProfileSummary | null>(null);

  if (saved) return <SecondStep summary={saved} />;

  return (
    <div className="mx-auto flex min-h-dvh max-w-[38rem] flex-col justify-center px-6 py-16">
      <Wordmark />

      <h1 className="mt-7 font-display text-[26px] font-extrabold leading-[1.15] tracking-[-0.03em]">
        First, tell cvx what you&rsquo;ve done.
      </h1>
      <p className="mt-3 text-[14.5px] leading-relaxed text-fg-muted">
        This becomes your profile, and every resume cvx makes comes from it. It only ever reorders
        and rewords what&rsquo;s in here, so it can never claim something you haven&rsquo;t done.
      </p>

      <div className="mt-7 flex gap-1.5">
        {(
          [
            { id: "upload", label: "Upload a CV", icon: Upload },
            { id: "paste", label: "Paste anything", icon: FileText },
            { id: "scratch", label: "I have neither", icon: PenLine },
          ] as const
        ).map((option) => (
          <button
            key={option.id}
            type="button"
            aria-pressed={door === option.id}
            onClick={() => setDoor(option.id)}
            className={cx(
              "flex h-9 shrink-0 items-center gap-1.5 rounded-full border px-3.5 text-[12.5px] font-medium",
              "transition-colors duration-[130ms]",
              door === option.id
                ? "border-ink bg-ink-soft text-ink"
                : "border-line text-fg-muted hover:border-line-strong hover:text-fg",
            )}
          >
            <option.icon size={13} />
            {option.label}
          </button>
        ))}
      </div>

      <div className="mt-5">
        {door === "upload" && <UploadDoor onSaved={setSaved} />}
        {door === "paste" && <TextDoor variant="paste" onSaved={setSaved} />}
        {door === "scratch" && <TextDoor variant="scratch" onSaved={setSaved} />}
      </div>
    </div>
  );
}

/** What cvx read, and what it still needs. Shown before the app unlocks:
 *  dates and numbers are the two things the guardrail forbids inventing, so
 *  the only way onto the page is to ask, and asking later means never. */
function SecondStep({ summary }: { summary: ProfileSummary }) {
  const { setProfile } = useSession();

  return (
    <div className="mx-auto min-h-dvh max-w-[38rem] px-6 py-16">
      <Wordmark />

      <h1 className="mt-7 font-display text-[26px] font-extrabold leading-[1.15] tracking-[-0.03em]">
        {summary.itemCount === 0
          ? "Saved. Now the parts cvx can't work out."
          : `Got ${summary.itemCount} ${summary.itemCount === 1 ? "entry" : "entries"} and ${summary.skillCount} skills.`}
      </h1>
      <p className="mt-3 text-[14.5px] leading-relaxed text-fg-muted">
        A couple of things it cannot fill in for you. Answer what you like, skip the rest, and
        change any of it later.
      </p>

      <div className="mt-7">
        <IntakeQuestions compact />
      </div>

      <Button variant="primary" className="mt-6" onClick={() => setProfile(summary)}>
        Start composing
      </Button>
    </div>
  );
}

function UploadDoor({ onSaved }: { onSaved: (summary: ProfileSummary) => void }) {
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const [dragging, setDragging] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  async function upload(file: File) {
    if (file.type !== "application/pdf" && !file.name.toLowerCase().endsWith(".pdf")) {
      toast("That file needs to be a PDF.", "error");
      return;
    }
    setBusy(true);
    try {
      onSaved(await api.uploadProfile(file));
    } catch (err) {
      toast(err instanceof Error ? err.message : "That upload didn't work", "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <div
        onDragOver={(e) => {
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDragging(false);
          const file = e.dataTransfer.files?.[0];
          if (file) upload(file);
        }}
        className={cx(
          "flex flex-col items-center rounded-[var(--radius-panel)] border border-dashed px-6 py-12",
          "transition-colors duration-[130ms]",
          dragging ? "border-ink bg-ink-soft" : "border-line-strong bg-raised",
        )}
      >
        <Upload size={20} className="text-fg-faint" />
        <p className="mt-3 text-[13.5px] text-fg-muted">Drop a PDF here</p>
        <Button
          variant="primary"
          className="mt-4"
          loading={busy}
          onClick={() => inputRef.current?.click()}
        >
          {busy ? "Reading your resume" : "Choose a file"}
        </Button>
        <input
          ref={inputRef}
          type="file"
          accept="application/pdf,.pdf"
          className="sr-only"
          onChange={(e) => {
            const file = e.target.files?.[0];
            if (file) upload(file);
            e.target.value = "";
          }}
        />
      </div>

      <p className="num mt-4 text-[11.5px] text-fg-faint">PDF, up to 15 MB</p>
    </>
  );
}

const COPY = {
  paste: {
    hint: "Your LinkedIn about section, an old bio, a job description of your last role. Anything that says what you have done.",
    placeholder:
      "Backend engineer at Venix since 2023. I built their payment service in Python and Postgres, and took checkout errors down by about a third. Before that I was at ChainPal for two years working on settlement.",
    action: "Read this",
  },
  scratch: {
    hint: "Write it however it comes out. Where you have worked, what you built, what you studied. cvx will sort it into a profile and ask about anything it needs.",
    placeholder:
      "I finished my computer engineering degree at FUTA last year. I interned at a fintech for six months where I worked on their API in Python, and I have built a couple of side projects, one of them a chat app with about 200 users.",
    action: "Build my profile",
  },
} as const;

function TextDoor({
  variant,
  onSaved,
}: {
  variant: "paste" | "scratch";
  onSaved: (summary: ProfileSummary) => void;
}) {
  const toast = useToast();
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);

  const copy = COPY[variant];
  const enough = text.trim().length >= 40;

  async function submit() {
    setBusy(true);
    try {
      onSaved(await api.profileFromText(text.trim()));
    } catch (err) {
      toast(err instanceof Error ? err.message : "cvx couldn't read that", "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <p className="text-[13px] leading-relaxed text-fg-muted">{copy.hint}</p>
      <textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder={copy.placeholder}
        className={cx(
          "mt-3 h-56 w-full rounded-[var(--radius-panel)] border border-line bg-raised p-4",
          "text-[13.5px] leading-relaxed outline-none transition-colors duration-[130ms] focus:border-ink",
        )}
      />
      <div className="mt-4 flex items-center gap-3">
        <Button variant="primary" loading={busy} disabled={!enough} onClick={submit}>
          {busy ? "Reading it" : copy.action}
        </Button>
        <p className="text-[11.5px] text-fg-faint">
          {enough ? "cvx will ask about anything it needs" : "A few sentences is enough to start"}
        </p>
      </div>
    </>
  );
}
