package channel_test

import (
	"context"
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
