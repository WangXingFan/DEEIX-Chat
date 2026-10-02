package channel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/security"
)

const (
	maxUserUpstreams       = 32
	maxUserImportedModels  = 100
	maxUserModelNameLength = 256
)

func (s *Service) ownedUpstream(ctx context.Context, userID uint, upstreamID uint) error {
	if userID == 0 {
		return ErrUpstreamNotFound
	}
	item, err := s.repo.GetUpstreamByID(ctx, upstreamID)
	if err != nil {
		return err
	}
	if item.OwnerUserID != userID {
		return ErrUpstreamNotFound
	}
	return nil
}

func (s *Service) ListUserUpstreams(ctx context.Context, userID uint, page int, pageSize int, input ListUpstreamsInput) ([]UpstreamView, int64, error) {
	if userID == 0 {
		return nil, 0, ErrUpstreamNotFound
	}
	input.OwnerUserID = &userID
	return s.ListUpstreams(ctx, page, pageSize, input)
}

func (s *Service) CreateUserUpstream(ctx context.Context, userID uint, input CreateUpstreamInput) (*UpstreamView, error) {
	if userID == 0 {
		return nil, ErrUpstreamNotFound
	}
	var ownerID uint
	if _, total, err := s.ListUserUpstreams(ctx, userID, 1, 1, ListUpstreamsInput{}); err != nil {
		return nil, err
	} else if total >= maxUserUpstreams {
		return nil, ErrUserUpstreamLimit
	}
	if err := security.ValidateOutboundHTTPURL(strings.TrimSpace(input.BaseURL), security.NewStrictOutboundPolicy(true)); err != nil {
		return nil, ErrInvalidUpstreamBaseURL
	}
	ownerID = userID
	input.OwnerUserID = ownerID
	return s.CreateUpstream(ctx, input)
}

