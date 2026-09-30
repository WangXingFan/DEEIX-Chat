package channel_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	appchannel "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/channel"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	channelrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/channel"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/schema"
	"gorm.io/gorm"
)

// openUserUpstreamTestDB 在渠道测试库上补播种生产启动时会写入的内置厂商目录。
// 平台模型写入要求厂商标识已存在于 llm_model_vendors，否则会以 ErrModelVendorNotFound 失败。
func openUserUpstreamTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openModelProtocolsTestDB(t)
	if err := db.AutoMigrate(&models.PermissionGroupModelAccess{}, &models.PermissionGroupModelRule{}); err != nil {
		t.Fatal(err)
	}
	if err := schema.SeedModelVendors(db); err != nil {
		t.Fatalf("seed model vendors: %v", err)
	}
	return db
}

func TestImportedUserModelsAppearOnlyInOwnerCatalog(t *testing.T) {
	db := openUserUpstreamTestDB(t)
	upstream := models.LLMUpstream{OwnerUserID: 7, Name: "my-provider", Compatible: "openai", Status: "active"}
	if err := db.Create(&upstream).Error; err != nil {
		t.Fatal(err)
	}
	repo := channelrepo.NewRepo(db)
	service := appchannel.NewServiceWithRuntime(config.NewRuntime(config.Config{}), repo, repo, nil, nil)
	ctx := context.Background()
	imported, err := service.ImportUserModels(ctx, 7, upstream.ID, []string{"gpt-4o", "gpt-4o", "gpt-4o-mini"})
	if err != nil || len(imported) != 2 {
		t.Fatalf("import models: items=%#v err=%v", imported, err)
	}
	catalog, err := service.ListActiveModels(ctx, 7)
	if err != nil || len(catalog) != 2 {
		t.Fatalf("owner catalog: items=%#v err=%v", catalog, err)
	}
	for _, item := range catalog {
		if item.OwnerUserID != 7 || item.ActiveSourceCount == 0 || item.DisplayName == "" || item.ProtocolsJSON == "[]" {
			t.Fatalf("expected available private model with display metadata, got %#v", item)
		}
	}
	for _, userID := range []uint{0, 8} {
		catalog, err = service.ListActiveModels(ctx, userID)
		if err != nil || len(catalog) != 0 {
			t.Fatalf("catalog for user %d must hide private models: items=%#v err=%v", userID, catalog, err)
		}
	}
	if err = db.Model(&upstream).Update("status", "inactive").Error; err != nil {
		t.Fatal(err)
	}
	catalog, err = service.ListActiveModels(ctx, 7)
	if err != nil || len(catalog) != 0 {
		t.Fatalf("inactive upstream must hide private models: items=%#v err=%v", catalog, err)
	}
}

func TestUserUpstreamEditsRefreshBindingsAndPreserveIdentity(t *testing.T) {
	db := openUserUpstreamTestDB(t)
	upstream := models.LLMUpstream{OwnerUserID: 7, Name: "before", Compatible: "openai", Status: "active"}
	if err := db.Create(&upstream).Error; err != nil {
		t.Fatal(err)
	}
	repo := channelrepo.NewRepo(db)
	service := appchannel.NewServiceWithRuntime(config.NewRuntime(config.Config{}), repo, repo, nil, nil)
	ctx := t.Context()
	if _, err := service.ImportUserModels(ctx, 7, upstream.ID, []string{"gpt-4o"}); err != nil {
		t.Fatal(err)
	}
	before, err := service.ListUserModels(ctx, 7)
	if err != nil || len(before) != 1 {
		t.Fatalf("before: %#v %v", before, err)
	}
	name, compatible := "after", "anthropic"
	if _, err := service.UpdateUserUpstream(ctx, 7, upstream.ID, appchannel.UpdateUpstreamInput{Name: &name, Compatible: &compatible}); err != nil {
		t.Fatal(err)
	}
	after, err := service.ListUserModels(ctx, 7)
	if err != nil || len(after) != 1 || after[0].PlatformModelName != before[0].PlatformModelName || after[0].DisplayName != "gpt-4o · after" {
		t.Fatalf("identity/display after update: %#v %v", after, err)
	}
	bindings, err := service.ListUserUpstreamModels(ctx, 7, upstream.ID)
	if err != nil || len(bindings) != 1 || bindings[0].Protocol != "anthropic_messages" {
		t.Fatalf("updated protocols: %#v %v", bindings, err)
	}
	for _, userID := range []uint{0, 8} {
		_, err := service.ResolveRoute(ctx, appchannel.ResolveRouteInput{PlatformModelName: before[0].PlatformModelName, UserID: userID})
		if !errors.Is(err, appchannel.ErrModelAccessDenied) {
			t.Fatalf("foreign route: %v", err)
		}
	}
	_, err = service.ResolveDefaultRoute(ctx, appchannel.ResolveRouteInput{PlatformModelName: before[0].PlatformModelName, UserID: 7})
	if !errors.Is(err, appchannel.ErrAllRoutesUnavailable) {
		t.Fatalf("private default fallback: %v", err)
	}
}

