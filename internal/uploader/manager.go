package uploader

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/zalando/go-keyring"
)

const keyringService = "btcpp-video-uploader"

type stateFile struct {
	Settings Settings `json:"settings"`
	Items    []Item   `json:"items"`
}

type Manager struct {
	mu              sync.Mutex
	state           stateFile
	statePath       string
	emit            func(Snapshot)
	wake            chan struct{}
	stop            chan struct{}
	cancels         map[string]context.CancelFunc
	started         bool
	currentSpeed    float64
	averageSpeed    float64
	sessionUploaded int64
	sessionStarted  time.Time
	online          bool
	lastError       string
	revision        uint64
}

func New(emit func(Snapshot)) (*Manager, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, "btcpp-video")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	m := &Manager{statePath: filepath.Join(dir, "queue.json"), emit: emit, wake: make(chan struct{}, 1), stop: make(chan struct{}), cancels: map[string]context.CancelFunc{}, online: true}
	m.state.Settings = Settings{Region: "nyc3", Endpoint: "https://nyc3.digitaloceanspaces.com", PartSizeMB: 64, AutoStart: true}
	if b, err := os.ReadFile(m.statePath); err == nil {
		_ = json.Unmarshal(b, &m.state)
	}
	m.state.Settings.Bucket = strings.ToLower(strings.TrimSpace(m.state.Settings.Bucket))
	kept := m.state.Items[:0]
	for i := range m.state.Items {
		if m.state.Items[i].Status == "uploading" {
			m.state.Items[i].Status = "queued"
		}
		if m.state.Items[i].Status != "removed" {
			kept = append(kept, m.state.Items[i])
		}
	}
	m.state.Items = kept
	return m, nil
}

func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.sessionStarted = time.Now()
	m.mu.Unlock()
	go m.loop(ctx)
	m.signal()
}
func (m *Manager) Stop() {
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}
	m.mu.Lock()
	for _, cancel := range m.cancels {
		cancel()
	}
	m.persistLocked()
	m.mu.Unlock()
}
func (m *Manager) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) AddFiles(paths []string, dest Destination) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.TrimSpace(dest.ConferenceTag) == "" || strings.TrimSpace(dest.Day) == "" || strings.TrimSpace(dest.Room) == "" {
		return m.snapshotLocked(), fmt.Errorf("choose an event, day, and room before adding files")
	}
	known := map[string]bool{}
	for _, item := range m.state.Items {
		known[item.Path] = item.Status != "complete" && item.Status != "removed"
	}
	var errs []string
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			errs = append(errs, filepath.Base(path))
			continue
		}
		if known[path] {
			continue
		}
		now := time.Now()
		id := fmt.Sprintf("%d", now.UnixNano())
		key := objectKey(dest, info.Name())
		m.state.Items = append(m.state.Items, Item{ID: id, Path: path, Name: info.Name(), Size: info.Size(), Status: "queued", ObjectKey: key, Destination: dest, AddedAt: now})
	}
	m.persistLocked()
	snap := m.snapshotLocked()
	go m.signal()
	go m.publish(snap)
	if len(errs) > 0 {
		return snap, fmt.Errorf("could not add: %s", strings.Join(errs, ", "))
	}
	return snap, nil
}

func objectKey(d Destination, name string) string {
	segments := []string{clean(d.ConferenceTag), "recordings", "raw"}
	if d.Day != "" {
		segments = append(segments, destinationDay(d.Day))
	}
	if d.Room != "" {
		segments = append(segments, destinationRoom(d.Room))
	}
	segments = append(segments, strings.ReplaceAll(filepath.Base(name), "/", "-"))
	return strings.Join(segments, "/")
}

func destinationDay(day string) string {
	return strings.ReplaceAll(clean(day), "-", "")
}

func destinationRoom(room string) string {
	switch clean(room) {
	case "main-stage":
		return "main"
	case "talks-stage":
		return "talks"
	default:
		return clean(room)
	}
}
func clean(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(" ", "-", "/", "-", "\\", "-").Replace(s)
	return strings.Trim(s, "-")
}

func (m *Manager) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stop:
			return
		case <-m.wake:
		}
		for {
			m.mu.Lock()
			idx := -1
			for i := range m.state.Items {
				if m.state.Items[i].Status == "queued" {
					idx = i
					break
				}
			}
			m.mu.Unlock()
			if idx < 0 {
				break
			}
			m.upload(ctx, idx)
		}
	}
}

