package matcher

import (
	"fmt"
	"sync"
)

type Match struct{
	SongID int
	Timestamp float64
}

type FingerprintDB struct{
	db map[uint32][]Match
	mu sync.RWMutex
}

func NewDB() *FingerprintDB{
	return &FingerprintDB{
		db:make(map[uint32][]Match),
	}
}

// func (f *FingerprintDB) RegisterSong(songID int,hashes map[uint32]float64){
// 	f.mu.Lock()
// 	defer f.mu.Unlock()
// 	f.db[hash]=append(f.db[hash])
// }

type MatchResult struct{
	SongID int
	Confidence float64
}

func (f *FingerprintDB) Match(queryHashes map[uint32]float64) MatchResult{


	fmt.Println("matcher: Matching fingerprints against database...")
	return MatchResult{SongID:-1,Confidence:0.0}
}
