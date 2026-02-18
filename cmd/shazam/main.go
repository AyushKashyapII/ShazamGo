package main

import (
	"fmt"
	"os"
	"shazam-go/internal/audio"
	"shazam-go/internal/fingerprint"
	"shazam-go/internal/matcher"
)

func main() {
	fmt.Println("Shazam-Go Entry Point")

	if len(os.Args) < 2 {
		fmt.Println("Usage: go run cmd/shazam/main.go <path_to_wav_file>")
		return
	}

	filePath := os.Args[1]

	// 1. Load WAV
	samples, sampleRate, err := audio.LoadWav(filePath)
	if err != nil {
		fmt.Printf("Error loading WAV: %v\n", err)
		return
	}
	fmt.Printf("Loaded %d samples at %d Hz\n", len(samples), sampleRate)
	
	// Debug: check sample range
	if len(samples) > 0 {
		min, max := samples[0], samples[0]
		for _, s := range samples {
			if s < min {
				min = s
			}
			if s > max {
				max = s
			}
		}
		fmt.Printf("Sample range: [%.6f, %.6f]\n", min, max)
	}

	// Samples are already normalized and converted to mono in LoadWav
	monoSamples := samples

	// 3. Generate Spectrogram
	spectrogram, err := fingerprint.GenerateSpectogram(monoSamples, sampleRate)
	if err != nil {
		fmt.Printf("Error generating spectrogram: %v\n", err)
		return
	}
	fmt.Printf("Generated spectrogram with %d segments\n", len(spectrogram))

	// 4. Extract Peaks
	peaks, err := fingerprint.ExtractPeaks(spectrogram, sampleRate)
	if err != nil {
		fmt.Printf("Error extracting peaks: %v\n", err)
		return
	}
	fmt.Printf("Extracted %d peaks\n", len(peaks))

	// 5. Generate Hashes
	hashes, err := fingerprint.GenerateHashes(peaks, sampleRate)
	if err != nil {
		fmt.Printf("Error generating hashes: %v\n", err)
		return
	}
	fmt.Printf("Generated %d hashes\n", len(hashes))

	// 6. Match (Placeholder for now)
	db := matcher.NewDB()
	result := db.Match(hashes)
	fmt.Printf("Match result: SongID=%d, Confidence=%.2f\n", result.SongID, result.Confidence)
}