func (m *Manager) upload(parent context.Context, idx int) {
	m.mu.Lock()
	if idx >= len(m.state.Items) || (m.state.Items[idx].Status != "queued" && m.state.Items[idx].Status != "waiting") {
		m.mu.Unlock()
		return
	}
	item := m.state.Items[idx]
	settings := m.state.Settings
	item.Status = "uploading"
	item.Error = ""
	m.state.Items[idx] = item
	ctx, cancel := context.WithCancel(parent)
	m.cancels[item.ID] = cancel
	m.persistLocked()
	m.publishLocked()
	m.mu.Unlock()
	defer func() { m.mu.Lock(); delete(m.cancels, item.ID); m.mu.Unlock() }()
	secret, _ := keyring.Get(keyringService, "spaces-secret")
	if settings.SecretKey != "" {
		secret = settings.SecretKey
	}
	if settings.Endpoint == "" || settings.Bucket == "" || settings.AccessKey == "" || secret == "" {
		m.fail(idx, "DigitalOcean Spaces is not configured", false)
		return
	}
	cfg := aws.Config{Region: settings.Region, Credentials: credentials.NewStaticCredentialsProvider(settings.AccessKey, secret, ""), HTTPClient: &http.Client{Timeout: 0}}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) { o.BaseEndpoint = aws.String(settings.Endpoint); o.UsePathStyle = false })
	file, err := os.Open(item.Path)
	if err != nil {
		m.fail(idx, "Source file is no longer available", false)
		return
	}
	defer file.Close()
	if item.HashSHA256 == "" {
		item.Status = "hashing"
		m.updateItem(idx, item)
		hash, err := m.hashFile(ctx, idx, file)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			m.fail(idx, fmt.Sprintf("Could not fingerprint file: %v", err), false)
			return
		}
		item = m.item(idx)
		item.HashSHA256 = hash
		item.HashProgress = item.Size
		if item.UploadID == "" {
			item.ObjectKey = hashedObjectKey(objectKey(item.Destination, item.Name), hash)
		}
		if duplicate := m.localDuplicate(idx, item); duplicate != "" {
			m.markDuplicate(idx, item, duplicate)
			return
		}
		m.updateItem(idx, item)
	}
	if duplicate, err := remoteDuplicate(ctx, client, settings.Bucket, item); err != nil {
		m.retry(idx, err)
		return
	} else if duplicate {
		m.markDuplicate(idx, item, "Already present in the remote hash manifest")
		return
	}
	item = m.item(idx)
	item.Status = "uploading"
	item.Error = ""
	m.updateItem(idx, item)
	if _, err := file.Seek(0, 0); err != nil {
		m.fail(idx, err.Error(), false)
		return
	}
	partSize := settings.PartSizeMB * 1024 * 1024
	if partSize < 5*1024*1024 {
		partSize = 64 * 1024 * 1024
	}
	if item.UploadID == "" {
		contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(item.Name)))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		created, err := client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: aws.String(settings.Bucket), Key: aws.String(item.ObjectKey), ContentType: aws.String(contentType), Metadata: map[string]string{"sha256": item.HashSHA256}})
		if err != nil {
			m.retry(idx, err)
			return
		}
		item.UploadID = aws.ToString(created.UploadId)
		m.updateItem(idx, item)
	}
	completed := map[int32]CompletedPart{}
	for _, p := range item.Parts {
		completed[p.Number] = p
	}
	parts := int((item.Size + partSize - 1) / partSize)
	for n := 1; n <= parts; n++ {
		pn := int32(n)
		offset := int64(n-1) * partSize
		size := min64(partSize, item.Size-offset)
		if _, ok := completed[pn]; ok {
			continue
		}
		if _, err := file.Seek(offset, 0); err != nil {
			m.fail(idx, err.Error(), false)
			return
		}
		reader := &progressReader{r: file, remaining: size, onProgress: func(delta int64, speed float64) { m.progress(idx, delta, speed) }}
		out, err := client.UploadPart(ctx, &s3.UploadPartInput{Bucket: aws.String(settings.Bucket), Key: aws.String(item.ObjectKey), UploadId: aws.String(item.UploadID), PartNumber: aws.Int32(pn), Body: reader, ContentLength: aws.Int64(size)})
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			m.retry(idx, err)
			return
		}
		item = m.item(idx)
		item.Parts = append(item.Parts, CompletedPart{Number: pn, ETag: aws.ToString(out.ETag), Size: size})
		sort.Slice(item.Parts, func(i, j int) bool { return item.Parts[i].Number < item.Parts[j].Number })
		item.Uploaded = completedBytes(item.Parts)
		m.updateItem(idx, item)
	}
	item = m.item(idx)
	completedParts := make([]s3types.CompletedPart, 0, len(item.Parts))
	for _, p := range item.Parts {
		completedParts = append(completedParts, s3types.CompletedPart{ETag: aws.String(p.ETag), PartNumber: aws.Int32(p.Number)})
	}
	_, err = client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{Bucket: aws.String(settings.Bucket), Key: aws.String(item.ObjectKey), UploadId: aws.String(item.UploadID), MultipartUpload: &s3types.CompletedMultipartUpload{Parts: completedParts}})
	if err != nil {
		m.retry(idx, err)
		return
	}
	now := time.Now()
	item.Status = "complete"
	item.Uploaded = item.Size
	item.SpeedBps = 0
	item.ETASeconds = 0
	item.CompletedAt = &now
	if err := writeManifest(ctx, client, settings.Bucket, item); err != nil {
		item.Error = fmt.Sprintf("Uploaded, but manifest write failed: %v", err)
	}
	m.updateItem(idx, item)
}

