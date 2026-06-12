package credentials

// Chain resolves a credential by trying several stores in priority order and
// returning the first that has one. It is a fallback chain (dispatch per
// provider), not a merge — distinct from policy.Union, which folds its inputs
// into one merged value.
type Chain struct {
	stores []Lookup
}

// NewChain creates a Chain over the given stores. The last store passed has the
// highest priority.
func NewChain(stores ...Lookup) *Chain {
	return &Chain{stores: stores}
}

// Get returns the credential from the highest-priority store that has it.
func (c *Chain) Get(provider string) Credential {
	cred, _ := c.Resolve(provider)
	return cred
}

// Resolve returns the credential from the highest-priority store that has it,
// along with whether that store reports it as an OAuth access token.
func (c *Chain) Resolve(provider string) (Credential, bool) {
	// Priority is determined by store order: the last one has the highest priority.
	for i := len(c.stores) - 1; i >= 0; i-- {
		if cred, oauth := Resolve(c.stores[i], provider); cred != "" {
			return cred, oauth
		}
	}
	return "", false
}

// Providers returns all providers available across all stores, without duplicates.
func (c *Chain) Providers() []string {
	providerSet := make(map[string]int)
	for _, store := range c.stores {
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
