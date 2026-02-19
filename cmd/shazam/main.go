package main

import (
	"flag"
	"fmt"
	"shazam-go/internal/audio"
	"shazam-go/internal/fingerprint"
	"shazam-go/internal/matcher"
)

func main() {
	addFlag := flag.Bool("add", false, "Add a song to the database")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("Usage:")
		fmt.Println("  Add song:    go run cmd/shazam/main.go --add <path_to_wav_file>")
		fmt.Println("  Query song:  go run cmd/shazam/main.go <path_to_wav_file>")
		flag.PrintDefaults()
		return
	}

	filePath := flag.Arg(0)

	// 1. Load WAV
	samples, sampleRate, err := audio.LoadWav(filePath)
	if err != nil {
		fmt.Printf("Error loading WAV: %v\n", err)
		return
	}
	fmt.Printf("Loaded %d samples at %d Hz\n", len(samples), sampleRate)

	// Samples are already normalized and converted to mono in LoadWav
	monoSamples := samples
	// 2. Generate Spectrogram
	spectrogram, err := fingerprint.GenerateSpectogram(monoSamples, sampleRate)
	if err != nil {
		fmt.Printf("Error generating spectrogram: %v\n", err)
		return
	}
	fmt.Printf("Generated spectrogram with %d segments\n", len(spectrogram))

	// 3. Extract Peaks
	peaks, err := fingerprint.ExtractPeaks(spectrogram, sampleRate)
	if err != nil {
		fmt.Printf("Error extracting peaks: %v\n", err)
		return
	}
	fmt.Printf("Extracted %d peaks\n", len(peaks))

	// 4. Generate Hashes
	hashes, err := fingerprint.GenerateHashes(peaks, sampleRate)
	if err != nil {
		fmt.Printf("Error generating hashes: %v\n", err)
		return
	}
	fmt.Printf("Generated %d hashes\n", len(hashes))

	db := matcher.NewDB()

	if *addFlag {
		// Add song to database
		addSong(db, filePath, hashes)
	} else {
		// Query/match song
		result := db.Match(hashes)
		fmt.Printf("Match result: SongID=%d, Confidence=%.2f\n", result.SongID, result.Confidence)
	}
}

func addSong(db *matcher.FingerprintDB, filePath string, hashes map[uint32]float64) {
	fmt.Println("\n=== Adding song to database ===")
	fmt.Printf("File: %s\n", filePath)
	fmt.Printf("Hashes: %d\n", len(hashes))
	
	// Generate a song ID (for now, use a simple hash of the filename)
	songID := generateSongID(filePath)
	
	err := db.RegisterSong(songID, hashes)
	if err != nil {
		fmt.Printf("Error registering song: %v\n", err)
		return
	}
	
	fmt.Printf("✓ Successfully added song with ID: %d\n", songID)
}

func generateSongID(filePath string) int {
	// Simple hash function to generate a song ID from filename
	hash := 0
	for _, char := range filePath {
		hash = hash*31 + int(char)
	}
	if hash < 0 {
		hash = -hash
	}
	return hash
}