type manifestRecord struct {
	SHA256           string    `json:"sha256"`
	ObjectKey        string    `json:"objectKey"`
	OriginalFilename string    `json:"originalFilename"`
	Size             int64     `json:"size"`
	UploadedAt       time.Time `json:"uploadedAt"`
}

func hashedObjectKey(base, hash string) string {
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return stem + "--" + hash[:12] + ext
}

func manifestKey(item Item) string {
	return filepath.ToSlash(filepath.Join(filepath.Dir(item.ObjectKey), "_manifest", "sha256", item.HashSHA256+".json"))
}

func remoteDuplicate(ctx context.Context, client *s3.Client, bucket string, item Item) (bool, error) {
	_, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(manifestKey(item))})
	if err == nil {
		return true, nil
	}
	if !isNotFound(err) {
		return false, fmt.Errorf("check remote hash manifest: %w", err)
	}
	existing, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(item.ObjectKey)})
	if err == nil {
		if existing.Metadata["sha256"] == item.HashSHA256 {
			return true, nil
		}
		return false, fmt.Errorf("collision-safe target already exists with different content: %s", item.ObjectKey)
	}
	if isNotFound(err) {
		return false, nil
	}
	return false, fmt.Errorf("check collision-safe target: %w", err)
}

func writeManifest(ctx context.Context, client *s3.Client, bucket string, item Item) error {
	record := manifestRecord{SHA256: item.HashSHA256, ObjectKey: item.ObjectKey, OriginalFilename: item.Name, Size: item.Size, UploadedAt: time.Now().UTC()}
	body, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(manifestKey(item)), Body: bytes.NewReader(body), ContentLength: aws.Int64(int64(len(body))), ContentType: aws.String("application/json"), Metadata: map[string]string{"sha256": item.HashSHA256}})
	return err
}

func isNotFound(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	code := strings.ToLower(apiErr.ErrorCode())
	return code == "notfound" || code == "nosuchkey" || code == "404"
}

