package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"shazam-go/internal/audio"
	"shazam-go/internal/fingerprint"
	"shazam-go/internal/matcher"
)

var db *matcher.FingerprintDB

type matchResponse struct {
	Success     bool    `json:"success"`
	Message     string  `json:"message"`
	SongID      int     `json:"songId,omitempty"`
	SongName    string  `json:"songName,omitempty"`
	Confidence  float64 `json:"confidence,omitempty"`
	MatchCount  int     `json:"matchCount,omitempty"`
	TotalHashes int     `json:"totalHashes,omitempty"`
}

type addResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	SongID  int    `json:"songId,omitempty"`
	SongName string `json:"songName,omitempty"`
}

func main() {
	fmt.Println("Starting Shazam-Go HTTP server on :8080")
	db = matcher.NewDB()

	http.HandleFunc("/api/match", handleMatch)
	http.HandleFunc("/api/add", handleAdd)

	// Serve static frontend from ./web
	fs := http.FileServer(http.Dir("web"))
	http.Handle("/", fs)

	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("server error: %v\n", err)
	}
}

func handleAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeAddError(w, fmt.Sprintf("failed to read file: %v", err))
		return
	}
	defer file.Close()

	// Save to temp file
	tmpDir := os.TempDir()
	tmpPath := filepath.Join(tmpDir, fmt.Sprintf("upload-%d-%s", time.Now().UnixNano(), filepath.Base(header.Filename)))
	out, err := os.Create(tmpPath)
	if err != nil {
		writeAddError(w, fmt.Sprintf("failed to create temp file: %v", err))
		return
	}
	_, err = io.Copy(out, file)
	out.Close()
	if err != nil {
		writeAddError(w, fmt.Sprintf("failed to save temp file: %v", err))
		return
	}
	defer os.Remove(tmpPath)

	// Process audio
	samples, sampleRate, err := audio.LoadWav(tmpPath)
	if err != nil {
		writeAddError(w, fmt.Sprintf("failed to load WAV: %v", err))
		return
	}

	monoSamples := samples
	spectrogram, err := fingerprint.GenerateSpectogram(monoSamples, sampleRate)
	if err != nil {
		writeAddError(w, fmt.Sprintf("failed to generate spectrogram: %v", err))
		return
	}

	peaks, err := fingerprint.ExtractPeaks(spectrogram, sampleRate)
	if err != nil {
		writeAddError(w, fmt.Sprintf("failed to extract peaks: %v", err))
		return
	}

	hashes, err := fingerprint.GenerateHashes(peaks, sampleRate)
	if err != nil {
		writeAddError(w, fmt.Sprintf("failed to generate hashes: %v", err))
		return
	}

	if len(hashes) == 0 {
		writeAddError(w, "no hashes generated from audio (audio may be silent)")
		return
	}

	songName := filepath.Base(header.Filename)
	songID := generateSongID(songName)

	if err := db.RegisterSong(songID, songName, hashes); err != nil {
		writeAddError(w, fmt.Sprintf("failed to register song: %v", err))
		return
	}

	resp := addResponse{
		Success:  true,
		Message:  "song added successfully",
		SongID:   songID,
		SongName: songName,
	}
	writeJSON(w, resp)
}

func handleMatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeMatchError(w, fmt.Sprintf("failed to read file: %v", err))
		return
	}
	defer file.Close()

	tmpDir := os.TempDir()
	tmpPath := filepath.Join(tmpDir, fmt.Sprintf("upload-%d-%s", time.Now().UnixNano(), filepath.Base(header.Filename)))
	out, err := os.Create(tmpPath)
	if err != nil {
		writeMatchError(w, fmt.Sprintf("failed to create temp file: %v", err))
		return
	}
	_, err = io.Copy(out, file)
	out.Close()
	if err != nil {
		writeMatchError(w, fmt.Sprintf("failed to save temp file: %v", err))
		return
	}
	defer os.Remove(tmpPath)

	samples, sampleRate, err := audio.LoadWav(tmpPath)
	if err != nil {
		writeMatchError(w, fmt.Sprintf("failed to load WAV: %v", err))
		return
	}

	monoSamples := samples
	spectrogram, err := fingerprint.GenerateSpectogram(monoSamples, sampleRate)
	if err != nil {
		writeMatchError(w, fmt.Sprintf("failed to generate spectrogram: %v", err))
		return
	}

	peaks, err := fingerprint.ExtractPeaks(spectrogram, sampleRate)
	if err != nil {
		writeMatchError(w, fmt.Sprintf("failed to extract peaks: %v", err))
		return
	}

	hashes, err := fingerprint.GenerateHashes(peaks, sampleRate)
	if err != nil {
		writeMatchError(w, fmt.Sprintf("failed to generate hashes: %v", err))
		return
	}

	if len(hashes) == 0 {
		writeMatchError(w, "no hashes generated from audio (audio may be silent)")
		return
	}

	result := db.Match(hashes)
	if result.SongID == -1 {
		resp := matchResponse{
			Success:     false,
			Message:     "no match found",
			SongID:      -1,
			Confidence:  result.Confidence,
			MatchCount:  result.MatchCount,
			TotalHashes: result.TotalHashes,
		}
		writeJSON(w, resp)
		return
	}

	resp := matchResponse{
		Success:     true,
		Message:     "match found",
		SongID:      result.SongID,
		SongName:    result.SongName,
		Confidence:  result.Confidence,
		MatchCount:  result.MatchCount,
		TotalHashes: result.TotalHashes,
	}
	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func writeMatchError(w http.ResponseWriter, msg string) {
	resp := matchResponse{
		Success: false,
		Message: msg,
	}
	writeJSON(w, resp)
}

func writeAddError(w http.ResponseWriter, msg string) {
	resp := addResponse{
		Success: false,
		Message: msg,
	}
	writeJSON(w, resp)
}

// generateSongID generates a stable positive song ID from a filename
// Same logic as in cmd/shazam/main.go to keep IDs consistent
func generateSongID(filePath string) int {
	var hash uint64 = 0
	for _, char := range filePath {
		hash = hash*31 + uint64(char)
	}
	result := int(hash % 2147483647)
	if result == 0 {
		result = 1
	}
	return result
}



