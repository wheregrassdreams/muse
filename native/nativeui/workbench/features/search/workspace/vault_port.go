package workspace

import (
	applicationsearch "muse/application/search"
	applicationvault "muse/application/vault"
)

// VaultSearchPort adapts vault index/query capabilities to the search Host.
// It carries no application shell or presentation state.
type VaultSearchPort struct {
	Session *applicationvault.Session
}

func NewVaultSearchPort(session *applicationvault.Session) VaultSearchPort {
	return VaultSearchPort{Session: session}
}

func (p VaultSearchPort) SearchState() applicationsearch.SearchState {
	if p.Session == nil {
		return applicationsearch.SearchState{}
	}
	snapshot := p.Session.Snapshot().Search
	return applicationsearch.SearchState{
		Token: snapshot.Token, Query: snapshot.Query, Literal: snapshot.Literal,
		Hits: snapshot.Hits, Busy: snapshot.Busy, Cost: snapshot.Cost,
	}
}

func (p VaultSearchPort) Query(input applicationsearch.QueryInput) (uint64, error) {
	if p.Session == nil {
		return 0, applicationvault.ErrSessionClosed
	}
	return p.Session.Query(input.Text, input.Literal, input.ActivePath, input.ActiveSource)
}

func (p VaultSearchPort) ResolveHitPath(path string) (string, error) {
	if p.Session == nil {
		return "", applicationvault.ErrSessionNotReady
	}
	return p.Session.ResolveSearchHitPath(path)
}

func (p VaultSearchPort) Refresh(path string) error {
	if p.Session == nil {
		return applicationvault.ErrSessionClosed
	}
	return p.Session.Refresh(path)
}
