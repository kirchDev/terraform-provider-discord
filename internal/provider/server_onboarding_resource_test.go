package provider

import (
	"regexp"
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
      key           = "interests"
      type          = 0
      title         = "` + promptTitle + `"
      single_select = false
      required      = true
      in_onboarding = true

      options = [
        {
          key         = "news"
          title       = "News"
          description = "Get announcements."
          emoji_name  = "📣"
          channel_ids = ["123"]
        },
        {
          key   = "chat"
          title = "Chat"
        },
      ]
    },
  ]
}
`
	}

	var promptID, newsID, chatID string

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
					resource.TestCheckResourceAttr(rn, "prompts.0.key", "interests"),
					resource.TestCheckResourceAttr(rn, "prompts.0.title", "Pick your interests"),
					resource.TestCheckResourceAttr(rn, "prompts.0.required", "true"),
					resource.TestCheckResourceAttr(rn, "prompts.0.options.#", "2"),
					resource.TestCheckResourceAttr(rn, "prompts.0.options.0.key", "news"),
					resource.TestCheckResourceAttr(rn, "prompts.0.options.0.title", "News"),
					resource.TestCheckResourceAttr(rn, "prompts.0.options.0.description", "Get announcements."),
					resource.TestCheckResourceAttr(rn, "prompts.0.options.0.emoji_name", "📣"),
					resource.TestCheckResourceAttr(rn, "prompts.0.options.0.channel_ids.#", "1"),
					resource.TestCheckResourceAttr(rn, "prompts.0.options.1.key", "chat"),
					resource.TestCheckResourceAttr(rn, "prompts.0.options.1.title", "Chat"),
					// server-assigned ids are reflected back into state
					resource.TestCheckResourceAttrSet(rn, "prompts.0.id"),
					resource.TestCheckResourceAttrSet(rn, "prompts.0.options.0.id"),
					resource.TestCheckResourceAttrSet(rn, "prompts.0.options.1.id"),
					captureAttr(rn, "prompts.0.id", &promptID),
					captureAttr(rn, "prompts.0.options.0.id", &newsID),
					captureAttr(rn, "prompts.0.options.1.id", &chatID),
				),
			},
			// Rename — issue #35's second reproducer. `title` is Required but freely
			// editable, so it can never be the identity: renaming a prompt must not
			// re-mint the ids members' "Channels & Roles" selections hang on.
			{
				Config: cfg("What brings you here?"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "prompts.0.title", "What brings you here?"),
					checkAttrEquals(rn, "prompts.0.id", &promptID),
					checkAttrEquals(rn, "prompts.0.options.0.id", &newsID),
					checkAttrEquals(rn, "prompts.0.options.1.id", &chatID),
				),
			},
			{ // import by server_id (singleton) — prompts must round-trip
				ResourceName:                         rn,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        "999",
				ImportStateVerifyIdentifierAttribute: "server_id",
				// Discord does not store the caller's keys, so an import falls back
				// to the snowflake as the key; the caller re-states its own keys in
				// config afterwards.
				ImportStateVerifyIgnore: []string{
					"prompts.0.key",
					"prompts.0.options.0.key",
					"prompts.0.options.1.key",
				},
			},
		},
	})
}

// TestAccServerOnboardingResourceDuplicateKey guards the invariant the whole
// key-based identity rests on: two prompts sharing a key would both match the
// same prior element, and the loser's id would be silently re-minted. It fails
// at plan time instead.
func TestAccServerOnboardingResourceDuplicateKey(t *testing.T) {
	newMockDiscord(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "discord_server_onboarding" "test" {
  server_id = "999"
  enabled   = true

  prompts = [
    {
      key     = "interests"
      type    = 0
      title   = "Pick your interests"
      options = [{ key = "news", title = "News" }]
    },
    {
      key     = "interests"
      type    = 0
      title   = "Pick more interests"
      options = [{ key = "chat", title = "Chat" }]
    },
  ]
}
`,
				ExpectError: regexp.MustCompile(`prompt key "interests" is already used at index 0`),
			},
		},
	})
}

// TestAccServerOnboardingResourceInsert covers issue #35's first reproducer:
// inserting a prompt ahead of the existing ones. Identity is keyed on the
// caller's `key`, not on the list index, so the surrounding prompts and options
// keep the snowflakes Discord already assigned them and only the new prompt gets
// a fresh one.
func TestAccServerOnboardingResourceInsert(t *testing.T) {
	newMockDiscord(t)
	const rn = "discord_server_onboarding.test"

	prompt := func(key, title, optKey, optTitle string) string {
		return `
    {
      key           = "` + key + `"
      type          = 0
      title         = "` + title + `"
      single_select = false
      required      = true
      in_onboarding = true

      options = [
        {
          key   = "` + optKey + `"
          title = "` + optTitle + `"
        },
      ]
    },`
	}
	cfg := func(prompts ...string) string {
		body := ""
		for _, p := range prompts {
			body += p
		}
		return `
resource "discord_server_onboarding" "test" {
  server_id = "999"
  enabled   = true
  mode      = 0

  prompts = [` + body + `
  ]
}
`
	}

	interests := prompt("interests", "Pick your interests", "news", "News")
	region := prompt("region", "Where are you?", "eu", "Europe")
	pronouns := prompt("pronouns", "Your pronouns", "they", "they/them")

	var interestsID, newsID, regionID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg(interests, region),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "prompts.#", "2"),
					captureAttr(rn, "prompts.0.id", &interestsID),
					captureAttr(rn, "prompts.0.options.0.id", &newsID),
					captureAttr(rn, "prompts.1.id", &regionID),
				),
			},
			// A third prompt inserted at the front. Matching prior state by index
			// made every later element inherit its predecessor's id and left the
			// new trailing index with none at all ("was null, but now ...").
			{
				Config: cfg(pronouns, interests, region),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "prompts.#", "3"),
					resource.TestCheckResourceAttr(rn, "prompts.0.key", "pronouns"),
					resource.TestCheckResourceAttr(rn, "prompts.1.key", "interests"),
					resource.TestCheckResourceAttr(rn, "prompts.2.key", "region"),
					resource.TestCheckResourceAttrSet(rn, "prompts.0.id"),
					checkAttrEquals(rn, "prompts.1.id", &interestsID),
					checkAttrEquals(rn, "prompts.1.options.0.id", &newsID),
					checkAttrEquals(rn, "prompts.2.id", &regionID),
				),
			},
		},
	})
}
