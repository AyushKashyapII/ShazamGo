package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	// Convert to WAV if needed
	fmt.Printf("Converting audio file: %s\n", tmpPath)
	wavPath, err := convertToWav(tmpPath)
	if err != nil {
		fmt.Printf("Conversion error: %v\n", err)
		writeAddError(w, fmt.Sprintf("failed to convert audio to WAV: %v", err))
		return
	}
	fmt.Printf("Conversion successful, WAV file: %s\n", wavPath)
	if wavPath != tmpPath {
		defer os.Remove(wavPath)
	}

	// Process audio
	samples, sampleRate, err := audio.LoadWav(wavPath)
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

	// Convert to WAV if needed
	fmt.Printf("Converting audio file for matching: %s\n", tmpPath)
	wavPath, err := convertToWav(tmpPath)
	if err != nil {
		fmt.Printf("Conversion error: %v\n", err)
		writeMatchError(w, fmt.Sprintf("failed to convert audio to WAV: %v", err))
		return
	}
	fmt.Printf("Conversion successful, WAV file: %s\n", wavPath)
	if wavPath != tmpPath {
		defer os.Remove(wavPath)
	}

	samples, sampleRate, err := audio.LoadWav(wavPath)
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

// detectAudioFormat detects the actual audio format by reading file header
func detectAudioFormat(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		return "unknown"
	}
	defer file.Close()
	
	header := make([]byte, 12)
	if n, _ := file.Read(header); n < 12 {
		return "unknown"
	}
	
	// Check for WAV (RIFF...WAVE)
	if string(header[0:4]) == "RIFF" && string(header[8:12]) == "WAVE" {
		return "wav"
	}
	// Check for WebM (starts with 0x1A 0x45 0xDF 0xA3)
	if header[0] == 0x1A && header[1] == 0x45 && header[2] == 0xDF && header[3] == 0xA3 {
		return "webm"
	}
	// Check for MP4/M4A (ftyp box)
	if string(header[4:8]) == "ftyp" {
		return "mp4"
	}
	// Check for OGG
	if string(header[0:4]) == "OggS" {
		return "ogg"
	}
	
	return "unknown"
}

// convertToWav converts an audio file to WAV format
// Returns the WAV file path (may be same as input if already WAV)
// Uses FFmpeg if available, otherwise tries to load directly as WAV
func convertToWav(inputPath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(inputPath))
	
	// If already WAV, try to load directly
	if ext == ".wav" {
		// Verify it's actually a valid WAV file
		if _, _, err := audio.LoadWav(inputPath); err == nil {
			return inputPath, nil
		} else {
			// If loading failed, detect actual format
			actualFormat := detectAudioFormat(inputPath)
			if actualFormat != "wav" {
				fmt.Printf("File has .wav extension but is actually %s format\n", actualFormat)
				ext = "." + actualFormat
			} else {
				fmt.Printf("Warning: WAV file failed to load, attempting conversion: %v\n", err)
			}
		}
	}
	
	// If no extension or unknown format, try to load as WAV first
	if ext == "" || ext == ".tmp" || ext == ".unknown" {
		fmt.Printf("Trying to detect format and load as WAV\n")
		if _, _, err := audio.LoadWav(inputPath); err == nil {
			return inputPath, nil
		}
		// Detect actual format
		actualFormat := detectAudioFormat(inputPath)
		if actualFormat != "unknown" && actualFormat != "wav" {
			ext = "." + actualFormat
			fmt.Printf("Detected format: %s\n", actualFormat)
		}
	}
	
	// Check if FFmpeg is available
	if !isFFmpegAvailable() {
		if ext == ".wav" || ext == "" {
			// File claims to be WAV but isn't, or no extension
			actualFormat := detectAudioFormat(inputPath)
			if actualFormat != "wav" && actualFormat != "unknown" {
				return "", fmt.Errorf("file is %s format (not WAV) and requires FFmpeg for conversion. FFmpeg is not installed. Please install FFmpeg from https://ffmpeg.org/download.html", actualFormat)
			}
			return "", fmt.Errorf("file appears to be WAV but failed to load. FFmpeg is not installed. Please install FFmpeg from https://ffmpeg.org/download.html or ensure the file is a valid WAV file")
		}
		return "", fmt.Errorf("audio format '%s' requires FFmpeg for conversion, but FFmpeg is not installed. Please install FFmpeg from https://ffmpeg.org/download.html or use WAV files", ext)
	}
	
	// Convert using FFmpeg
	outputPath := inputPath + ".wav"
	cmd := exec.Command("ffmpeg", "-i", inputPath, "-acodec", "pcm_s16le", "-ar", "44100", "-ac", "1", "-y", outputPath)
	
	// Capture stderr to see FFmpeg errors
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = nil
	
	if err := cmd.Run(); err != nil {
		// Include FFmpeg error message in our error
		ffmpegError := strings.TrimSpace(stderr.String())
		if ffmpegError != "" {
			return "", fmt.Errorf("FFmpeg conversion failed: %v\nFFmpeg output: %s", err, ffmpegError)
		}
		return "", fmt.Errorf("FFmpeg conversion failed: %v", err)
	}
	
	// Verify output file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		return "", fmt.Errorf("FFmpeg conversion completed but output file was not created: %s", outputPath)
	}
	
	return outputPath, nil
}

// isFFmpegAvailable checks if FFmpeg is installed and available
func isFFmpegAvailable() bool {
	cmd := exec.Command("ffmpeg", "-version")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}



