package uploader

import "time"

type Destination struct {
	ConferenceID   string `json:"conferenceId"`
	ConferenceTag  string `json:"conferenceTag"`
	ConferenceName string `json:"conferenceName"`
	Day            string `json:"day"`
	Room           string `json:"room"`
}

type Settings struct {
	APIBaseURL string `json:"apiBaseUrl"`
	Endpoint   string `json:"endpoint"`
	Region     string `json:"region"`
	Bucket     string `json:"bucket"`
	AccessKey  string `json:"accessKey"`
	SecretSet  bool   `json:"secretSet"`
	SecretKey  string `json:"secretKey,omitempty"`
	PartSizeMB int64  `json:"partSizeMb"`
	AutoStart  bool   `json:"autoStart"`
}

type CompletedPart struct {
	Number int32  `json:"number"`
	ETag   string `json:"etag"`
	Size   int64  `json:"size"`
}

type Item struct {
	ID           string          `json:"id"`
	Path         string          `json:"path"`
	Name         string          `json:"name"`
	Size         int64           `json:"size"`
	HashSHA256   string          `json:"sha256,omitempty"`
	HashProgress int64           `json:"hashProgress,omitempty"`
	Uploaded     int64           `json:"uploaded"`
	Status       string          `json:"status"`
	ObjectKey    string          `json:"objectKey"`
	Destination  Destination     `json:"destination"`
	UploadID     string          `json:"uploadId,omitempty"`
	Parts        []CompletedPart `json:"parts,omitempty"`
	Error        string          `json:"error,omitempty"`
	AddedAt      time.Time       `json:"addedAt"`
	CompletedAt  *time.Time      `json:"completedAt,omitempty"`
	SpeedBps     float64         `json:"speedBps"`
	ETASeconds   int64           `json:"etaSeconds"`
	Retries      int             `json:"retries"`
	DuplicateOf  string          `json:"duplicateOf,omitempty"`
}

type Snapshot struct {
	Revision        uint64  `json:"revision"`
	Items           []Item  `json:"items"`
	Online          bool    `json:"online"`
	Running         bool    `json:"running"`
	CurrentSpeedBps float64 `json:"currentSpeedBps"`
	AverageSpeedBps float64 `json:"averageSpeedBps"`
	SessionUploaded int64   `json:"sessionUploaded"`
	TotalBytes      int64   `json:"totalBytes"`
	UploadedBytes   int64   `json:"uploadedBytes"`
	PendingBytes    int64   `json:"pendingBytes"`
	LastError       string  `json:"lastError,omitempty"`
}
