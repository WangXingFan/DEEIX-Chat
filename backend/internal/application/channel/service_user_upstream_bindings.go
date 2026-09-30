package channel

import (
	"context"
	"fmt"

	domainchannel "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/channel"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
)

func listAllUpstreamModelRows(ctx context.Context, repo repository.ChannelRepository, upstreamID uint) ([]repository.ChannelUpstreamModelListRow, error) {
	const batchSize = 500
	var results []repository.ChannelUpstreamModelListRow
	for offset := 0; ; offset += batchSize {
		rows, _, err := repo.ListUpstreamModels(ctx, upstreamID, repository.ListChannelUpstreamModelsInput{
			Offset: offset, Limit: batchSize, Sort: "id_asc",
		})
		if err != nil {
			return nil, err
		}
		results = append(results, rows...)
		if len(rows) < batchSize {
			return results, nil
		}
	}
}

// refreshUserUpstreamBindings 在更新上游的事务中同步展示名称和已有路由协议。
func refreshUserUpstreamBindings(ctx context.Context, repo repository.ChannelRepository, upstream *domainchannel.Upstream, updateProtocols bool) error {
	rows, err := listAllUpstreamModelRows(ctx, repo, upstream.ID)
	if err != nil {
		return err
	}
	seen := make(map[uint]struct{})
	for _, row := range rows {
		if row.PlatformModelID == 0 {
			continue
		}
		if _, exists := seen[row.PlatformModelID]; exists {
			continue
		}
		seen[row.PlatformModelID] = struct{}{}
		model, err := repo.GetModelByID(ctx, row.PlatformModelID)
		if err != nil {
			return err
		}
		if model.OwnerUserID != upstream.OwnerUserID {
			return ErrModelAccessDenied
		}
		displayName := fmt.Sprintf("%s · %s", row.UpstreamModelName, upstream.Name)
		if err := repo.UpdateModel(ctx, model.ID, repository.UpdateChannelModelInput{DisplayName: &displayName}); err != nil {
			return err
		}
		if !updateProtocols {
			continue
		}
		protocols, err := resolveRouteProtocols(nil, upstream.Compatible, upstream.ProtocolDefaultsJSON, model.KindsJSON)
		if err != nil {
			return err
		}
		existing, err := repo.ListPlatformModelRoutesByPair(ctx, upstream.ID, model.ID, row.ID)
		if err != nil {
			return err
		}
		routeIDs := make([]uint, 0, len(existing))
		for _, route := range existing {
			routeIDs = append(routeIDs, route.ID)
		}
		routes := make([]domainchannel.PlatformModelRoute, 0, len(protocols))
		for _, protocol := range protocols {
			route := replacementRouteTemplate(existing, protocol)
			route.Protocol = protocol
			route.PlatformModelID = model.ID
			route.UpstreamModelID = row.ID
			routes = append(routes, route)
		}
		if _, err := repo.ReplacePlatformModelRoutes(ctx, []repository.ReplaceChannelPlatformRoutesInput{{
			UpstreamID: upstream.ID, ExistingRouteIDs: routeIDs, Routes: routes,
		}}); err != nil {
			return err
		}
	}
	return nil
}
