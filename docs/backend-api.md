# Proposed btcpp-web upload API

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
