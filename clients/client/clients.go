package clients

import (
	"sync"
)

type ExcludeClient struct {
	mu         sync.RWMutex
	dictionary map[string]bool
}

var once = sync.Once{}

var clients *ExcludeClient = nil

func GetClients() *ExcludeClient {
	once.Do(func() {
		if clients == nil {
			clients = &ExcludeClient{
				dictionary: make(map[string]bool),
				mu:         sync.RWMutex{},
			}
		}
	})
	return clients
}

func (f *ExcludeClient) ClientExist(domain string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	_, ok := f.dictionary[domain]
	return ok
}

func (f *ExcludeClient) UpdateClients(rows []string) *ExcludeClient {
	dictionary := make(map[string]bool)
	for _, item := range rows {
		dictionary[item] = true
	}

	f.mu.Lock()
	f.dictionary = dictionary
	f.mu.Unlock()
	return f
}
