# Bitcoin++ API integration

The app uses the public v1 API at `https://btcpp.dev/api/v1`:

- `GET /conferences?limit=100`: follows `meta.next_cursor` to list all published events. Uses `id`, `tag`, `description`, `location`, `starts_at`, and `ends_at`.
- `GET /conferences/{tag}/days`: reads `day_number` and `venues` from each day. Venues are room names; no fixed room list is assumed.
- `GET /conferences/{tag}/hackathons?limit=1`: a nonempty `data` array enables the **Hackathon** room choice. Only publicly exposed hackathons are discoverable. Errors are surfaced separately from “no hackathon.”

Responses wrap results in `{ "data": ..., "meta": ... }`. Requests run through the Go backend with a 15-second timeout per request, avoiding WebView CORS restrictions. These reads receive no Spaces credentials.

Normal footage retains `{tag}/recordings/raw/day{number}/{room}/{filename}`. Hackathon footage uses `{tag}/recordings/raw/day{number}/hackathon/{filename}`, including the usual hash suffix and per-directory duplicate manifests. `destination.room` stores `Hackathon`, while `destination.day` retains the selected day, preserving the queue schema and existing queued paths. The app adds this room option when the API reports a published hackathon.

Events without published days cannot accept new footage yet. Days without venues can only accept hackathon footage when a published hackathon exists. The catalog is fetched on startup and refresh, and after saving settings; it is not cached for offline destination selection. Previously queued transfers remain resumable independently.

## Future authenticated upload API (proposal; not implemented)

The desktop app needs one authenticated endpoint to remove destination and credential decisions from volunteers.

## `GET /api/video-upload/context`

Returns the event currently in progress (or the nearest upcoming event), allowed recording locations, and temporary S3-compatible credentials.

```json
{
  "conference": {
    "id": "3ac…",
    "tag": "toronto",
    "name": "Bitcoin++ Toronto 2026",
    "timezone": "America/Toronto",
    "startDate": "2026-07-22",
    "endDate": "2026-07-24"
  },
  "days": [
    { "id": "2026-07-22", "label": "Day 1" },
    { "id": "2026-07-23", "label": "Day 2" },
    { "id": "2026-07-24", "label": "Day 3" }
  ],
  "rooms": [
    { "id": "main-stage", "label": "Main Stage" },
    { "id": "talks-stage", "label": "Talks Stage" }
  ],
  "upload": {
    "endpoint": "https://nyc3.digitaloceanspaces.com",
    "region": "nyc3",
    "bucket": "btcpp",
    "keyPrefix": "toronto/recordings/raw",
    "accessKeyId": "temporary-key",
    "secretAccessKey": "temporary-secret",
    "sessionToken": "if-supported",
    "expiresAt": "2026-07-19T20:30:00Z"
  }
}
```

The credentials should allow only multipart create/upload/list/complete/abort beneath `keyPrefix`. If DigitalOcean Spaces cannot issue short-lived credentials with the needed scope, return presigned URLs from a multipart session API instead.

## Optional upload registration

`POST /api/video-upload/files` can register a completed rough mix with its conference ID, day, room, object key, byte size, checksum, original filename, and client upload ID. This gives the later editing workflow a reliable ingest manifest without listing the bucket.
