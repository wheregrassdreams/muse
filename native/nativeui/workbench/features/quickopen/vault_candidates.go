package quickopen

import (
	"sync"

	applicationvault "muse/application/vault"
)

// VaultQuickOpen is the narrow application capability Quick Open needs. The
// vault owns candidate construction and matching; this feature adapts its
// ranked value results to the palette's row contract.
type VaultQuickOpen interface {
	QuickOpen(string) []applicationvault.QuickOpenMatch
	Snapshot() applicationvault.Snapshot
}

// VaultCandidates is a live candidate source whose session can be replaced
// when the active vault changes. It retains no NativeUI root or view state.
type VaultCandidates struct {
	mu    sync.RWMutex
	vault VaultQuickOpen
}

func NewVaultCandidates(vault VaultQuickOpen) *VaultCandidates {
	return &VaultCandidates{vault: vault}
}

func (s *VaultCandidates) SetVault(vault VaultQuickOpen) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.vault = vault
	s.mu.Unlock()
}

func (s *VaultCandidates) Rows(query string) Rows {
	vault := s.currentVault()
	if vault == nil {
		return vaultRows(nil)
	}
	snapshot := vault.Snapshot()
	if !snapshot.Ready {
		return vaultRows(nil)
	}
	matches := vault.QuickOpen(query)
	rows := make(vaultRows, 0, len(matches))
	for _, match := range matches {
		rows = append(rows, Row{
			Path: match.Path, Title: match.Title, PathText: match.PathText,
			TitleMatches: append([]int(nil), match.TitleMatches...),
			PathMatches:  append([]int(nil), match.PathMatches...),
		})
	}
	return rows
}

func (s *VaultCandidates) Revision() Revision {
	vault := s.currentVault()
	if vault == nil {
		return Revision{}
	}
	snapshot := vault.Snapshot()
	return Revision{Catalog: snapshot.FilesRevision, Index: snapshot.IndexRevision}
}

func (s *VaultCandidates) currentVault() VaultQuickOpen {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	vault := s.vault
	s.mu.RUnlock()
	return vault
}

type vaultRows []Row

func (rows vaultRows) Len() int     { return len(rows) }
func (rows vaultRows) At(i int) Row { return rows[i] }

var _ CandidateSource = (*VaultCandidates)(nil)
