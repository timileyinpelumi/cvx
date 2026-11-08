<img src="web/public/icon-192.png" alt="" width="76">

# cvx

Paste a job ad, get a one page resume aimed at it.

## The idea

You give cvx your history once. Every resume after that is chosen and reworded
from that one record.

Code enforces this. Every line on the page has to cite the profile entry it
came from, or the generation is rejected. A resume can use the job's own words,
and it can't claim something you didn't do.

## What it does

**Builds your profile once.** Upload a CV, paste your LinkedIn, or just write
a few sentences. cvx reads it into a structured record, then asks about the
things it can't work out for itself — dates, and what actually changed
because of your work.

**Tailors a resume per job.** Paste the ad. It picks what fits, orders it by
relevance, and writes a summary and headline in the job's vocabulary. To fit
exactly one page, it selects more than fits and trims from the weakest end.

**Tells you where you stand.** A fit score, the requirements you're missing,
and which of the technologies the ad names appear on your page.

**Writes the rest of the application.** A cover letter and a forwardable email
to the recruiter, both checked against your profile for claims it can't
support before you send them.

**Keeps track.** Which jobs you sent to, what came back, and a nudge when
something has gone quiet. Requirements that keep coming up as gaps turn into
a list of what to learn next.

## Editing

Everything is editable, with the real page beside you. Rewrite a single line,
add back something the model skipped, reorder anything, and see the PDF and
how full the page is before you save. Every bullet shows the profile entry it
came from.

## Running it

Needs Docker and a [Groq](https://console.groq.com) API key.

```
git clone https://github.com/timileyinpelumi/cvx && cd cvx
cp .env.example .env      # fill in the key and a session secret
docker compose up -d --build
```

The whole app is one container: a Go API and a Next.js frontend behind one
port, with SQLite on a disk beside them. A backup is one file.

For development, `cd server && go run ./cmd/cvx` and `cd web && bun run dev`.

Deploying it somewhere real: [docs/deploy.md](docs/deploy.md).

## How it's built

| | |
|---|---|
| API | Go, Echo, raw SQL |
| Web | Next.js, Tailwind |
| Storage | SQLite on a volume |
| PDFs | generated in Go, then read back to check what actually landed on the page |
| Models | Groq by default, OpenAI or Anthropic by config |

An admin panel at `/admin` shows what it's doing, what it's costing per
model, what's failing, and how good the output has been.
