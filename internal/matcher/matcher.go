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
	return f.songs[songID]
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
}

func (f *FingerprintDB) Match(queryHashes map[uint32]float64) MatchResult{
	fmt.Println("matcher: Matching fingerprints against database...")
	return MatchResult{SongID:-1,Confidence:0.0}
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
	
	// Load existing songs
	songs := make(map[int]string)
	if data, err := os.ReadFile(songsDBFile); err == nil {
		json.Unmarshal(data, &songs)
	}
	
	// Add/update song
	songs[songID] = songName
	
	// Save to file
	data, err := json.MarshalIndent(songs, "", "  ")
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
			return nil // File doesn't exist yet, that's okay
		}
		return err
	}
	return json.Unmarshal(data, &f.songs)
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
	
	// Write each hash entry
	for hash, timestamp := range hashes {
		// Format: hash (4 bytes) + songID (4 bytes) + timestamp (8 bytes)
		if err := binary.Write(file, binary.LittleEndian, hash); err != nil {
			return err
		}
		if err := binary.Write(file, binary.LittleEndian, int32(songID)); err != nil {
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
		
		// Store in memory
		match := Match{
			SongID:    int(songID),
			Timestamp: timestamp,
		}
		f.db[hash] = append(f.db[hash], match)
	}
	
	return nil
}
