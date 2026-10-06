package uploader

import "testing"

func TestObjectKey(t *testing.T) {
	d := Destination{ConferenceTag: "Riga 26", Day: "Day 1", Room: "Main Stage"}
	got := objectKey(d, "A001_C001.MOV")
	want := "riga-26/recordings/raw/day1/main/A001_C001.MOV"
	if got != want {
		t.Fatalf("objectKey = %q, want %q", got, want)
	}
}

func TestCompletedBytes(t *testing.T) {
	got := completedBytes([]CompletedPart{{Size: 10}, {Size: 25}})
	if got != 35 {
		t.Fatalf("completedBytes = %d", got)
	}
}

func TestHackathonObjectKey(t *testing.T) {
	d := Destination{ConferenceTag: "toronto", Day: "Day 1", Room: "Hackathon"}
	got := objectKey(d, "clip.mov")
	if want := "toronto/recordings/raw/day1/hackathon/clip.mov"; got != want {
		t.Fatalf("objectKey = %q, want %q", got, want)
	}
	conference := d
	conference.Room = "one"
	if objectKey(conference, "clip.mov") == got {
		t.Fatal("hackathon and conference destinations collide")
	}
}

func TestAddFilesRequiresDestination(t *testing.T) {
	m := &Manager{}
	if _, err := m.AddFiles([]string{"clip.mov"}, Destination{}); err == nil {
		t.Fatal("expected missing destination error")
	}
	if len(m.state.Items) != 0 {
		t.Fatal("invalid destination changed queue")
	}
}

func TestHashedObjectKey(t *testing.T) {
	got := hashedObjectKey("toronto/recordings/raw/day1/main/clip.mov", "0123456789abcdef")
	want := "toronto/recordings/raw/day1/main/clip--0123456789ab.mov"
	if got != want {
		t.Fatalf("hashedObjectKey = %q, want %q", got, want)
	}
}

func TestManifestKey(t *testing.T) {
	item := Item{ObjectKey: "toronto/recordings/raw/day1/main/clip--0123456789ab.mov", HashSHA256: "0123456789abcdef"}
	got := manifestKey(item)
	want := "toronto/recordings/raw/day1/main/_manifest/sha256/0123456789abcdef.json"
	if got != want {
		t.Fatalf("manifestKey = %q, want %q", got, want)
	}
}
