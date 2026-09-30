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
	return s.UpdateUpstream(ctx, upstreamID, input)
}

func (s *Service) DeleteUserUpstream(ctx context.Context, userID uint, upstreamID uint) error {
	if err := s.ownedUpstream(ctx, userID, upstreamID); err != nil {
		return err
	}
	rows, _, err := s.repo.ListUpstreamModels(ctx, upstreamID, repository.ListChannelUpstreamModelsInput{Offset: 0, Limit: 5000, Sort: "id_asc"})
	if err != nil {
		return err
	}
	modelIDs := make([]uint, 0, len(rows))
	seen := make(map[uint]struct{}, len(rows))
	for _, row := range rows {
		if row.PlatformModelID == 0 {
			continue
		}
		model, modelErr := s.repo.GetModelByID(ctx, row.PlatformModelID)
		if modelErr != nil || model.OwnerUserID != userID {
			continue
		}
		if _, exists := seen[model.ID]; !exists {
			seen[model.ID] = struct{}{}
			modelIDs = append(modelIDs, model.ID)
		}
	}
	if err := s.DeleteUpstream(ctx, upstreamID); err != nil {
		return err
	}
	for _, modelID := range modelIDs {
		if err := s.repo.DeleteModelCascade(ctx, modelID); err != nil && !errors.Is(err, ErrModelNotFound) {
			return err
		}
	}
	return nil
}

func (s *Service) ListUserRemoteModels(ctx context.Context, userID uint, upstreamID uint) (*UpstreamRemoteModelsData, error) {
	if err := s.ownedUpstream(ctx, userID, upstreamID); err != nil {
		return nil, err
	}
	return s.ListRemoteModels(ctx, upstreamID)
}

func (s *Service) ListUserModels(ctx context.Context, userID uint) ([]ModelView, error) {
	if userID == 0 {
		return []ModelView{}, nil
	}
	items, _, err := s.repo.ListModels(ctx, repository.ListChannelModelsInput{
		OwnerUserID:    &userID,
		OnlyActive:     true,
		OnlyAvailable:  true,
		Sort:           "sortOrder_asc",
	})
	if err != nil {
		return nil, err
	}
	views := make([]ModelView, 0, len(items))
	for _, item := range items {
		views = append(views, s.toModelView(item))
	}
	return views, nil
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