func (m *Manager) hashFile(ctx context.Context, idx int, file *os.File) (string, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}
	h := sha256.New()
	buf := make([]byte, 4*1024*1024)
	var read int64
	lastUpdate := time.Now()
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		n, err := file.Read(buf)
		if n > 0 {
			if _, writeErr := h.Write(buf[:n]); writeErr != nil {
				return "", writeErr
			}
			read += int64(n)
			if time.Since(lastUpdate) >= 150*time.Millisecond {
				m.mu.Lock()
				if idx < len(m.state.Items) {
					m.state.Items[idx].HashProgress = read
					m.publishLocked()
				}
				m.mu.Unlock()
				lastUpdate = time.Now()
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func (m *Manager) localDuplicate(idx int, item Item) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, existing := range m.state.Items {
		if i == idx || existing.Status == "removed" || existing.HashSHA256 == "" {
			continue
		}
		if existing.HashSHA256 == item.HashSHA256 && filepath.Dir(existing.ObjectKey) == filepath.Dir(item.ObjectKey) && (existing.Status == "complete" || existing.Status == "duplicate" || existing.UploadID != "") {
			return existing.ObjectKey
		}
	}
	return ""
}

func (m *Manager) markDuplicate(idx int, item Item, duplicateOf string) {
	now := time.Now()
	item.Status = "duplicate"
	item.DuplicateOf = duplicateOf
	item.Error = "Duplicate content — upload skipped"
	item.SpeedBps = 0
	item.ETASeconds = 0
	item.CompletedAt = &now
	m.updateItem(idx, item)
}

type progressReader struct {
	r          *os.File
	remaining  int64
	onProgress func(int64, float64)
	last       time.Time
}

func (r *progressReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	if r.last.IsZero() {
		r.last = time.Now()
	}
	n, e := r.r.Read(p)
	r.remaining -= int64(n)
	elapsed := time.Since(r.last).Seconds()
	if n > 0 {
		r.onProgress(int64(n), float64(n)/maxFloat(elapsed, .001))
		r.last = time.Now()
	}
	if r.remaining == 0 && e == nil {
		e = io.EOF
	}
	return n, e
}

func (m *Manager) progress(idx int, delta int64, instant float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if idx >= len(m.state.Items) {
		return
	}
	it := m.state.Items[idx]
	it.Uploaded = min64(it.Size, it.Uploaded+delta)
	m.currentSpeed = .82*m.currentSpeed + .18*instant
	it.SpeedBps = m.currentSpeed
	if m.currentSpeed > 0 {
		it.ETASeconds = int64(float64(it.Size-it.Uploaded) / m.currentSpeed)
	}
	m.sessionUploaded += delta
	elapsed := time.Since(m.sessionStarted).Seconds()
	if elapsed > 0 {
		m.averageSpeed = float64(m.sessionUploaded) / elapsed
	}
	m.state.Items[idx] = it
	m.online = true
	m.publishLocked()
}
func (m *Manager) retry(idx int, err error) {
	m.mu.Lock()
	if idx >= len(m.state.Items) {
		m.mu.Unlock()
		return
	}
	it := m.state.Items[idx]
	it.Status = "waiting"
	it.Error = fmt.Sprintf("Upload interrupted: %v — retrying automatically", err)
	it.Retries++
	it.Uploaded = completedBytes(it.Parts)
	it.SpeedBps = 0
	m.state.Items[idx] = it
	m.online = false
	m.lastError = err.Error()
	m.persistLocked()
	m.publishLocked()
	retry := time.Duration(minInt(30, 1<<minInt(it.Retries, 5))) * time.Second
	m.mu.Unlock()
	go func() {
		select {
		case <-time.After(retry):
			m.mu.Lock()
			for i := range m.state.Items {
				if m.state.Items[i].ID == it.ID && m.state.Items[i].Status == "waiting" {
					m.state.Items[i].Status = "queued"
					m.persistLocked()
					m.publishLocked()
				}
			}
			m.mu.Unlock()
			m.signal()
		case <-m.stop:
		}
	}()
}
func (m *Manager) fail(idx int, message string, retry bool) {
	m.mu.Lock()
	if idx < len(m.state.Items) {
		m.state.Items[idx].Status = "error"
		m.state.Items[idx].Error = message
		m.state.Items[idx].SpeedBps = 0
	}
	m.lastError = message
	m.persistLocked()
	m.publishLocked()
	m.mu.Unlock()
}
func (m *Manager) updateItem(idx int, it Item) {
	m.mu.Lock()
	if idx < len(m.state.Items) {
		m.state.Items[idx] = it
	}
	m.persistLocked()
	m.publishLocked()
	m.mu.Unlock()
}
func (m *Manager) item(idx int) Item { m.mu.Lock(); defer m.mu.Unlock(); return m.state.Items[idx] }
func completedBytes(parts []CompletedPart) int64 {
	var n int64
	for _, p := range parts {
		n += p.Size
	}
	return n
}

func (m *Manager) Pause(id string) Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.cancels[id]; c != nil {
		c()
	}
	for i := range m.state.Items {
		if m.state.Items[i].ID == id {
			m.state.Items[i].Status = "paused"
			m.state.Items[i].Uploaded = completedBytes(m.state.Items[i].Parts)
		}
	}
	m.persistLocked()
	s := m.snapshotLocked()
	go m.publish(s)
	return s
}
func (m *Manager) Resume(id string) Snapshot {
	m.mu.Lock()
	for i := range m.state.Items {
		if m.state.Items[i].ID == id && !terminalStatus(m.state.Items[i].Status) {
			m.state.Items[i].Status = "queued"
			m.state.Items[i].Error = ""
		}
	}
	m.persistLocked()
	s := m.snapshotLocked()
	m.mu.Unlock()
	m.signal()
	go m.publish(s)
	return s
}
func (m *Manager) Remove(id string) Snapshot {
	m.mu.Lock()
	if c := m.cancels[id]; c != nil {
		c()
	}
	for i := range m.state.Items {
		if m.state.Items[i].ID == id {
			m.state.Items[i].Status = "removed"
		}
	}
	m.persistLocked()
	s := m.snapshotLocked()
	m.mu.Unlock()
	go m.publish(s)
	return s
}
func (m *Manager) PauseAll() Snapshot {
	m.mu.Lock()
	for id, c := range m.cancels {
		c()
		delete(m.cancels, id)
	}
	for i := range m.state.Items {
		if !terminalStatus(m.state.Items[i].Status) {
			m.state.Items[i].Status = "paused"
			m.state.Items[i].Uploaded = completedBytes(m.state.Items[i].Parts)
		}
	}
	m.persistLocked()
	s := m.snapshotLocked()
	m.mu.Unlock()
	go m.publish(s)
	return s
}
func (m *Manager) ResumeAll() Snapshot {
	m.mu.Lock()
	for i := range m.state.Items {
		if !terminalStatus(m.state.Items[i].Status) {
			m.state.Items[i].Status = "queued"
			m.state.Items[i].Error = ""
		}
	}
	m.persistLocked()
	s := m.snapshotLocked()
	m.mu.Unlock()
	m.signal()
	go m.publish(s)
	return s
}

