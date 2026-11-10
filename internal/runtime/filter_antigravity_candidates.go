package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"quota-activation/internal/config"
	"quota-activation/internal/detector"
	"quota-activation/internal/host"
)

// filterAntigravityCandidates 根据宿主已注册凭证模型过滤 Antigravity 候选集。
// 仅保留明确注册到相同 auth_index 与 provider 的候选模型。
func filterAntigravityCandidates(
	ctx context.Context,
	client host.Client,
	file host.AuthFile,
	candidates []autoCandidate,
	mappings ...config.OAuthModelAliases,
) ([]autoCandidate, error) {
	authIndex := strings.TrimSpace(file.AuthIndex)
	fileID := strings.TrimSpace(firstNonBlankAuto(file.ID, file.Name))
	if authIndex == "" && fileID == "" {
		return nil, errors.New("missing auth index")
	}

	document := decodeAutoAuthFile(file)
	providerRaw := firstNonBlankAuto(file.Provider, file.Type, document.Provider, document.Type)
	provider, ok := autoProvider(providerRaw)
	if !ok || provider != detector.ProviderAntigravity {
		return nil, fmt.Errorf("unsupported file provider %q: expected antigravity", providerRaw)
	}

	for _, candidate := range candidates {
		if candidate.provider != detector.ProviderAntigravity {
			return nil, fmt.Errorf("candidate provider %q mismatch: expected antigravity", candidate.provider)
		}
	}

	credentials, err := host.GetAuthModels(ctx, client, authIndex)
	if err != nil {
		return nil, fmt.Errorf("get auth models for index %q: %w", authIndex, err)
	}
	if len(credentials) == 0 {
		return nil, fmt.Errorf("no credentials returned for auth index %q", authIndex)
	}

	var matched *host.HostAuthModels
	for i := range credentials {
		credIndex := strings.TrimSpace(credentials[i].AuthIndex)
		credID := strings.TrimSpace(credentials[i].ID)
		if (authIndex != "" && credIndex == authIndex) || (authIndex == "" && credID == fileID) {
			if matched != nil {
				return nil, errors.New("ambiguous credentials")
			}
			matched = &credentials[i]
		}
	}
	if matched == nil {
		return nil, fmt.Errorf("credential not found for auth index %q", authIndex)
	}

	credProvider, ok := autoProvider(matched.Provider)
	if !ok || credProvider != detector.ProviderAntigravity {
		return nil, fmt.Errorf("credential provider mismatch: got %q, expected antigravity", matched.Provider)
	}

	var aliases config.OAuthModelAliases
	if len(mappings) > 0 {
		aliases = mappings[0]
	}

	filtered := make([]autoCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		_, ok, err := aliases.RegisteredModel(config.ModelRegistrationRequest{Provider: string(candidate.provider), Model: candidate.model, Registered: matched.Models})
		if err != nil {
			return nil, err
		}
		if ok {
			filtered = append(filtered, candidate)
		}
	}

	return filtered, nil
}
