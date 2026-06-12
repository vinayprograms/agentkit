package credentials

// UnionStore is a credential store that merges multiple underlying stores, in order of priority.
type UnionStore struct {
	stores []Lookup
}

func NewUnionStore(stores ...Lookup) *UnionStore {
	return &UnionStore{stores: stores}
}

// Return the credential from the highest-priority store that has it.
func (u *UnionStore) Get(provider string) Credential {
	cred, _ := u.Resolve(provider)
	return cred
}

// Resolve returns the credential from the highest-priority store that has it,
// along with whether that store reports it as an OAuth access token.
func (u *UnionStore) Resolve(provider string) (Credential, bool) {
	// Priority is determined by the order of stores in the UnionStore (last one has the highest priority).
	for i := len(u.stores) - 1; i >= 0; i-- {
		if cred, oauth := Resolve(u.stores[i], provider); cred != "" {
			return cred, oauth
		}
	}
	return "", false
}

// Return a list of all providers available across all stores, without duplicates.
func (u *UnionStore) Providers() []string {
	providerSet := make(map[string]int)
	for _, store := range u.stores {
		for _, provider := range store.Providers() {
			providerSet[provider]++
		}
	}

	var providers []string
	for provider := range providerSet {
		providers = append(providers, provider)
	}
	return providers
}