// ClearCompleted removes local history entries only. Uploaded objects and
// their remote hash manifests remain untouched in Spaces.
func (m *Manager) ClearCompleted() Snapshot {
	m.mu.Lock()
	for i := range m.state.Items {
		if m.state.Items[i].Status == "complete" || m.state.Items[i].Status == "duplicate" {
			m.state.Items[i].Status = "removed"
		}
	}
	m.persistLocked()
	s := m.snapshotLocked()
	m.mu.Unlock()
	go m.publish(s)
	return s
}

func (m *Manager) Settings() Settings {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.state.Settings
	s.SecretKey = ""
	_, err := keyring.Get(keyringService, "spaces-secret")
	s.SecretSet = err == nil
	return s
}

func (m *Manager) APIBaseURL() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state.Settings.APIBaseURL
}
func (m *Manager) SaveSettings(s Settings) error {
	if s.SecretKey != "" {
		if err := keyring.Set(keyringService, "spaces-secret", s.SecretKey); err != nil {
			return err
		}
	}
	s.SecretKey = ""
	s.SecretSet = true
	s.Bucket = strings.ToLower(strings.TrimSpace(s.Bucket))
	if s.PartSizeMB < 5 {
		s.PartSizeMB = 64
	}
	m.mu.Lock()
	m.state.Settings = s
	m.persistLocked()
	snap := m.snapshotLocked()
	m.mu.Unlock()
	go m.publish(snap)
	m.signal()
	return nil
}
func (m *Manager) Snapshot() Snapshot { m.mu.Lock(); defer m.mu.Unlock(); return m.snapshotLocked() }
func terminalStatus(status string) bool {
	return status == "complete" || status == "duplicate" || status == "removed"
}
func (m *Manager) snapshotLocked() Snapshot {
	// Every snapshot gets a sequence number while the manager lock is held.
	// Events are delivered asynchronously by Wails, so the UI uses this value
	// to ignore an older event that arrives after a newer method response.
	m.revision++
	s := Snapshot{Revision: m.revision, Online: m.online, CurrentSpeedBps: m.currentSpeed, AverageSpeedBps: m.averageSpeed, SessionUploaded: m.sessionUploaded, LastError: m.lastError}
	for _, it := range m.state.Items {
		if it.Status == "removed" {
			continue
		}
		s.Items = append(s.Items, it)
		if it.Status == "duplicate" {
			continue
		}
		s.TotalBytes += it.Size
		s.UploadedBytes += it.Uploaded
		if it.Status != "complete" && it.Status != "duplicate" {
			s.PendingBytes += it.Size - it.Uploaded
		}
		if it.Status == "uploading" {
			s.Running = true
		}
	}
	return s
}
func (m *Manager) persistLocked() {
	b, _ := json.MarshalIndent(m.state, "", "  ")
	_ = os.WriteFile(m.statePath, b, 0600)
}
func (m *Manager) publishLocked() {
	if m.emit != nil {
		s := m.snapshotLocked()
		go m.emit(s)
	}
}
func (m *Manager) publish(s Snapshot) {
	if m.emit != nil {
		m.emit(s)
	}
}
func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