func TestUserUpstreamProtocolChangeRollsBackUnsupportedModels(t *testing.T) {
	db := openUserUpstreamTestDB(t)
	upstream := models.LLMUpstream{OwnerUserID: 7, Name: "before", Compatible: "openai", Status: "active"}
	if err := db.Create(&upstream).Error; err != nil {
		t.Fatal(err)
	}
	repo := channelrepo.NewRepo(db)
	service := appchannel.NewServiceWithRuntime(config.NewRuntime(config.Config{}), repo, repo, nil, nil)
	if _, err := service.ImportUserModels(t.Context(), 7, upstream.ID, []string{"gpt-image-1"}); err != nil {
		t.Fatal(err)
	}
	name, compatible := "after", "anthropic"
	if _, err := service.UpdateUserUpstream(t.Context(), 7, upstream.ID, appchannel.UpdateUpstreamInput{Name: &name, Compatible: &compatible}); err == nil {
		t.Fatal("accepted unsupported image protocol")
	}
	stored, err := repo.GetUpstreamByID(t.Context(), upstream.ID)
	if err != nil || stored.Name != "before" || stored.Compatible != "openai" {
		t.Fatalf("upstream update not rolled back: %#v %v", stored, err)
	}
	bindings, err := service.ListUserUpstreamModels(t.Context(), 7, upstream.ID)
	if err != nil || len(bindings) != 2 {
		t.Fatalf("bindings not preserved: %#v %v", bindings, err)
	}
}

func TestUserModelDeletionRetainsOtherProtocolsAndOwners(t *testing.T) {
	db := openUserUpstreamTestDB(t)
	repo := channelrepo.NewRepo(db)
	service := appchannel.NewServiceWithRuntime(config.NewRuntime(config.Config{}), repo, repo, nil, nil)
	ctx := t.Context()
	upstreams := []models.LLMUpstream{
		{OwnerUserID: 7, Name: "first", Compatible: "openai", Status: "active"},
		{OwnerUserID: 7, Name: "second", Compatible: "openai", Status: "active"},
		{OwnerUserID: 8, Name: "other", Compatible: "openai", Status: "active"},
	}
	identities := make(map[string]bool)
	for i := range upstreams {
		upstream := &upstreams[i]
		if err := db.Create(upstream).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := service.ImportUserModels(ctx, upstream.OwnerUserID, upstream.ID, []string{"gpt-image-1"}); err != nil {
			t.Fatal(err)
		}
		bindings, err := service.ListUserUpstreamModels(ctx, upstream.OwnerUserID, upstream.ID)
		if err != nil || len(bindings) != 2 {
			t.Fatalf("image bindings: %#v %v", bindings, err)
		}
		identity := bindings[0].PlatformModelName
		if identities[identity] {
			t.Fatalf("same-name models share identity %q", identity)
		}
		identities[identity] = true
	}
	first := upstreams[0]
	bindings, err := service.ListUserUpstreamModels(ctx, 7, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteUserUpstreamModel(ctx, 8, first.ID, bindings[0].RouteID); err == nil {
		t.Fatal("foreign delete accepted")
	}
	if err := service.DeleteUserUpstreamModel(ctx, 7, first.ID, bindings[0].RouteID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetModelByName(ctx, bindings[0].PlatformModelName); err != nil {
		t.Fatalf("model removed before last protocol: %v", err)
	}
	if err := service.DeleteUserUpstreamModel(ctx, 7, first.ID, bindings[1].RouteID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetModelByName(ctx, bindings[0].PlatformModelName); !errors.Is(err, appchannel.ErrModelNotFound) {
		t.Fatalf("orphan model remains: %v", err)
	}
	if err := service.DeleteUserUpstream(ctx, 7, upstreams[1].ID); err != nil {
		t.Fatal(err)
	}
	ownerModels, err := service.ListUserModels(ctx, 7)
	if err != nil || len(ownerModels) != 0 {
		t.Fatalf("owner cleanup: %#v %v", ownerModels, err)
	}
	otherModels, err := service.ListUserModels(ctx, 8)
	if err != nil || len(otherModels) != 1 {
		t.Fatalf("other user's model lost: %#v %v", otherModels, err)
	}
}

func TestUserModelCatalogLoadsEveryPage(t *testing.T) {
	db := openUserUpstreamTestDB(t)
	upstream := models.LLMUpstream{OwnerUserID: 7, Name: "many-models", Compatible: "openai", Status: "active"}
	if err := db.Create(&upstream).Error; err != nil {
		t.Fatal(err)
	}
	const modelCount = 505
	platformModels := make([]models.LLMPlatformModel, modelCount)
	upstreamModels := make([]models.LLMUpstreamModel, modelCount)
	for index := range platformModels {
		name := fmt.Sprintf("model-%03d", index)
		platformModels[index] = models.LLMPlatformModel{OwnerUserID: 7, Name: name, KindsJSON: `["chat"]`, Status: "active", SortOrder: index}
		upstreamModels[index] = models.LLMUpstreamModel{UpstreamID: upstream.ID, BindingCode: name, UpstreamModelName: name, KindsJSON: `["chat"]`, Status: "active"}
	}
	if err := db.CreateInBatches(&platformModels, 100).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.CreateInBatches(&upstreamModels, 100).Error; err != nil {
		t.Fatal(err)
	}
	routes := make([]models.LLMPlatformModelRoute, modelCount)
	for index := range routes {
		routes[index] = models.LLMPlatformModelRoute{PlatformModelID: platformModels[index].ID, UpstreamModelID: upstreamModels[index].ID, Protocol: "openai_chat_completions", Status: "active"}
	}
	if err := db.CreateInBatches(&routes, 100).Error; err != nil {
		t.Fatal(err)
	}
	repo := channelrepo.NewRepo(db)
	service := appchannel.NewServiceWithRuntime(config.NewRuntime(config.Config{}), repo, repo, nil, nil)
	catalog, err := service.ListUserModels(context.Background(), 7)
	if err != nil || len(catalog) != modelCount {
		t.Fatalf("expected all %d private models, got %d: %v", modelCount, len(catalog), err)
	}
	for index, item := range catalog {
		if item.PlatformModelName != platformModels[index].Name {
			t.Fatalf("unexpected model at index %d: %q", index, item.PlatformModelName)
		}
	}
}
