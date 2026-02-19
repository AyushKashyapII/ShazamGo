package matcher

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const (
	hashesDBFile = "data/hashes.db"
	songsDBFile  = "data/songs.json"
	offsetTolerance = 0.5 // seconds - group offsets within this range
)

type Match struct{
	SongID int
	Timestamp float64
}

type FingerprintDB struct{
	db map[uint32][]Match
	mu sync.RWMutex
	songs map[int]string // songID -> song name
}

func NewDB() *FingerprintDB{
	db := &FingerprintDB{
		db: make(map[uint32][]Match),
		songs: make(map[int]string),
	}
	// Load existing data from files
	if err := db.LoadFromFiles(); err != nil {
		fmt.Printf("Warning: Could not load database files: %v (starting with empty database)\n", err)
	}
	return db
}

func (f *FingerprintDB) RegisterSong(songID int, songName string, hashes map[uint32]float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	// Store song metadata
	f.songs[songID] = songName
	
	// Store hashes in memory
	for hash, timestamp := range hashes {
		match := Match{
			SongID:    songID,
			Timestamp: timestamp,
		}
		f.db[hash] = append(f.db[hash], match)
	}
	
	// Save to files
	if err := f.saveSongMetadata(songID, songName); err != nil {
		return fmt.Errorf("failed to save song metadata: %v", err)
	}
	if err := f.appendHashesToFile(songID, hashes); err != nil {
		return fmt.Errorf("failed to save hashes: %v", err)
	}
	
	return nil
}

func (f *FingerprintDB) GetSongName(songID int) string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	// Normalize to positive ID for lookup
	positiveID := normalizeSongID(songID)
	return f.songs[positiveID]
}

// normalizeSongID converts negative IDs to positive by taking absolute value
func normalizeSongID(songID int) int {
	if songID < 0 {
		return -songID
	}
	if songID == 0 {
		return 1 // Ensure non-zero
	}
	return songID
}

// GetStats returns database statistics for debugging
func (f *FingerprintDB) GetStats() (totalHashes int, totalMatches int) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	totalHashes = len(f.db)
	for _, matches := range f.db {
		totalMatches += len(matches)
	}
	return totalHashes, totalMatches
}

// GetMatchesForHash returns all matches for a given hash (for debugging)
func (f *FingerprintDB) GetMatchesForHash(hash uint32) []Match {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.db[hash]
}

type MatchResult struct{
	SongID int
	Confidence float64
	SongName string
	MatchCount int
	TotalHashes int
}

// Match finds the best matching song for the given query hashes
func (f *FingerprintDB) Match(queryHashes map[uint32]float64) MatchResult {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	fmt.Println("matcher: Matching fingerprints against database...")
	
	if len(queryHashes) == 0 {
		return MatchResult{SongID: -1, Confidence: 0.0, MatchCount: 0, TotalHashes: 0}
	}
	
	if len(f.db) == 0 {
		fmt.Println("matcher: Database is empty")
		return MatchResult{SongID: -1, Confidence: 0.0, MatchCount: 0, TotalHashes: len(queryHashes)}
	}
	
	// Track matches: (songID, offsetBucket) -> count
	// timeOffset = queryTime - dbTime (how much earlier/later the query is)
	// We bucket offsets to handle small timing variations
	type offsetKey struct {
		songID int
		offsetBucket int // offset rounded to nearest tolerance
	}
	offsetMatches := make(map[offsetKey]int)
	
	// For each query hash, find matches in database
	for queryHash, queryTime := range queryHashes {
		dbMatches := f.db[queryHash]
		
		// For each database match, calculate time offset and bucket it
		for _, dbMatch := range dbMatches {
			offset := queryTime - dbMatch.Timestamp
			// Round offset to nearest bucket (e.g., 0.5s buckets)
			offsetBucket := int(offset / offsetTolerance)
			
			key := offsetKey{
				songID: dbMatch.SongID,
				offsetBucket: offsetBucket,
			}
			offsetMatches[key]++
		}
	}
	
	if len(offsetMatches) == 0 {
		fmt.Println("matcher: No matching hashes found")
		return MatchResult{SongID: -1, Confidence: 0.0, MatchCount: 0, TotalHashes: len(queryHashes)}
	}
	
	// Find the (songID, offsetBucket) with most matches
	bestKey := offsetKey{songID: -1, offsetBucket: 0}
	bestCount := 0
	
	for key, count := range offsetMatches {
		if count > bestCount {
			bestCount = count
			bestKey = key
		}
	}
	
	// Calculate confidence: matches / total query hashes
	confidence := float64(bestCount) / float64(len(queryHashes))
	
	// Get song name (normalize ID to positive for lookup)
	positiveID := normalizeSongID(bestKey.songID)
	songName := f.songs[positiveID]
	if songName == "" {
		songName = "Unknown"
	}
	
	fmt.Printf("matcher: Best match - SongID: %d, Matches: %d/%d, Confidence: %.2f%%\n",
		positiveID, bestCount, len(queryHashes), confidence*100)
	
	return MatchResult{
		SongID:     positiveID,
		Confidence: confidence,
		SongName:   songName,
		MatchCount: bestCount,
		TotalHashes: len(queryHashes),
	}
}

