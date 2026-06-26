package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// Ensure DiscordProvider satisfies the provider.Provider interface.
var _ provider.Provider = (*DiscordProvider)(nil)

// DiscordProvider is the provider implementation.
type DiscordProvider struct {
	// version is set to the release version on build, or "dev" for local builds.
	version string
}

// defaultAuditLogReason is recorded in the guild audit log for changes this
// provider makes, unless overridden (or disabled with an empty string).
const defaultAuditLogReason = "Managed by OpenTofu (terraform-provider-discord)"

// DiscordProviderModel maps provider schema data to a Go type.
type DiscordProviderModel struct {
	Token          types.String `tfsdk:"token"`
	Endpoint       types.String `tfsdk:"endpoint"`
	AuditLogReason types.String `tfsdk:"audit_log_reason"`
}

// New returns a function that instantiates the provider.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &DiscordProvider{version: version}
	}
}

func (p *DiscordProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "discord"
	resp.Version = p.version
}

func (p *DiscordProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage [Discord](https://discord.com) guild infrastructure — servers, roles, channels, " +
			"permissions, members, webhooks, events and moderation — as code. One bot token manages every guild the " +
			"bot is a member of.",
		Attributes: map[string]schema.Attribute{
			"token": schema.StringAttribute{
				MarkdownDescription: "Discord bot token. The provider authenticates as `Bot <token>`. " +
					"May also be set via the `DISCORD_TOKEN` environment variable.",
				Optional:  true,
				Sensitive: true,
			},
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "Base URL of the Discord REST API. Defaults to `https://discord.com/api/v10`. " +
					"May also be set via `DISCORD_ENDPOINT` (mainly for testing).",
				Optional: true,
			},
			"audit_log_reason": schema.StringAttribute{
				MarkdownDescription: "Reason recorded in the guild audit log (`X-Audit-Log-Reason`) for every change this " +
					"provider makes. Defaults to `" + defaultAuditLogReason + "`. Set to an empty string to disable. " +
					"May also be set via `DISCORD_AUDIT_LOG_REASON`.",
				Optional: true,
			},
		},
	}
}

func (p *DiscordProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config DiscordProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.Token.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("token"),
			"Unknown Discord bot token",
			"The provider cannot create the Discord API client because the token is unknown. "+
				"Set the value statically in the configuration or via the DISCORD_TOKEN environment variable.",
		)
		return
	}

	// Env vars are the default; explicit config wins.
	token := os.Getenv("DISCORD_TOKEN")
	if !config.Token.IsNull() {
		token = config.Token.ValueString()
	}

	endpoint := os.Getenv("DISCORD_ENDPOINT")
	if !config.Endpoint.IsNull() {
		endpoint = config.Endpoint.ValueString()
	}

	if token == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("token"),
			"Missing Discord bot token",
			"Set the provider `token` argument or the DISCORD_TOKEN environment variable.",
		)
		return
	}

	// Audit-log reason: explicit config wins, else the env var (if present),
	// else the default. An empty string disables the header.
	reason := defaultAuditLogReason
	if v, ok := os.LookupEnv("DISCORD_AUDIT_LOG_REASON"); ok {
		reason = v
	}
	if !config.AuditLogReason.IsNull() {
		reason = config.AuditLogReason.ValueString()
	}

	c := client.New(endpoint, token, reason)
	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *DiscordProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewManagedServerResource,
		NewRoleResource,
		NewRoleEveryoneResource,
		NewRoleOrderResource,
		NewCategoryChannelResource,
		NewTextChannelResource,
		NewVoiceChannelResource,
		NewNewsChannelResource,
		NewStageChannelResource,
		NewForumChannelResource,
		NewForumPostResource,
		NewMessageResource,
		NewMediaChannelResource,
		NewChannelPermissionResource,
		NewChannelOrderResource,
		NewMemberRolesResource,
		NewMemberRoleResource,
		NewMemberNicknameResource,
		NewInviteResource,
		NewWebhookResource,
		NewStageInstanceResource,
		NewGuildTemplateResource,
		NewThreadResource,
		NewEmojiResource,
		NewApplicationEmojiResource,
		NewStickerResource,
		NewSoundboardSoundResource,
		NewScheduledEventResource,
		NewAutoModerationRuleResource,
		NewGuildBanResource,
		NewWelcomeScreenResource,
		NewServerOnboardingResource,
		NewMemberVerificationResource,
		NewGuildWidgetResource,
		NewApplicationCommandResource,
	}
}

func (p *DiscordProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewPermissionDataSource,
		NewColorDataSource,
		NewApplicationDataSource,
		NewWebhookDataSource,
		NewEmojiDataSource,
		NewApplicationCommandDataSource,
		NewRolesDataSource,
		NewChannelsDataSource,
		NewEmojisDataSource,
		NewBansDataSource,
		NewMembersDataSource,
		NewServerDataSource,
		NewRoleDataSource,
		NewMemberDataSource,
		NewUserDataSource,
		NewChannelDataSource,
		NewLocalImageDataSource,
		NewLocalFileDataSource,
		NewInviteDataSource,
	}
}