func (s *Service) UpdateUserUpstream(ctx context.Context, userID uint, upstreamID uint, input UpdateUpstreamInput) (*UpstreamView, error) {
	if err := s.ownedUpstream(ctx, userID, upstreamID); err != nil {
		return nil, err
	}
	if input.BaseURL != nil {
		if err := security.ValidateOutboundHTTPURL(strings.TrimSpace(*input.BaseURL), security.NewStrictOutboundPolicy(true)); err != nil {
			return nil, ErrInvalidUpstreamBaseURL
		}
	}
	err := s.repo.WithinTransaction(ctx, func(repo repository.ChannelRepository) error {
		upstream, err := repo.GetUpstreamByID(ctx, upstreamID)
		if err != nil {
			return err
		}
		if upstream.OwnerUserID != userID {
			return ErrUpstreamNotFound
		}
		protocolChanged := input.Compatible != nil && normalizeCompatible(*input.Compatible) != upstream.Compatible
		if protocolChanged && input.ProtocolDefaultsJSON == nil {
			input.ProtocolDefaultsJSON = stringPtr(protocolDefaultsForCompatible(*input.Compatible))
		}
		if err := s.saveUpstream(ctx, repo, upstreamID, input); err != nil {
			return err
		}
		if input.Name == nil && !protocolChanged && input.ProtocolDefaultsJSON == nil {
			return nil
		}
		upstream, err = repo.GetUpstreamByID(ctx, upstreamID)
		if err != nil {
			return err
		}
		return refreshUserUpstreamBindings(ctx, repo, upstream, protocolChanged || input.ProtocolDefaultsJSON != nil)
	})
	if err != nil {
		return nil, err
	}
	s.InvalidateModelCatalog()
	view, err := s.getUpstreamView(ctx, upstreamID)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func (s *Service) DeleteUserUpstream(ctx context.Context, userID uint, upstreamID uint) error {
	if err := s.ownedUpstream(ctx, userID, upstreamID); err != nil {
		return err
	}
	err := s.repo.WithinTransaction(ctx, func(repo repository.ChannelRepository) error {
		rows, err := listAllUpstreamModelRows(ctx, repo, upstreamID)
		if err != nil {
			return err
		}
		models := make(map[uint]string)
		for _, row := range rows {
			if row.PlatformModelID == 0 {
				continue
			}
			model, err := repo.GetModelByID(ctx, row.PlatformModelID)
			if err != nil {
				return err
			}
			if model.OwnerUserID != userID {
				return ErrModelAccessDenied
			}
			models[model.ID] = model.PlatformModelName
		}
		if err := repo.DeleteUpstreamCascade(ctx, upstreamID); err != nil {
			return err
		}
		for modelID, name := range models {
			if err := deleteUnreferencedUserModel(ctx, repo, modelID, name); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.localAPIKeyCounters.Delete(upstreamID)
	s.InvalidateModelCatalog()
	return nil
}

func (s *Service) ListUserRemoteModels(ctx context.Context, userID uint, upstreamID uint) (*UpstreamRemoteModelsData, error) {
	if err := s.ownedUpstream(ctx, userID, upstreamID); err != nil {
		return nil, err
	}
	return s.ListRemoteModels(ctx, upstreamID)
}

// DeleteUserUpstreamModel 解除用户私有上游上的单个模型绑定，并在该平台模型不再被任何路由引用时清理它。
func (s *Service) DeleteUserUpstreamModel(ctx context.Context, userID uint, upstreamID uint, routeID uint) error {
	if err := s.ownedUpstream(ctx, userID, upstreamID); err != nil {
		return err
	}
	err := s.repo.WithinTransaction(ctx, func(repo repository.ChannelRepository) error {
		route, err := repo.GetPlatformModelRouteByID(ctx, routeID, upstreamID)
		if err != nil {
			return err
		}
		model, err := repo.GetModelByID(ctx, route.PlatformModelID)
		if err != nil {
			return err
		}
		if model.OwnerUserID != userID {
			return ErrModelNotFound
		}
		if err := repo.DeletePlatformModelRoute(ctx, routeID, upstreamID); err != nil {
			return err
		}
		return deleteUnreferencedUserModel(ctx, repo, model.ID, model.PlatformModelName)
	})
	if err != nil {
		return err
	}
	s.InvalidateModelCatalog()
	return nil
}

func deleteUnreferencedUserModel(ctx context.Context, repo repository.ChannelRepository, modelID uint, name string) error {
	_, total, err := repo.ListModelUpstreamSources(ctx, name, 0, 1)
	if err != nil {
		return err
	}
	if total == 0 {
		return repo.DeleteModelCascade(ctx, modelID)
	}
	return nil
}

func (s *Service) ListUserModels(ctx context.Context, userID uint) ([]ModelView, error) {
	if userID == 0 {
		return []ModelView{}, nil
	}
	const batchSize = 500
	views := make([]ModelView, 0)
	for offset := 0; ; offset += batchSize {
		items, _, err := s.repo.ListModels(ctx, repository.ListChannelModelsInput{
			OwnerUserID: &userID,
			Offset:      offset,
			Limit:       batchSize,
			OnlyActive:  true,
			Sort:        "sortOrder_asc",
		})
		if err != nil {
			return nil, err
		}
		views = append(views, s.filterPublicRoutableModels(items)...)
		if len(items) < batchSize {
			return views, nil
		}
	}
}

func (s *Service) ListUserUpstreamModels(ctx context.Context, userID uint, upstreamID uint) ([]UpstreamModelView, error) {
	if err := s.ownedUpstream(ctx, userID, upstreamID); err != nil {
		return nil, err
	}
	const batchSize = 500
	results := make([]UpstreamModelView, 0)
	for page := 1; ; page++ {
		items, total, err := s.ListUpstreamModels(ctx, upstreamID, page, batchSize, ListUpstreamModelsInput{Sort: "id_asc"})
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.RouteID != 0 {
				results = append(results, item)
			}
		}
		if int64(page*batchSize) >= total {
			return results, nil
		}
	}
}

func userModelPlatformName(userID uint, upstreamID uint, modelName string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s", userID, upstreamID, strings.TrimSpace(modelName))))
	return "user_" + fmt.Sprintf("%d_%d_", userID, upstreamID) + hex.EncodeToString(digest[:])[:32]
}

func (s *Service) UpsertUserModel(ctx context.Context, userID uint, upstreamID uint, modelName string) (*UpstreamModelView, error) {
	if err := s.ownedUpstream(ctx, userID, upstreamID); err != nil {
		return nil, err
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || len(modelName) > maxUserModelNameLength {
		return nil, ErrUpstreamModelNotFound
	}
	upstream, err := s.repo.GetUpstreamByID(ctx, upstreamID)
	if err != nil {
		return nil, err
	}
	kinds := inferKindsJSON(modelName)
	protocols, err := resolveRouteProtocols(nil, upstream.Compatible, upstream.ProtocolDefaultsJSON, kinds)
	if err != nil {
		return nil, err
	}
	displayName := fmt.Sprintf("%s · %s", modelName, upstream.Name)
	platformName := userModelPlatformName(userID, upstreamID, modelName)
	// Re-importing a private model must retain its verified native-search route.
	// Explicit upstream protocol changes are handled by refreshUserUpstreamBindings.
	if existing, findErr := s.repo.GetModelByName(ctx, platformName); findErr == nil {
		if existing.OwnerUserID != userID {
			return nil, ErrModelAccessDenied
		}
		sources, _, sourceErr := s.repo.ListModelUpstreamSources(ctx, platformName, 0, 100)
		if sourceErr != nil {
			return nil, sourceErr
		}
		preserved := make([]string, 0, len(sources))
		for _, source := range sources {
			if source.UpstreamID == upstreamID {
				preserved = append(preserved, source.Protocol)
			}
		}
		if len(preserved) > 0 {
			protocols = preserved
		}
	} else if !errors.Is(findErr, ErrModelNotFound) {
		return nil, findErr
	}
	return s.UpsertUpstreamModel(ctx, upstreamID, UpsertUpstreamModelInput{
		OwnerUserID:       userID,
		DisplayName:       displayName,
		PlatformModelName: platformName,
		UpstreamModelName: modelName,
		Protocols:         protocols,
		KindsJSON:         kinds,
		Source:            stringPtr("user"),
		CatalogSource:     stringPtr("user"),
	})
}

func (s *Service) ImportUserModels(ctx context.Context, userID uint, upstreamID uint, modelNames []string) ([]UpstreamModelView, error) {
	if len(modelNames) == 0 || len(modelNames) > maxUserImportedModels {
		return nil, ErrUserModelLimit
	}
	if err := s.ownedUpstream(ctx, userID, upstreamID); err != nil {
		return nil, err
	}
	result := make([]UpstreamModelView, 0, len(modelNames))
	seen := make(map[string]struct{}, len(modelNames))
	for _, name := range modelNames {
		name = strings.TrimSpace(name)
		if _, exists := seen[name]; exists || name == "" {
			continue
		}
		seen[name] = struct{}{}
		item, err := s.UpsertUserModel(ctx, userID, upstreamID, name)
		if err != nil {
			return nil, err
		}
		result = append(result, *item)
	}
	return result, nil
}

func stringPtr(value string) *string { return &value }

var (
	ErrUserUpstreamLimit = errors.New("user upstream limit exceeded")
	ErrUserModelLimit    = errors.New("user model import limit exceeded")
)
