package edgar

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// TickerCache maps stock tickers to CIK numbers.
type TickerCache struct {
	mu      sync.RWMutex
	entries map[string]TickerEntry // ticker -> entry
	all     []TickerEntry
	loaded  bool
}

func NewTickerCache() *TickerCache {
	return &TickerCache{
		entries: make(map[string]TickerEntry),
	}
}

// GetCIK returns the CIK for a given ticker symbol.
func (tc *TickerCache) GetCIK(c *Client, ticker string) (int, error) {
	if err := tc.ensureLoaded(c); err != nil {
		return 0, err
	}

	tc.mu.RLock()
	defer tc.mu.RUnlock()

	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	entry, ok := tc.entries[ticker]
	if !ok {
		return 0, fmt.Errorf("ticker %q not found", ticker)
	}
	return entry.CIK, nil
}

// Search finds companies matching a query string in name or ticker.
func (tc *TickerCache) Search(c *Client, query string, limit int) ([]TickerEntry, error) {
	if err := tc.ensureLoaded(c); err != nil {
		return nil, err
	}

	tc.mu.RLock()
	defer tc.mu.RUnlock()

	query = strings.ToUpper(strings.TrimSpace(query))
	var results []TickerEntry

	// Exact ticker match first.
	if entry, ok := tc.entries[query]; ok {
		results = append(results, entry)
	}

	// Then search by name.
	for _, entry := range tc.all {
		if len(results) >= limit {
			break
		}
		if strings.Contains(strings.ToUpper(entry.Name), query) || strings.Contains(strings.ToUpper(entry.Ticker), query) {
			// Avoid duplicates from exact match.
			dup := false
			for _, r := range results {
				if r.CIK == entry.CIK {
					dup = true
					break
				}
			}
			if !dup {
				results = append(results, entry)
			}
		}
	}

	return results, nil
}

func (tc *TickerCache) ensureLoaded(c *Client) error {
	tc.mu.RLock()
	if tc.loaded {
		tc.mu.RUnlock()
		return nil
	}
	tc.mu.RUnlock()

	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.loaded {
		return nil
	}

	data, err := c.get(tickerMapURL)
	if err != nil {
		return fmt.Errorf("failed to fetch ticker mapping: %w", err)
	}

	// The response is {"fields": [...], "data": [[cik, name, ticker, exchange], ...]}
	var raw struct {
		Fields []string        `json:"fields"`
		Data   [][]interface{} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to parse ticker mapping: %w", err)
	}

	for _, row := range raw.Data {
		if len(row) < 4 {
			continue
		}
		cik := 0
		switch v := row[0].(type) {
		case float64:
			cik = int(v)
		}

		name, _ := row[1].(string)
		ticker, _ := row[2].(string)
		exchange, _ := row[3].(string)

		entry := TickerEntry{
			CIK:      cik,
			Name:     name,
			Ticker:   strings.ToUpper(ticker),
			Exchange: exchange,
		}
		tc.entries[entry.Ticker] = entry
		tc.all = append(tc.all, entry)
	}

	tc.loaded = true
	return nil
}
