"use client";

import { useCallback, useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { ArrowLeft, PenLine, Pin, Trash2 } from "lucide-react";
import { api } from "@/lib/api";
import { cx } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import { STATUS_TONES, STATUSES, type GenerationMeta, type Tailored } from "@/lib/types";
import { useToast } from "@/components/Toast";
import { Proof } from "@/components/Proof";
import { TailorEditor } from "@/components/TailorEditor";
import { FollowUp } from "@/components/FollowUp";
import { Button, EmptyState, ErrorState, Eyebrow, PageHeader, Skeleton } from "@/components/ui";

export default function GenerationDetailPage() {
  const params = useParams<{ id: string }>();
  const id = params.id;
  const router = useRouter();
  const toast = useToast();

  const [confirming, setConfirming] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [editing, setEditing] = useState<Tailored | null>(null);
  const [loadingEdit, setLoadingEdit] = useState(false);

  // The API has no single-generation endpoint; the list already carries the
  // full record, so the detail view reads from it. null means "loaded, but no
  // such id" — undefined is still loading.
  const fetcher = useCallback(
    async () => (await api.generations()).find((g) => g.id === id) ?? null,
    [id],
  );
  const { data: row, error, reload } = useResource<GenerationMeta | null>(fetcher);

  async function togglePin() {
    if (!row) return;
    try {
      await api.setPinned(id, !row.pinned);
      reload();
    } catch (err) {
      toast(err instanceof Error ? err.message : "Couldn't pin that one", "error");
    }
  }

  async function setStatus(status: string) {
    try {
      await api.setStatus(id, status);
      reload();
    } catch (err) {
      toast(err instanceof Error ? err.message : "Couldn't save that", "error");
    }
  }

  async function startEditing() {
    setLoadingEdit(true);
    try {
      setEditing(await api.tailored(id));
    } catch (err) {
      toast(err instanceof Error ? err.message : "Couldn't load the content", "error");
    } finally {
      setLoadingEdit(false);
    }
  }

  async function remove() {
    setDeleting(true);
    try {
      await api.deleteGeneration(id);
      toast("Resume deleted.", "success");
      router.push("/resumes");
    } catch (err) {
      toast(err instanceof Error ? err.message : "Couldn't delete that one", "error");
      setDeleting(false);
      setConfirming(false);
    }
  }

  return (
    <>
      <PageHeader
        title={row?.targetRole || "Resume"}
        meta={
          <Link href="/resumes" className="inline-flex items-center gap-1.5 hover:text-fg">
            <ArrowLeft size={12} />
            Back to resumes
          </Link>
        }
        actions={
          row ? (
            confirming ? (
              <div className="flex items-center gap-2">
                <span className="hidden text-[12.5px] text-fg-muted sm:inline">Delete this resume?</span>
                <Button size="sm" variant="danger" onClick={remove} loading={deleting}>
                  Delete
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setConfirming(false)} disabled={deleting}>
                  Cancel
                </Button>
              </div>
            ) : (
              <div className="flex items-center gap-1">
                {!editing && (
                  <Button size="sm" variant="ghost" onClick={startEditing} loading={loadingEdit}>
                    <PenLine size={13} />
                    Edit
                  </Button>
                )}
                <Button size="sm" variant="ghost" onClick={togglePin} aria-pressed={row.pinned}>
                  <Pin size={13} fill={row.pinned ? "currentColor" : "none"} />
                  {row.pinned ? "Pinned" : "Pin"}
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setConfirming(true)}>
                  <Trash2 size={13} />
                  Delete
                </Button>
              </div>
            )
          ) : null
        }
      />

      {/* The editor runs a preview pane beside itself, so it gets the wider
          measure; everything else stays at reading width. */}
      <div
        className={cx(
          "mx-auto px-4 py-5 sm:px-7 sm:py-6",
          editing ? "max-w-[78rem]" : "max-w-[46rem]",
        )}
      >
        {error ? (
          <ErrorState message={error} onRetry={reload} />
        ) : row === undefined ? (
          <div className="space-y-3">
            <Skeleton className="h-28 w-full" />
            <Skeleton className="h-40 w-full" />
          </div>
        ) : row === null ? (
          <EmptyState
            title="Can't find that one"
            body="It may have been deleted. The rest are still in your resumes."
            action={
              <Link href="/resumes">
                <Button variant="primary">Back to resumes</Button>
              </Link>
            }
          />
        ) : editing ? (
          <TailorEditor
            id={id}
            initial={editing}
            onSaved={() => {
              setEditing(null);
              reload();
            }}
            onCancel={() => setEditing(null)}
          />
        ) : (
          <>
            <div className="mb-4">
              <Eyebrow>Where it stands</Eyebrow>
              <div className="mt-2 flex flex-wrap gap-1.5">
                {STATUSES.map((s) => (
                  <button
                    key={s.value}
                    type="button"
                    onClick={() => setStatus(s.value)}
                    aria-pressed={row.status === s.value}
                    className={cx(
                      "h-7 rounded-full border px-2.5 text-[12px] font-medium transition-colors duration-[130ms]",
                      row.status === s.value
                        ? s.value === ""
                          ? "border-line-strong bg-sunken text-fg"
                          : STATUS_TONES[s.value].chip
                        : "border-line text-fg-muted hover:border-line-strong hover:text-fg",
                    )}
                  >
                    {s.label}
                  </button>
                ))}
              </div>
            </div>

            <FollowUp id={row.id} status={row.status} statusAt={row.statusAt} />

            <Proof
              data={{
                id: row.id,
                targetRole: row.targetRole,
                roleSummary: row.roleSummary,
                filename: row.filename,
                gaps: row.gaps,
                whatChanged: row.whatChanged,
                hasCoverLetter: row.hasCoverLetter,
                createdAt: row.createdAt,
                fit: row.fit,
                coverage: row.coverage,
              }}
            />
          </>
        )}
      </div>
    </>
  );
}
