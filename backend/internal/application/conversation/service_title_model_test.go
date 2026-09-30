package conversation

import (
	"context"
	"errors"
	"testing"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/channel"
)

func TestTitleModelUsesUserPreferenceAndUserScope(t *testing.T) {
	for _, tc := range []struct{ name, preference, want string }{
		{"explicit", "my-title-model", "my-title-model"},
		{"follow", "follow", "conversation-model"},
		{"unset", "", "conversation-model"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			denied := channel.ErrModelAccessDenied
			resolver := &textTaskRouteResolverStub{fail: map[string]error{tc.want: denied}}
			service := &Service{
				repo:          &mutableUserSettingsRepository{values: map[uint]map[string]string{7: {"chat.title_model": tc.preference}, 8: {"chat.title_model": "other-user-model"}}},
				routeResolver: resolver,
			}
			_, err := service.callConversationMetadataLLM(context.Background(), conversationMetadataLLMInput{
				ConfiguredModel: "admin-internal-model", ConversationModel: "conversation-model", UserID: 7, ConversationID: 9, ServiceCode: "title",
			})
			if err == nil || len(resolver.inputs) != 1 {
				t.Fatalf("expected route rejection, got %v, %#v", err, resolver.inputs)
			}
			input := resolver.inputs[0]
			if input.PlatformModelName != tc.want || input.Scope != channel.RouteScopeUser || input.UserID != 7 || input.ConversationID != 9 {
				t.Fatalf("wrong title route: %#v", input)
			}
			if tc.name == "explicit" && (!errors.Is(err, denied) || len(resolver.defaultInputs) != 0) {
				t.Fatalf("explicit title model must not fall back: %v", err)
			}
		})
	}
}

func TestPrivateTextTaskFollowDoesNotAddDefaultRoute(t *testing.T) {
	resolver := &textTaskRouteResolverStub{
		routes:       map[string]*channel.ResolvedRoute{"private": {PlatformModelName: "private", UserConfigured: true}},
		defaultRoute: &channel.ResolvedRoute{PlatformModelName: "public"},
	}
	service := &Service{routeResolver: resolver}
	routes, err := service.resolveTextTaskRouteCandidates(context.Background(), textTaskRouteInput{ConfiguredModel: "follow", ConversationModel: "private", UserID: 7})
	if err != nil || len(routes) != 1 || !routes[0].UserConfigured || len(resolver.defaultInputs) != 0 {
		t.Fatalf("private task must stay on its upstream: routes=%#v defaults=%#v err=%v", routes, resolver.defaultInputs, err)
	}
}

func TestFailedTextTaskFollowPassesOriginalModelToDefaultResolver(t *testing.T) {
	resolver := &textTaskRouteResolverStub{}
	service := &Service{routeResolver: resolver}
	_, _ = service.resolveTextTaskRouteCandidates(context.Background(), textTaskRouteInput{ConfiguredModel: "follow", ConversationModel: "private", UserID: 7, UserSelectedModel: true})
	if len(resolver.defaultInputs) != 1 || resolver.defaultInputs[0].PlatformModelName != "private" || resolver.defaultInputs[0].Scope != channel.RouteScopeUser {
		t.Fatalf("default resolver needs original identity and user scope: %#v", resolver.defaultInputs)
	}
}