// LoadFromFiles loads database from disk
func (f *FingerprintDB) LoadFromFiles() error {
	if err := f.loadSongsFromFile(); err != nil {
		return fmt.Errorf("failed to load songs: %v", err)
	}
	if err := f.loadHashesFromFile(); err != nil {
		return fmt.Errorf("failed to load hashes: %v", err)
	}
	return nil
}

// saveSongMetadata saves song metadata to JSON file
func (f *FingerprintDB) saveSongMetadata(songID int, songName string) error {
	// Ensure data directory exists
	dir := filepath.Dir(songsDBFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	
	// Load existing songs (JSON keys are strings)
	songsStr := make(map[string]string)
	if data, err := os.ReadFile(songsDBFile); err == nil {
		json.Unmarshal(data, &songsStr)
	}
	
	// Convert to int map for internal use
	songs := make(map[int]string)
	for k, v := range songsStr {
		var id int
		fmt.Sscanf(k, "%d", &id)
		songs[id] = v
	}
	
	// Add/update song (ensure positive ID)
	positiveID := normalizeSongID(songID)
	songs[positiveID] = songName
	
	// Convert back to string keys for JSON
	songsStr = make(map[string]string)
	for k, v := range songs {
		songsStr[fmt.Sprintf("%d", k)] = v
	}
	
	// Save to file
	data, err := json.MarshalIndent(songsStr, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(songsDBFile, data, 0644)
}

// loadSongsFromFile loads song metadata from JSON file
func (f *FingerprintDB) loadSongsFromFile() error {
	data, err := os.ReadFile(songsDBFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil 
		}
		return err
	}
	// JSON keys are strings, so unmarshal to string map first
	songsStr := make(map[string]string)
	if err := json.Unmarshal(data, &songsStr); err != nil {
		return err
	}
	// Convert string keys to int keys
	for k, v := range songsStr {
		var id int
		fmt.Sscanf(k, "%d", &id)
		// Normalize to positive ID
		positiveID := normalizeSongID(id)
		f.songs[positiveID] = v
	}
	return nil
}

// appendHashesToFile appends hashes to binary file
func (f *FingerprintDB) appendHashesToFile(songID int, hashes map[uint32]float64) error {
	// Ensure data directory exists
	dir := filepath.Dir(hashesDBFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	
	file, err := os.OpenFile(hashesDBFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	
	// Normalize songID to positive before storing
	positiveID := normalizeSongID(songID)
	
	// Write each hash entry
	for hash, timestamp := range hashes {
		// Format: hash (4 bytes) + songID (4 bytes) + timestamp (8 bytes)
		if err := binary.Write(file, binary.LittleEndian, hash); err != nil {
			return err
		}
		if err := binary.Write(file, binary.LittleEndian, int32(positiveID)); err != nil {
			return err
		}
		if err := binary.Write(file, binary.LittleEndian, timestamp); err != nil {
			return err
		}
	}
	
	return nil
}

// loadHashesFromFile loads all hashes from binary file
func (f *FingerprintDB) loadHashesFromFile() error {
	file, err := os.Open(hashesDBFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, that's okay
		}
		return err
	}
	defer file.Close()
	
	// Read entries until EOF
	for {
		var hash uint32
		var songID int32
		var timestamp float64
		
		// Try to read hash
		if err := binary.Read(file, binary.LittleEndian, &hash); err != nil {
			if err == io.EOF {
				break // End of file
			}
			return err
		}
		
		// Read songID
		if err := binary.Read(file, binary.LittleEndian, &songID); err != nil {
			return err
		}
		
		// Read timestamp
		if err := binary.Read(file, binary.LittleEndian, &timestamp); err != nil {
			return err
		}
		
		// Normalize songID to positive
		positiveID := normalizeSongID(int(songID))
		
		// Store in memory
		match := Match{
			SongID:    positiveID,
			Timestamp: timestamp,
		}
		f.db[hash] = append(f.db[hash], match)
	}
	
	return nil
}
