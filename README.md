# bitcoin++ videos

A resilient field uploader for Bitcoin++ conference recordings. It is a Wails v2 desktop app with a Go multipart-upload worker and a Svelte 5 interface.

## What works

- Drag/drop or native file selection for MOV, MP4, MXF, MTS, and other rough recordings.
- A persistent upload queue that survives app restarts.
- Resumable DigitalOcean Spaces multipart uploads. Completed 64 MiB parts are recorded locally; a dropped connection retries with exponential backoff instead of restarting the whole file.
- Per-file and overall progress, current/average upload speed, remaining bytes, and ETA.
- Pause/resume/remove controls and automatic continuation after transient failures.
- SHA-256 fingerprinting before transfer, local and remote duplicate detection, and collision-safe object names.
- Events, days, and rooms load from the public Bitcoin++ API.
- Hackathon footage is available for events with a published hackathon.
- Conference, day, and room metadata are captured with every queued file.
- Spaces secrets are stored in the operating system keychain, not the queue file.

## Development

The included Nix shell contains Go, Node 22, and Wails:

```sh
nix develop
cd frontend && npm install && cd ..
wails dev
```

Build the desktop application with:

```sh
wails build
```

## Automated releases

GitHub Actions builds macOS ARM64, macOS Intel, Windows x64, and Linux x64 downloads whenever a version tag is pushed. To publish a release:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The workflow in `.github/workflows/release.yml` runs the tests, packages each platform, generates `SHA256SUMS.txt`, and attaches everything to the matching GitHub Release. These initial downloads are unsigned. macOS notarization and Windows code signing should be configured before distributing outside a small trusted team.

On macOS, Wails links against Apple frameworks. If a Nix-provided `ld` exits with a `Trace/BPT trap`, install/use a native Go toolchain for the packaging command (the Nix shell remains suitable for frontend work and Go tests). This is a local linker-toolchain issue, not a source compilation error.

For frontend-only work:

```sh
cd frontend
npm run dev
```

The browser preview includes representative upload data; the Wails build starts with the real persisted queue.

## Spaces setup

Open the gear menu and provide endpoint, region, bucket, access key, and secret. Use a narrowly scoped Spaces key. Rough mixes use this object-key convention:

```text
{conference-tag}/recordings/raw/{day}/{room}/{original-filename}
```

Days become compact path segments (`Day 1` → `day1`). Room names come directly from the API and are lowercased with spaces replaced by hyphens; the legacy `Main Stage` → `main` and `Talks Stage` → `talks` mappings remain supported. Select `Hackathon` under **Room** to upload footage for the selected day to `{conference-tag}/recordings/raw/{day}/hackathon/{original-filename}`. This room option is available only for events with a published hackathon.

Before uploading, the app computes the file's SHA-256 digest. Stored video keys receive a 12-character digest suffix (`filename--a1b2c3d4e5f6.mov`) so distinct recordings can never overwrite each other. Each upload directory also receives an immutable marker at `_manifest/sha256/{full-hash}.json` containing the original filename, final object key, size, and upload time. If that marker—or the same local fingerprint—already exists, the duplicate is skipped.

The uploader creates private objects by default. Access/publishing should be managed by the backend after ingestion.

## Bitcoin++ API integration

The desktop app reads published events from `GET /api/v1/conferences`, days and rooms from `GET /api/v1/conferences/{tag}/days`, and hackathon availability from `GET /api/v1/conferences/{tag}/hackathons`. These public reads require no authentication. Configure the API base URL in settings (default `https://btcpp.dev`; a URL ending in `/api/v1` is also accepted).

The app initially selects the current or nearest upcoming event, keeps past events available, and updates rooms when the event or day changes. Loading failures offer **Refresh events**; missing days or rooms prevent adding new files. A failed hackathon check displays a warning and leaves regular day uploads available. Existing queued uploads continue independently of the catalog. Browser preview uses explicitly labeled sample data.

Only published hackathons can be detected; an empty list does not reveal private or unpublished hackathons. The app adds `Hackathon` to the Room dropdown when the API reports a published hackathon. Spaces credentials are still configured separately and stored in the OS keychain. See [docs/backend-api.md](docs/backend-api.md) for the implemented catalog contract and future credential integration.

## Reliability notes

Queue metadata lives at the OS user config path under `btcpp-video/queue.json`. Removing a queue item does not abort its unfinished multipart upload in Spaces yet; a backend lifecycle rule should expire incomplete multipart uploads until an explicit abort flow is added.

Configure that safety-net rule without replacing other bucket lifecycle policies:

```sh
read "SPACES_ADMIN_KEY?Spaces admin access key: "
read -s "SPACES_ADMIN_SECRET?Spaces admin secret key: "
echo
export SPACES_ADMIN_KEY SPACES_ADMIN_SECRET

nix develop --command go run ./cmd/configure-lifecycle -days 7
nix develop --command go run ./cmd/configure-lifecycle -days 7 -apply

unset SPACES_ADMIN_KEY SPACES_ADMIN_SECRET
```

The command reads the endpoint, region, and bucket from the uploader's settings. The admin credentials exist only in that terminal session and are not written to disk. It preserves existing rules and adds or updates `btcpp-video-abort-incomplete-multipart`.
