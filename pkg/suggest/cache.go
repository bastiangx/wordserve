package suggest

import (
	"sync"

	"github.com/charmbracelet/log"
	"github.com/tchap/go-patricia/v2/patricia"
)

type HotCache struct {
	hotWords    map[string]uint16
	hotTrie     *patricia.Trie
	accessTime  map[string]int64
	accessCount int64
	maxWords    int
	mu          sync.RWMutex
}

func NewHotCache(maxWords int) *HotCache {
	return &HotCache{
		hotWords:    make(map[string]uint16, maxWords),
		hotTrie:     patricia.NewTrie(),
		accessTime:  make(map[string]int64, maxWords),
		accessCount: 0,
		maxWords:    maxWords,
	}
}

func (hc *HotCache) Search(lowerPrefix string, minThreshold int) []patricia.Prefix {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	var results []patricia.Prefix

	err := hc.hotTrie.VisitSubtree(patricia.Prefix(lowerPrefix), func(p patricia.Prefix, item patricia.Item) error {
		word := string(p)

		if word == lowerPrefix {
			return nil
		}

		score := item.(int)
		if score < minThreshold {
			return nil
		}

		hc.markAccessed(word)
		results = append(results, p)
		return nil
	})

	if err != nil {
		log.Errorf("Error searching hot cache: %v", err)
	}

	return results
}

func (hc *HotCache) Populate(trie *patricia.Trie) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if trie == nil {
		return
	}

	count := 0
	maxInitial := hc.maxWords / 2

	trie.Visit(func(prefix patricia.Prefix, item patricia.Item) error {
		if count >= maxInitial {
			return nil
		}

		word := internString(string(prefix))
		score := item.(int)

		rank := uint16(65536 - score)

		if len(hc.hotWords) >= hc.maxWords {
			hc.evictLRU()
		}

		hc.hotWords[word] = rank
		hc.hotTrie.Insert(prefix, score)
		hc.accessTime[word] = hc.getNextAccessTime()

		count++
		return nil
	})

	log.Debugf("Populated hot cache with %d words", count)
}

func (hc *HotCache) GetTrie() *patricia.Trie {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.hotTrie
}

func (hc *HotCache) Stats() map[string]int {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	return map[string]int{
		"hotCacheWords": len(hc.hotWords),
		"maxHotWords":   hc.maxWords,
		"hotCacheHits":  int(hc.accessCount),
	}
}

func (hc *HotCache) markAccessed(word string) {
	hc.accessTime[word] = hc.getNextAccessTime()
}

func (hc *HotCache) getNextAccessTime() int64 {
	hc.accessCount++
	return hc.accessCount
}

func (hc *HotCache) evictLRU() {
	var oldestWord string
	var oldestTime int64 = 9223372036854775807

	for word, accessTime := range hc.accessTime {
		if accessTime < oldestTime {
			oldestTime = accessTime
			oldestWord = word
		}
	}

	if oldestWord != "" {
		delete(hc.hotWords, oldestWord)
		delete(hc.accessTime, oldestWord)
		log.Debugf("Evicted word '%s' from hot cache", oldestWord)
	}
}
