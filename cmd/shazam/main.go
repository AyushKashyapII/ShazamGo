package main

import (
	"flag"
	"fmt"
	"path/filepath"
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
		fmt.Println("\n=== Match Result ===")
		if result.SongID != -1 {
			fmt.Printf("✓ Match found!\n")
			fmt.Printf("  Song ID: %d\n", result.SongID)
			fmt.Printf("  Song Name: %s\n", result.SongName)
			fmt.Printf("  Matches: %d/%d hashes\n", result.MatchCount, result.TotalHashes)
			fmt.Printf("  Confidence: %.2f%%\n", result.Confidence*100)
		} else {
			fmt.Printf("✗ No match found\n")
			fmt.Printf("  Confidence: %.2f%%\n", result.Confidence*100)
		}
	}
}

type SongInfo struct{
	SongID int
	Title string
}

func addSong(db *matcher.FingerprintDB, filePath string, hashes map[uint32]float64) {
	fmt.Println("\n=== Adding song to database ===")
	fmt.Printf("File: %s\n", filePath)
	fmt.Printf("Hashes: %d\n", len(hashes))
	
	// Generate a song ID (for now, use a simple hash of the filename)
	songID := generateSongID(filePath)
	
	// Extract just the filename for storage
	songName := filepath.Base(filePath)
	
	err := db.RegisterSong(songID, songName, hashes)
	if err != nil {
		fmt.Printf("Error registering song: %v\n", err)
		return
	}
	
	// Show database stats
	totalHashes, totalMatches := db.GetStats()
	fmt.Printf("✓ Successfully added song with ID: %d\n", songID)
	fmt.Printf("✓ Song name: %s\n", songName)
	fmt.Printf("Database stats: %d unique hashes, %d total matches\n", totalHashes, totalMatches)
	fmt.Printf("✓ Data saved to disk (data/hashes.db and data/songs.json)\n")
}



func generateSongID(filePath string) int {
	// Simple hash function to generate a song ID from filename
	// Use uint64 to avoid overflow, then convert to positive int
	var hash uint64 = 0
	for _, char := range filePath {
		hash = hash*31 + uint64(char)
	}
	// Convert to int and ensure positive (mod by max int32 to keep it reasonable)
	// Use 2147483647 (max int32) as modulus to ensure positive result
	result := int(hash % 2147483647)
	if result == 0 {
		result = 1 // Ensure non-zero
	}
	return result
}
