package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccServerOnboardingResource covers the onboarding singleton with a
// structured prompts tree: the prompt and its options (including a unicode emoji
// and opt-in channel_ids) round-trip through create → update → import, and the
// server-assigned prompt/option ids are reflected in state.
func TestAccServerOnboardingResource(t *testing.T) {
	newMockDiscord(t)
	const rn = "discord_server_onboarding.test"

	cfg := func(promptTitle string) string {
		return `
resource "discord_server_onboarding" "test" {
  server_id           = "999"
  enabled             = true
  mode                = 1
  default_channel_ids = ["123"]

  prompts = [
    {
      type          = 0
      title         = "` + promptTitle + `"
      single_select = false
      required      = true
      in_onboarding = true

      options = [
        {
          title       = "News"
          description = "Get announcements."
          emoji_name  = "📣"
          channel_ids = ["123"]
        },
        {
          title = "Chat"
        },
      ]
    },
  ]
}
`
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{ // create
				Config: cfg("Pick your interests"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "enabled", "true"),
					resource.TestCheckResourceAttr(rn, "mode", "1"),
					resource.TestCheckResourceAttr(rn, "default_channel_ids.#", "1"),
					resource.TestCheckResourceAttr(rn, "prompts.#", "1"),
					resource.TestCheckResourceAttr(rn, "prompts.0.title", "Pick your interests"),
					resource.TestCheckResourceAttr(rn, "prompts.0.required", "true"),
					resource.TestCheckResourceAttr(rn, "prompts.0.options.#", "2"),
					resource.TestCheckResourceAttr(rn, "prompts.0.options.0.title", "News"),
					resource.TestCheckResourceAttr(rn, "prompts.0.options.0.description", "Get announcements."),
					resource.TestCheckResourceAttr(rn, "prompts.0.options.0.emoji_name", "📣"),
					resource.TestCheckResourceAttr(rn, "prompts.0.options.0.channel_ids.#", "1"),
					resource.TestCheckResourceAttr(rn, "prompts.0.options.1.title", "Chat"),
					// server-assigned ids are reflected back into state
					resource.TestCheckResourceAttrSet(rn, "prompts.0.id"),
					resource.TestCheckResourceAttrSet(rn, "prompts.0.options.0.id"),
					resource.TestCheckResourceAttrSet(rn, "prompts.0.options.1.id"),
				),
			},
			{ // update the prompt title (prompts tree refreshes, ids preserved)
				Config: cfg("What brings you here?"),
				Check:  resource.TestCheckResourceAttr(rn, "prompts.0.title", "What brings you here?"),
			},
			{ // import by server_id (singleton) — prompts must round-trip
				ResourceName:                         rn,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        "999",
				ImportStateVerifyIdentifierAttribute: "server_id",
			},
		},
	})
}
